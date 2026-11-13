package tvrock

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"time"
)

// 完全なパケットとして追加されたもののみがスキャン対象となります。
// ReadAtを使用する場合、再生カーソルはインデックス作成処理とは独立して維持されます。
// これには再生が一時停止されている場合や、シークによって巻き戻された場合も含まれます。
type playbackIndexTail struct {
	offset, patOffset, previous int64
	elapsed, patTime, indexed   time.Duration
	seen                        bool
}

func (p *playbackIndex) extend(ctx context.Context, file *os.File) error {
	t := p.tail
	if t == nil {
		return nil
	}
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < t.offset {
		return errors.New("再生中の録画ファイルが短くなりました")
	}
	end := info.Size() / 188 * 188
	if end == t.offset {
		return nil
	}

	reader := bufio.NewReaderSize(io.NewSectionReader(file, t.offset, end-t.offset), 188*1024)
	packet := make([]byte, 188)
	for t.offset < end {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := io.ReadFull(reader, packet); err != nil {
			return err
		}
		if packet[0] != 0x47 {
			return errors.New("188バイト形式のTSファイルに対応しています")
		}
		if packet[1]&31 == 0 && packet[2] == 0 && packet[1]&0x40 != 0 {
			t.patOffset, t.patTime = t.offset, t.elapsed
		}
		if pcr, ok := playbackPCR(packet, p.pcrPID); ok {
			if t.seen {
				if pcr != t.previous && playbackPCRDelta(t.previous, pcr) == 0 {
					p.metrics.PCRDiscontinuities++
				}
				t.elapsed += playbackPCRDelta(t.previous, pcr)
			}
			if !t.seen || t.elapsed-t.indexed >= 500*time.Millisecond {
				if len(p.points) == 0 || p.points[len(p.points)-1].offset != t.patOffset {
					p.points = append(p.points, playbackPoint{t.patOffset, t.patTime})
				}
				t.indexed = t.elapsed
			}
			t.previous, t.seen = pcr, true
		}
		t.offset += 188
	}
	p.duration = t.elapsed
	if t.elapsed > 0 {
		p.metrics.AverageBitrate = int64(float64(t.offset) * 8 / t.elapsed.Seconds())
	}
	if len(p.metrics.Candidates) > 0 {
		p.metrics.CMStatus = "splice-candidates"
	}

	return nil
}

// 起動時のデータが揃うまで少し待機する。
// 進行中のスナップショットを「完了した分析」として保存してはならない。
// 最終的なスナップショットは記録終了後のジョブが別途保存する。
func loadGrowingPlaybackTS(ctx context.Context, file *os.File, schedule *RecordingSchedule) (*playbackIndex, error) {
	for {
		index, err := indexPlaybackTS(ctx, file, schedule.Sid, true)
		if err == nil {
			return index, nil
		}
		if !recordingTSStillActive(schedule.ID) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// 録画中のEOFは一時的なものです。録画が停止した後はレコーダーによる
// 最後の書き込みが行われるための短い無音期間を設け、その後に真の終了とみなすようにします。
type playbackTailWait struct {
	size       int64
	quietSince time.Time
}

func (w *playbackTailWait) finished(active bool, size int64, now time.Time) bool {
	if active || size != w.size || w.quietSince.IsZero() {
		w.size = size
		w.quietSince = now
		return false
	}
	return now.Sub(w.quietSince) >= 2*time.Second
}
