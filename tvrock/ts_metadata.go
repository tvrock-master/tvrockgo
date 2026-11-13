package tvrock

import (
	"encoding/binary"
	"sort"
	"time"
)

// Metadata only: 映像・音声のデコードは行わず、ソースのTSにも変更を加えない
type tsMetadataScanner struct {
	metrics     *tsAnalysisMetrics
	cc          [8192]byte
	seen        [8192]bool
	sections    map[uint16]*tsSectionAssembler
	cues        map[tsCueKey]bool
	programPID  uint16
	programSeen map[tsProgramKey]bool
	programID   uint16
	haveProgram bool
	pidCM       *tsPIDCMScanner
}

type tsCueKey struct {
	event uint32
	pts   uint64
	out   bool
}

type tsSectionAssembler struct {
	data []byte
	cc   byte
	seen bool
}

func newTSMetadataScanner(metrics *tsAnalysisMetrics, sid uint) *tsMetadataScanner {
	s := &tsMetadataScanner{metrics: metrics, sections: map[uint16]*tsSectionAssembler{}, cues: map[tsCueKey]bool{}}
	s.pidCM = newTSPIDCMScanner(sid)
	metrics.CMStatus = "not-signaled"
	metrics.Candidates = []tsBoundaryCandidate{}
	metrics.ProgramBoundaries = []tsBoundaryCandidate{}
	s.programSeen = map[tsProgramKey]bool{}
	s.sections[0x12] = &tsSectionAssembler{}
	s.sections[0x27] = &tsSectionAssembler{}
	for _, stream := range metrics.Streams {
		if stream.Type == 0x86 {
			s.sections[stream.PID] = &tsSectionAssembler{}
			metrics.CMStatus = "no-supported-splice-boundary"
		}
	}
	return s
}

func tsPayloadOffset(packet []byte) int {
	if len(packet) != 188 || packet[3]&0x10 == 0 {
		return -1
	}
	offset := 4
	if packet[3]&0x20 != 0 {
		offset += 1 + int(packet[4])
	}
	if offset >= 188 {
		return -1
	}
	return offset
}

func (s *tsMetadataScanner) observe(packet []byte, offset int64, elapsed time.Duration, pcr int64, hasPCR bool) {
	m := s.metrics
	m.Packets++
	pid := uint16(packet[1]&31)<<8 | uint16(packet[2])
	if hasPCR && s.pidCM != nil {
		s.pidCM.observe(m, pid, offset, elapsed, packet[1]&0x80 == 0)
	}
	if packet[1]&0x80 != 0 {
		m.TEI++
		s.seen[pid] = false
		if assembler := s.sections[pid]; assembler != nil {
			assembler.data = nil
			assembler.seen = false
		}
		return
	}
	if packet[3]&0xc0 != 0 {
		m.Scrambled++
	}
	discontinuity := packet[3]&0x20 != 0 && packet[4] > 0 && packet[4] <= 183 && packet[5]&0x80 != 0
	if discontinuity {
		s.seen[pid] = false
	}
	payload := tsPayloadOffset(packet)
	if pid != 8191 && payload >= 0 {
		cc := packet[3] & 15
		if s.seen[pid] && cc != s.cc[pid] && cc != (s.cc[pid]+1)&15 {
			m.ContinuityErrors++
		}
		s.cc[pid], s.seen[pid] = cc, true
	}
	if assembler := s.sections[pid]; assembler != nil {
		if discontinuity {
			assembler.data = nil
			assembler.seen = false
		}
		assembler.feed(packet, func(section []byte) {
			if pid == 0x12 || pid == 0x27 {
				s.observePresentProgram(section, pid, offset, elapsed, hasPCR)
				return
			}
			if !hasPCR {
				return
			}
			id, pts, out, ok := parseTSSpliceInsert(section)
			if !ok {
				return
			}
			key := tsCueKey{id, pts, out}
			if s.cues[key] {
				return
			}
			s.cues[key] = true
			const wrap = int64(1) << 33
			delta := (int64(pts) - pcr/300 + wrap) % wrap
			if delta > wrap/2 {
				delta -= wrap
			}
			at := elapsed + time.Duration(delta)*time.Second/90000
			if at < 0 {
				return
			}
			kind := "splice-in"
			if out {
				kind = "splice-out"
			}
			m.Candidates = append(m.Candidates, tsBoundaryCandidate{
				TimeNS:   int64(at),
				Offset:   offset,
				Kind:     kind,
				Evidence: "scte35-splice-insert",
				EventID:  id, ConfirmedCM: false,
			})
		})
	}
}

