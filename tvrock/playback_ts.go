package tvrock

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"github.com/asticode/go-astits"
)

// オフセットは常に変更が加えられていない元の188バイトTSパケットを指す。
// インデックスエントリはその直前のPATから始まるため、シークを行う際、視聴側でキャッシュされたPSIに依存することはない。
type playbackPoint struct {
	offset int64
	time   time.Duration
}

type playbackIndex struct {
	metrics         tsAnalysisMetrics
	points          []playbackPoint
	duration        time.Duration
	pcrPID          uint16
	tail            *playbackIndexTail
	analysisVersion int64
}

func playbackPCR(packet []byte, pid uint16) (int64, bool) {
	if len(packet) != 188 || packet[0] != 0x47 || packet[1]&0x80 != 0 ||
		(uint16(packet[1]&31)<<8|uint16(packet[2])) != pid || packet[3]&0x20 == 0 ||
		packet[4] < 7 || packet[4] > 183 || packet[5]&0x10 == 0 {
		return 0, false
	}
	p := packet[6:12]
	base := int64(p[0])<<25 | int64(p[1])<<17 | int64(p[2])<<9 | int64(p[3])<<1 | int64(p[4]>>7)
	ext := int64(p[4]&1)<<8 | int64(p[5])
	if ext >= 300 {
		return 0, false
	}
	return base*300 + ext, true
}

// PCRのラップアラウンド周期は約26.5時間です。大きな値の跳躍は不連続点であり
// 待ち時間が発生しているわけではありません。複数の録画データを連結する際は経過タイムラインを維持してください。
func playbackPCRDelta(previous, current int64) time.Duration {
	const wrap = (int64(1) << 33) * 300
	delta := (current - previous + wrap) % wrap
	if delta > 27000000*10 {
		return 0
	}
	return time.Duration(delta * int64(time.Second) / 27000000)
}

func indexPlaybackTS(ctx context.Context, file *os.File, sid uint, growing ...bool) (*playbackIndex, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	// ここではPATと選択したプログラムのPMTのみが必要となる。
	// 再生可能なTSであってもそれらと無関係な放送SI/PESを解析しようとするとPMTの取得に至る前に処理が失敗する可能性がある。
	pmtPIDs := map[uint16]bool{}
	var source io.Reader = file
	live := len(growing) > 0 && growing[0]
	if live {
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		source = io.NewSectionReader(file, 0, info.Size()/188*188)
	}

	dem := astits.NewDemuxer(ctx, io.LimitReader(source, 16*1024*1024), astits.DemuxerOptPacketSize(188),
		astits.DemuxerOptPacketSkipper(func(packet *astits.Packet) bool {
			return packet.Header.PID != astits.PIDPAT && !pmtPIDs[packet.Header.PID]
		}))
	var pid uint16
	index := &playbackIndex{}
	found := false
	sawPAT, advertised := false, false
	for !found {
		data, err := dem.NextData()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if errors.Is(err, astits.ErrNoMorePackets) {
				if !sawPAT {
					return nil, errors.New("TS先頭16MiBにPATが見つかりません")
				}
				if !advertised {
					return nil, fmt.Errorf("TSのPATに録画ログのサービスID %d がありません", sid&65535)
				}
				return nil, fmt.Errorf("TS先頭16MiBにサービスID %d のPMTが見つかりません", sid&65535)
			}
			return nil, fmt.Errorf("TSのPAT/PMT解析に失敗しました（SID=%d）: %w", sid&65535, err)
		}
		if data.PAT != nil {
			sawPAT = true
			index.metrics.TransportStreamID = data.PAT.TransportStreamID
			// A PAT may span multiple sections; retain matching candidates.
			for _, program := range data.PAT.Programs {
				if program.ProgramNumber != 0 && (sid == 0 || program.ProgramNumber == uint16(sid&65535)) {
					pmtPIDs[program.ProgramMapID] = true
					advertised = true
				}
			}
		}
		if data.PMT != nil && (sid == 0 || data.PMT.ProgramNumber == uint16(sid&65535)) {
			index.metrics.ServiceID = data.PMT.ProgramNumber
			pid, found = data.PMT.PCRPID, true
			index.metrics.PMTPID = data.PID
			for _, stream := range data.PMT.ElementaryStreams {
				index.metrics.Streams = append(index.metrics.Streams, tsStreamInfo{stream.ElementaryPID, uint8(stream.StreamType), stream.StreamType.String()})
			}
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	if live {
		// レコーダーがデータを追記している最中であってもパケット全体が揃った固定のスナップショットを使用する
		section := source.(*io.SectionReader)
		section.Seek(0, io.SeekStart)
	} else {
		source = file
	}

	reader := bufio.NewReaderSize(source, 188*1024)
	index.pcrPID = pid
	scanner := newTSMetadataScanner(&index.metrics, sid)
	index.analysisVersion = tsAnalysisCacheVersion(scanner.pidCM)
	var previous, offset, patOffset int64
	var elapsed, patTime, indexed time.Duration
	seen := false
	packet := make([]byte, 188)
	for {
		if offset%(188*1024) == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		_, err := io.ReadFull(reader, packet)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("TSパケットが途中で切れています")
		}
		if packet[0] != 0x47 {
			return nil, errors.New("188バイト形式のTSファイルに対応しています")
		}
		if packet[1]&31 == 0 && packet[2] == 0 && packet[1]&0x40 != 0 {
			patOffset, patTime = offset, elapsed
		}
		if pcr, ok := playbackPCR(packet, pid); ok {
			if seen {
				if pcr != previous && playbackPCRDelta(previous, pcr) == 0 {
					index.metrics.PCRDiscontinuities++
				}
				elapsed += playbackPCRDelta(previous, pcr)
			}
			if !seen || elapsed-indexed >= 500*time.Millisecond {
				if len(index.points) == 0 || index.points[len(index.points)-1].offset != patOffset {
					index.points = append(index.points, playbackPoint{patOffset, patTime})
				}
				indexed = elapsed
			}
			previous, seen = pcr, true
		}
		if !live {
			scanner.observe(packet, offset, elapsed, previous, seen)
		}
		offset += 188
	}
	if !seen || elapsed <= 0 {
		return nil, errors.New("再生時間を読み取れるPCRがありません")
	}
	index.duration = elapsed
	index.metrics.AverageBitrate = int64(float64(offset) * 8 / elapsed.Seconds())
	if live {
		index.tail = &playbackIndexTail{offset: offset, patOffset: patOffset, previous: previous, elapsed: elapsed, patTime: patTime, indexed: indexed, seen: seen}
		index.metrics.CMStatus = "recording"
		if len(index.metrics.Candidates) > 0 {
			index.metrics.CMStatus = "splice-candidates"
		}
	} else {
		scanner.finish(elapsed)
	}
	_, err := file.Seek(0, io.SeekStart)
	return index, err
}

func (p *playbackIndex) seek(target time.Duration) playbackPoint {
	i := sort.Search(len(p.points), func(i int) bool { return p.points[i].time > target }) - 1
	if i < 0 {
		i = 0
	}
	return p.points[i]
}
