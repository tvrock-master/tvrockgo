package tvrock

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/datarhei/gosrt"
)

type playbackCloseProbe struct {
	srt.Conn
	closed bool
}

func TestPlaybackIdleControlAndLegacyFallback(t *testing.T) {
	for _, supported := range []bool{false, true} {
		r := &playbackReceiver{idleSupported: supported, streaming: true, queue: make(chan RequestMessage, 8)}
		s := &playbackSession{state: PlaybackState{Generation: 1, Phase: "paused", DurationMS: 123}, receivers: map[string]*playbackReceiver{"one": r}}
		m := &playbackManager{}
		m.setIdle(s)
		var command struct {
			Action string `json:"action"`
		}
		if err := json.Unmarshal([]byte((<-r.queue).Message), &command); err != nil {
			t.Fatal(err)
		}
		want := "reset"
		if supported {
			want = "idle"
		}
		if command.Action != want || r.streaming || s.state.Phase != "idle" {
			t.Fatalf("incorrect idle transition: %+v %+v", command, s.state)
		}
		if supported {
			s.acknowledgeControl(r, s.state.Generation, "", nil)
			if r.controlAck != 0 {
				t.Fatal("old reset acknowledgment accepted as idle success")
			}
			success := true
			s.acknowledgeControl(r, s.state.Generation, "idle", &success)
			if r.controlAck != s.state.Generation {
				t.Fatal("idle success was not acknowledged")
			}
			s.state.Generation++
			s.control(r, "idle")
			failed := false
			s.acknowledgeControl(r, s.state.Generation, "idle", &failed)
			if r.idleSupported || !r.idleDisabled || r.pendingAction != "reset" {
				t.Fatal("failed idle did not fall back to legacy reset")
			}
		}
	}
}

func (p *playbackCloseProbe) Close() error { p.closed = true; return nil }

func TestPlaybackControlAcknowledgmentAndFallback(t *testing.T) {
	for _, failure := range []string{"failure", "timeout", "missing-success"} {
		t.Run(failure, func(t *testing.T) {
			conn := &playbackCloseProbe{}
			r := &playbackReceiver{connected: true, graphSupported: true, graphReady: true, fastPaused: true, hold: true, streaming: true, conn: conn, queue: make(chan RequestMessage, 4)}
			s := &playbackSession{state: PlaybackState{Generation: 2}, receivers: map[string]*playbackReceiver{"one": r}}
			s.control(r, "pause-direct")
			<-r.queue
			ok, failed := true, false
			s.acknowledgeControl(r, 1, "pause-direct", &ok)
			s.acknowledgeControl(r, 2, "resume-direct", &ok)
			if r.controlAck != 0 {
				t.Fatal("stale or mismatched acknowledgment was accepted")
			}
			switch failure {
			case "failure":
				s.acknowledgeControl(r, 2, "pause-direct", &failed)
			case "missing-success":
				s.acknowledgeControl(r, 2, "pause-direct", nil)
			case "timeout":
				r.controlDeadline = time.Now().Add(-time.Millisecond)
			}
			s.fallbackControls(time.Now(), true)
			var command map[string]any
			json.Unmarshal([]byte((<-r.queue).Message), &command)
			if command["action"] != "pause" || !conn.closed || !r.graphDisabled || r.hold {
				t.Fatalf("did not fall back safely: %+v / %+v", command, r)
			}
			// A late success from the failed operation must not acknowledge fallback.
			s.acknowledgeControl(r, 2, "pause-direct", &ok)
			if r.controlAck != 0 {
				t.Fatal("late direct success acknowledged legacy operation")
			}
			s.acknowledgeControl(r, 2, "", nil)
			if r.controlAck != 2 {
				t.Fatal("legacy acknowledgment lost compatibility")
			}
		})
	}
}

func TestPlaybackDirectPausePreservesConnectionAndQueue(t *testing.T) {
	conn := &playbackCloseProbe{}
	packets := make(chan []byte, 2)
	packets <- []byte("queued TS")
	r := &playbackReceiver{connected: true, graphSupported: true, graphReady: true, fastPaused: true, hold: true, streaming: true, conn: conn, packets: packets, queue: make(chan RequestMessage, 4)}
	s := &playbackSession{state: PlaybackState{Generation: 2}, receivers: map[string]*playbackReceiver{"one": r}}
	ok := true
	s.control(r, "pause-direct")
	s.acknowledgeControl(r, 2, "pause-direct", &ok)
	s.fallbackControls(time.Now().Add(2*time.Second), true)
	if !s.fastResumeReady() || conn.closed || r.packets != packets || len(packets) != 1 {
		t.Fatal("successful pause discarded the existing stream")
	}
	r.graphReady = false
	if s.fastResumeReady() {
		t.Fatal("uninitialized graph considered ready")
	}
	r.graphReady = true
	s.state.Generation++
	s.control(r, "resume-direct")
	s.acknowledgeControl(r, 3, "resume-direct", &ok)
	if r.hold || r.fastPaused || conn.closed || r.controlAck != 3 {
		t.Fatal("resume did not release existing stream")
	}
}
