package tvrock

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPlaybackProgressPersistsThroughAnalysisAndResetsAtEnd(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	savePlaybackPosition(CurrentSlot, schedule, 1500*time.Millisecond, 2*time.Second, false)
	if got := readPlaybackPosition(CurrentSlot, schedule, 2*time.Second); got != 1500*time.Millisecond {
		t.Fatalf("resume: %v", got)
	}
	file, err := os.Open(schedule.FileName)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = analyzeRecordedTS(context.Background(), file, schedule, CurrentSlot, false); err != nil {
		t.Fatal(err)
	}
	if got := readPlaybackPosition(CurrentSlot, schedule, 2*time.Second); got != 1500*time.Millisecond {
		t.Fatalf("analysis overwrote progress: %v", got)
	}
	for _, ended := range []bool{false, true} {
		position := 2 * time.Second
		if ended {
			position = time.Second
		}
		savePlaybackPosition(CurrentSlot, schedule, position, 2*time.Second, ended)
		var saved int64
		if err = rockDB.GetData(CurrentSlot).QueryRow(`SELECT resume_position_ms FROM recording_ts_analysis WHERE schedule_id=?`, schedule.ID).Scan(&saved); err != nil || saved != 0 {
			t.Fatalf("end did not reset: %d %v", saved, err)
		}
	}
}

func TestPlaybackMissingFileIsExcludedAndNonMissingErrorsAreNot(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	if err := os.Remove(schedule.FileName); err != nil {
		t.Fatal(err)
	}
	if _, err := openPlaybackRecording(schedule); err == nil {
		t.Fatal("missing file opened")
	}
	rows, err := playbackRecordings()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == schedule.ID {
			t.Fatal("deleted file appeared in list")
		}
	}
	var deleted int
	if err = rockDB.GetData(CurrentSlot).QueryRow(`SELECT file_deleted FROM recording_ts_analysis WHERE schedule_id=?`, schedule.ID).Scan(&deleted); err != nil || deleted != 1 {
		t.Fatalf("missing flag: %d %v", deleted, err)
	}
	if err = os.WriteFile(schedule.FileName, playbackTestTS(t), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := openPlaybackRecording(schedule)
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if err = rockDB.GetData(CurrentSlot).QueryRow(`SELECT file_deleted FROM recording_ts_analysis WHERE schedule_id=?`, schedule.ID).Scan(&deleted); err != nil || deleted != 0 {
		t.Fatalf("restored file remained excluded: %d %v", deleted, err)
	}
	// Opening a directory fails validation but does not mean the recording was deleted.
	other := tsAnalysisTestSchedule(t)
	other.FileName = t.TempDir()
	if _, err = openPlaybackRecording(other); err == nil {
		t.Fatal("directory accepted as TS")
	}
	var count int
	if err = rockDB.GetData(CurrentSlot).QueryRow(`SELECT count(*) FROM recording_ts_analysis WHERE schedule_id=? AND file_deleted=1`, other.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("non-missing error marked deleted: %d %v", count, err)
	}
}

func TestPlaybackStartsFromSavedPositionAndSavesOnStop(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	savePlaybackPosition(CurrentSlot, schedule, 1500*time.Millisecond, 2*time.Second, false)
	m, token, _ := idlePlaybackManager(t, false)
	m.selectFile("alice", playbackCommand{Session: token, ScheduleID: schedule.ID})
	wait := func(phase string) PlaybackState {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			m.Lock()
			s := m.sessions[token]
			m.Unlock()
			s.Lock()
			state := s.state
			s.Unlock()
			if state.Phase == phase && state.ScheduleID == schedule.ID {
				return state
			}
			time.Sleep(time.Millisecond)
		}
		t.Fatalf("phase %s not reached", phase)
		return PlaybackState{}
	}
	state := wait("preparing")
	if state.PositionMS <= 0 || state.PositionMS > 1500 {
		t.Fatalf("did not resume from saved PCR point: %+v", state)
	}
	m.command("alice", "", []byte(`{"action":"stop","session":"`+token+`"}`))
	wait("idle")
	if got := readPlaybackPosition(CurrentSlot, schedule, 2*time.Second); got.Milliseconds() != state.PositionMS {
		t.Fatalf("stop did not save position: %v expected %d", got, state.PositionMS)
	}
}

func TestPlaybackEOFResetsResume(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	savePlaybackPosition(CurrentSlot, schedule, time.Second, 2*time.Second, false)
	file, err := os.Open(schedule.FileName)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r := &playbackReceiver{uid: "alice", connected: true, ready: true, streaming: true, service: uint32(schedule.Sid), controlAck: 1, lastSeen: time.Now(), queue: make(chan RequestMessage, 16), packets: make(chan []byte, 100)}
	s := &playbackSession{schedule: *schedule, state: PlaybackState{Generation: 1, Slot: CurrentSlot}, fileContext: ctx, invited: map[string]bool{"alice": true}, receivers: map[string]*playbackReceiver{"alice": r}, commands: make(chan playbackCommand, 8)}
	m := &playbackManager{}
	m.playFile(s, file)
	if ctx.Err() != nil {
		t.Fatal("test file did not finish")
	}
	if got := readPlaybackPosition(CurrentSlot, schedule, 2*time.Second); got != 0 {
		t.Fatalf("EOF did not clear progress: %v", got)
	}
}
