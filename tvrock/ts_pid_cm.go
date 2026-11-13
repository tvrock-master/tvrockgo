package tvrock

import "time"

// PIDの送出開始・停止から得る局別のCM境界候補。時刻は壁時計ではなくPCRを使う。
type tsPIDCMScanner struct {
	pid            uint16
	back           int
	forward        int
	startRemaining int
	timeout        time.Duration
	active         bool
	last           time.Duration
	// 終了位置の補正に必要な対象PIDパケットだけを保持する。
	tail []playbackPoint
}

func newTSPIDCMScanner(sid uint) *tsPIDCMScanner {
	rule, ok := currentTSCMBoundaryRule(sid)
	if !ok {
		return nil
	}
	return &tsPIDCMScanner{pid: rule.PID, back: rule.EndBackPackets, forward: rule.StartForwardPackets, timeout: time.Duration(rule.NoPacketSeconds) * time.Second}
}

func (s *tsPIDCMScanner) boundary(m *tsAnalysisMetrics, point playbackPoint, kind string) {
	m.Candidates = append(m.Candidates, tsBoundaryCandidate{
		TimeNS: int64(point.time), Offset: point.offset, Kind: kind,
		Evidence: "service-pid-activity", ConfirmedCM: false,
	})
}

func (s *tsPIDCMScanner) observe(m *tsAnalysisMetrics, pid uint16, offset int64, elapsed time.Duration, valid bool) {
	// タイムアウト時刻ちょうどに対象PIDが再開した場合も、直前の区間を先に閉じる。
	if s.active && elapsed-s.last >= s.timeout {
		if len(s.tail) == s.back+1 {
			s.boundary(m, s.tail[0], "splice-in")
		}
		s.active = false
		s.tail = s.tail[:0]
	}
	if pid != s.pid || !valid {
		return
	}
	point := playbackPoint{offset: offset, time: elapsed}
	if !s.active {
		s.active = true
		s.startRemaining = s.forward
	}
	// 最初の対象PIDパケットを0個後とし、指定個数に到達した時だけ開始を記録する。
	if s.startRemaining >= 0 {
		if s.startRemaining == 0 {
			s.boundary(m, point, "splice-out")
		}
		s.startRemaining--
	}
	if len(s.tail) == s.back+1 {
		copy(s.tail, s.tail[1:])
		s.tail = s.tail[:s.back]
	}
	s.tail = append(s.tail, point)
	s.last = elapsed
}
