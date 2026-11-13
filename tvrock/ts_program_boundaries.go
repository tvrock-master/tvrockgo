package tvrock

import (
	"encoding/binary"
	"time"
)

type tsProgramKey struct {
	event uint16
	start [5]byte
}

// 現在のストリームにおけるEIT p/f（セクション0）のみを読み取る。ディスクリプタやテキストのデコードは行わない。
// 不正な形式のデータや無関係な放送データが含まれていても再生が停止してはならない。
func (s *tsMetadataScanner) observePresentProgram(section []byte, pid uint16, offset int64, elapsed time.Duration, hasPCR bool) {
	if len(section) < 30 || section[0] != 0x4e || section[1]&0x80 == 0 || section[5]&1 == 0 || section[6] != 0 || s.metrics.ServiceID == 0 {
		return
	}
	if binary.BigEndian.Uint16(section[3:5]) != s.metrics.ServiceID || binary.BigEndian.Uint16(section[8:10]) != s.metrics.TransportStreamID {
		return
	}
	end := len(section) - 4
	length := int(section[24]&15)<<8 | int(section[25])
	if 26+length != end {
		return
	}
	for at := 26; at < end; {
		if at+2 > end || at+2+int(section[at+1]) > end {
			return
		}
		at += 2 + int(section[at+1])
	}
	key := tsProgramKey{event: binary.BigEndian.Uint16(section[14:16])}
	copy(key.start[:], section[16:21])
	if s.programPID == 0 {
		s.programPID = pid
	} else if s.programPID != pid {
		// 通常のEIT PIDを優先すること。ソース切り替え時には基準を新たに設定し
		// 下位レイヤーからの遅延したEITによって偽の変更が交互に発生する事態を防ぐこと。
		if pid != 0x12 {
			return
		}
		s.programPID = pid
		s.haveProgram = false
	}
	if !s.haveProgram || key.event == s.programID {
		s.programID = key.event
		s.haveProgram = true
		s.programSeen[key] = true
		return
	}
	// より新しい現在のイベントが到着した後、古いセクションが繰り返されることがある
	if s.programSeen[key] {
		return
	}
	previous := s.programID
	s.programID = key.event
	s.programSeen[key] = true
	if !hasPCR {
		return
	}
	s.metrics.ProgramBoundaries = append(s.metrics.ProgramBoundaries,
		tsBoundaryCandidate{TimeNS: int64(elapsed), Offset: offset,
			Kind: "program-change", Evidence: "eit-pf-present", EventID: uint32(key.event), PreviousEventID: previous})
}

func programPlaybackBoundaries(index *playbackIndex) []playbackBoundary {
	rows := []playbackBoundary{}
	for _, row := range index.metrics.ProgramBoundaries {
		if row.TimeNS >= 0 && row.TimeNS <= int64(index.duration) {
			rows = append(rows, playbackBoundary{PositionMS: row.TimeNS / int64(time.Millisecond),
				Kind: "program-change", EventID: row.EventID, PreviousEventID: row.PreviousEventID})
		}
	}
	return rows
}
