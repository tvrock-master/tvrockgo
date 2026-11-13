package tvrock

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGrowingPlaybackIndexExtendsOnlyCompletePackets(t *testing.T) {
	data := playbackTestTS(t)
	cut := len(data)/2/188*188 + 37
	name := filepath.Join(t.TempDir(), "growing.ts")
	if err := os.WriteFile(name, data[:cut], 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	index, err := indexPlaybackTS(context.Background(), file, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	firstDuration := index.duration
	writer, err := os.OpenFile(name, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err = writer.Write(data[cut : cut+20]); err != nil {
		t.Fatal(err)
	}
	if err = index.extend(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if index.tail.offset != int64(cut/188*188) {
		t.Fatal("indexed incomplete TS packet")
	}
	file.Seek(188, io.SeekStart)
	if _, err = writer.Write(data[cut+20:]); err != nil {
		t.Fatal(err)
	}
	if err = index.extend(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	cursor, _ := file.Seek(0, io.SeekCurrent)
	if cursor != 188 {
		t.Fatal("indexing moved playback cursor")
	}
	complete, err := indexPlaybackTS(context.Background(), file, 1)
	if err != nil {
		t.Fatal(err)
	}
	if index.duration <= firstDuration || index.duration != complete.duration || index.tail.offset != int64(len(data)) {
		t.Fatalf("bad growing index: %v / %v", index.duration, complete.duration)
	}
	for _, target := range []time.Duration{-time.Second, 0, time.Second, index.duration, 100 * time.Hour} {
		point := index.seek(target)
		if point.offset < 0 || point.offset >= int64(len(data)) || point.time > index.duration {
			t.Fatal(point)
		}
	}
	if index.metrics.Packets != 0 || len(index.metrics.Candidates) != 0 || len(index.metrics.ProgramBoundaries) != 0 {
		t.Fatal("growing playback must not perform metadata analysis")
	}
}

func TestPlaybackTailWaitRequiresRecordingEndAndQuietFinalWrites(t *testing.T) {
	var wait playbackTailWait
	now := time.Now()
	if wait.finished(true, 100, now) || wait.finished(true, 100, now.Add(time.Hour)) {
		t.Fatal("active recording reached final EOF")
	}
	if wait.finished(false, 100, now.Add(time.Hour+time.Second)) {
		t.Fatal("did not wait for final writes")
	}
	if wait.finished(false, 200, now.Add(time.Hour+2*time.Second)) {
		t.Fatal("growth did not restart quiet interval")
	}
	if !wait.finished(false, 200, now.Add(time.Hour+4*time.Second)) {
		t.Fatal("stable completed recording did not finish")
	}
}

func TestGrowingPlaybackWaitsForInitialPCRAndCancels(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	scheduler.Lock()
	scheduler.CurrentRecording[t.Name()] = schedule
	scheduler.Unlock()
	defer func() { scheduler.Lock(); delete(scheduler.CurrentRecording, t.Name()); scheduler.Unlock() }()
	if err := os.WriteFile(schedule.FileName, nil, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(schedule.FileName)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := loadGrowingPlaybackTS(ctx, file, schedule); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup wait ignored cancellation")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() { _, err := loadGrowingPlaybackTS(ctx, file, schedule); done <- err }()
	if err := os.WriteFile(schedule.FileName, playbackTestTS(t), 0600); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestGrowingPlaybackWaitAppendStopAndFinish(t *testing.T) {
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "finish", true: "stop"}[stop], func(t *testing.T) {
			schedule := tsAnalysisTestSchedule(t)
			schedule.Complete = false
			data := playbackTestTS(t)
			cut := len(data)/2/188*188 + 37
			if err := os.WriteFile(schedule.FileName, data[:cut], 0600); err != nil {
				t.Fatal(err)
			}
			setActive := func(active bool) {
				scheduler.Lock()
				defer scheduler.Unlock()
				if active {
					scheduler.CurrentRecording[t.Name()] = schedule
				} else {
					delete(scheduler.CurrentRecording, t.Name())
				}
			}
			setActive(true)
			defer setActive(false)
			file, err := openPlaybackRecording(schedule)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			packets := make(chan []byte, 256)
			s := &playbackSession{ctx: ctx, fileContext: ctx, schedule: *schedule, state: PlaybackState{Slot: CurrentSlot, Generation: 1}, commands: make(chan playbackCommand, 4), invited: map[string]bool{"test": true}, receivers: map[string]*playbackReceiver{"test": {connected: true, ready: true, streaming: true, controlAck: 1, packets: packets, service: 1, lastSeen: time.Now()}}}
			m := &playbackManager{}
			done := make(chan struct{})
			go func() { defer close(done); m.playFile(s, file) }()
			defer func() {
				cancel()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("playback did not cancel")
				}
			}()
			await := func(condition func(PlaybackState) bool) {
				t.Helper()
				deadline := time.Now().Add(5 * time.Second)
				for time.Now().Before(deadline) {
					s.Lock()
					state := s.state
					s.Unlock()
					if state.Error != "" {
						t.Fatal(state.Error)
					}
					if condition(state) {
						return
					}
					select {
					case <-done:
						t.Fatal("ended before recording completed")
					case <-time.After(10 * time.Millisecond):
					}
				}
				t.Fatal("timed out waiting for state")
			}
			await(func(state PlaybackState) bool { return state.WaitingForData && state.PositionMS > 0 })
			s.Lock()
			position := s.state.PositionMS
			s.Unlock()
			if stop {
				s.commands <- playbackCommand{Action: "stop"}
			} else {
				writer, err := os.OpenFile(schedule.FileName, os.O_WRONLY|os.O_APPEND, 0600)
				if err != nil {
					t.Fatal(err)
				}
				// A partial packet must survive temporary EOF without duplication or loss.
				if _, err = writer.Write(data[cut : cut+20]); err != nil {
					t.Fatal(err)
				}
				time.Sleep(250 * time.Millisecond)
				setActive(false) // final recorder flush can follow the end notification
				if _, err = writer.Write(data[cut+20:]); err != nil {
					t.Fatal(err)
				}
				writer.Close()
			}
			select {
			case <-done:
			case <-time.After(6 * time.Second):
				t.Fatal("playback did not stop/finish")
			}
			var saved int64
			if err := rockDB.GetData(CurrentSlot).QueryRow(`SELECT resume_position_ms FROM recording_ts_analysis WHERE schedule_id=?`, schedule.ID).Scan(&saved); err != nil {
				t.Fatal(err)
			}
			if stop {
				if saved != position {
					t.Fatalf("stop at growing EOF lost position: %d / %d", saved, position)
				}
			} else {
				if saved != 0 {
					t.Fatal("final EOF did not reset resume position")
				}
				var received []byte
				for len(packets) > 0 {
					received = append(received, (<-packets)...)
				}
				if !bytes.Equal(received, data) {
					t.Fatalf("raw TS changed: received=%d expected=%d", len(received), len(data))
				}
			}
		})
	}
}
