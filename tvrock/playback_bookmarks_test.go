package tvrock

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
)

func TestManualPlaybackBookmarksPersistAndSurviveAnalysis(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	for _, at := range []int64{500, 1000, 1800} {
		if _, err := changeManualPlaybackBookmark(CurrentSlot, schedule, at, 2000, false); err != nil {
			t.Fatal(err)
		}
	}
	rows := loadManualPlaybackBookmarks(CurrentSlot, schedule, 2000)
	if len(rows) != 2 || rows[0].PositionMS != 500 || rows[1].PositionMS != 1800 {
		t.Fatalf("bad saved bookmarks/duplicate handling: %+v", rows)
	}
	file, err := os.Open(schedule.FileName)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = analyzeRecordedTS(context.Background(), file, schedule, CurrentSlot, false); err != nil {
		t.Fatal(err)
	}
	if rows = loadManualPlaybackBookmarks(CurrentSlot, schedule, 2000); len(rows) != 2 {
		t.Fatal("reanalysis erased manual bookmarks")
	}
	if _, err = changeManualPlaybackBookmark(CurrentSlot, schedule, 500, 2000, true); err != nil {
		t.Fatal(err)
	}
	rows = loadManualPlaybackBookmarks(CurrentSlot, schedule, 2000)
	if len(rows) != 1 || rows[0].PositionMS != 1800 {
		t.Fatalf("delete not persisted: %+v", rows)
	}
	if _, err = changeManualPlaybackBookmark(CurrentSlot, schedule, -1, 2000, false); err == nil {
		t.Fatal("negative bookmark accepted")
	}
	if _, err = changeManualPlaybackBookmark(CurrentSlot, schedule, 2001, 2000, false); err == nil {
		t.Fatal("bookmark outside file accepted")
	}
}

func TestManualPlaybackBookmarksConcurrentEditsDoNotLoseData(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	var wg sync.WaitGroup
	for _, at := range []int64{10000, 20000, 30000, 40000} {
		wg.Add(1)
		go func(position int64) {
			defer wg.Done()
			if _, err := changeManualPlaybackBookmark(CurrentSlot, schedule, position, 60000, false); err != nil {
				t.Error(err)
			}
		}(at)
	}
	wg.Wait()
	if rows := loadManualPlaybackBookmarks(CurrentSlot, schedule, 60000); len(rows) != 4 {
		t.Fatalf("concurrent bookmark lost: %+v", rows)
	}
}

func TestManualPlaybackBookmarkCommandsValidateSessionAndGeneration(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	m, a, _ := idlePlaybackManager(t, false)
	m.Lock()
	s := m.sessions[a]
	m.Unlock()
	s.Lock()
	s.schedule = *schedule
	s.state.ScheduleID = schedule.ID
	s.state.Phase = "paused"
	s.state.DurationMS = 2000
	s.state.Generation = 5
	s.Unlock()
	command := func(uid, action string, generation uint32, position int64) {
		raw, _ := json.Marshal(playbackCommand{Session: a, Action: action, Generation: generation, PositionMS: &position})
		m.command(uid, "", raw)
	}
	command("bob", "bookmark-add", 5, 1000)
	command("alice", "bookmark-add", 4, 1000)
	if got := loadManualPlaybackBookmarks(CurrentSlot, schedule, 2000); len(got) != 0 {
		t.Fatal("unauthorized or stale bookmark saved")
	}
	command("alice", "bookmark-add", 5, 1000)
	s.Lock()
	rows := s.state.ManualBookmarks
	s.Unlock()
	if len(rows) != 1 || rows[0].PositionMS != 1000 {
		t.Fatalf("saved bookmark not published: %+v", rows)
	}
	command("alice", "bookmark-delete", 5, 1000)
	if got := loadManualPlaybackBookmarks(CurrentSlot, schedule, 2000); len(got) != 0 {
		t.Fatal("deleted bookmark remained")
	}
}