func (s *tsMetadataScanner) finish(duration time.Duration) {
	list := s.metrics.Candidates[:0]
	for _, candidate := range s.metrics.Candidates {
		if candidate.TimeNS <= int64(duration) {
			list = append(list, candidate)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].TimeNS < list[j].TimeNS })
	s.metrics.Candidates = list
	if len(list) > 0 {
		s.metrics.CMStatus = "splice-candidates"
	}
	programs := s.metrics.ProgramBoundaries[:0]
	for _, row := range s.metrics.ProgramBoundaries {
		if row.TimeNS >= 0 && row.TimeNS <= int64(duration) {
			programs = append(programs, row)
		}
	}
	sort.SliceStable(programs, func(i, j int) bool { return programs[i].TimeNS < programs[j].TimeNS })
	s.metrics.ProgramBoundaries = programs
}

func (a *tsSectionAssembler) consume(data []byte, emit func([]byte)) {
	a.data = append(a.data, data...)
	for len(a.data) >= 3 {
		if a.data[0] == 0xff {
			a.data = nil
			return
		}
		size := 3 + (int(a.data[1]&15)<<8 | int(a.data[2]))
		if size < 7 || size > 4096 {
			a.data = nil
			return
		}
		if len(a.data) < size {
			return
		}
		section := a.data[:size]
		if tsSectionCRC(section) == 0 {
			emit(section)
		}
		a.data = a.data[size:]
	}
}

func (a *tsSectionAssembler) feed(packet []byte, emit func([]byte)) {
	start := tsPayloadOffset(packet)
	if start < 0 || packet[1]&0x80 != 0 || packet[3]&0xc0 != 0 {
		a.data = nil
		a.seen = false
		return
	}
	cc := packet[3] & 15
	if a.seen {
		if cc == a.cc {
			return
		}
		if cc != (a.cc+1)&15 {
			a.data = nil
		}
	}
	a.cc, a.seen = cc, true
	payload := packet[start:]
	if packet[1]&0x40 != 0 {
		pointer := int(payload[0])
		payload = payload[1:]
		if pointer > len(payload) {
			a.data = nil
			return
		}
		if len(a.data) > 0 {
			a.consume(payload[:pointer], emit)
		}
		a.data = nil
		a.consume(payload[pointer:], emit)
	} else if len(a.data) > 0 {
		a.consume(payload, emit)
	}
}

func tsSectionCRC(data []byte) uint32 {
	crc := uint32(0xffffffff)
	for _, b := range data {
		crc ^= uint32(b) << 24
		for range 8 {
			if crc&0x80000000 != 0 {
				crc = crc<<1 ^ 0x04c11db7
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

// 明示的なsplice_insertプログラム／PTS形式のみが対象です。
// スプライスは挿入の機会を示すものでありCMの存在を証明するものではありません。
// サポートされていないコマンド、暗号化されたセクション、キャンセルされたイベント、およびコンポーネント・スプライスや即時スプライスは無視されます。
func parseTSSpliceInsert(section []byte) (event uint32, pts uint64, out bool, ok bool) {
	if len(section) < 28 || section[0] != 0xfc || section[3] != 0 || section[4]&0x80 != 0 || section[13] != 5 {
		return
	}

	length := int(section[11]&15)<<8 | int(section[12])
	if length == 0xfff || length < 15 || 14+length > len(section)-4 {
		return
	}

	end := 14 + length
	if end+2 > len(section)-4 || end+2+int(binary.BigEndian.Uint16(section[end:end+2])) > len(section)-4 {
		return
	}

	command := section[14 : 14+length]
	if command[4]&0x80 != 0 || command[5]&0x40 == 0 || command[5]&0x10 != 0 || command[6]&0x80 == 0 {
		return
	}

	if command[5]&0x20 != 0 && length < 20 {
		return
	}
	event = binary.BigEndian.Uint32(command[:4])
	adjustment := uint64(section[4]&1)<<32 | uint64(binary.BigEndian.Uint32(section[5:9]))
	pts = (uint64(command[6]&1)<<32 | uint64(binary.BigEndian.Uint32(command[7:11]))) + adjustment
	pts &= (1 << 33) - 1
	out, ok = command[5]&0x80 != 0, true
	return
}
