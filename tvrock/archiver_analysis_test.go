package tvrock

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func archiveAnalysisStores(t *testing.T) []*sql.DB {
	t.Helper()
	stores := make([]*sql.DB, 2)
	for i, name := range []string{"current.db", "archive.db"} {
		db, err := rockDB.initDataDB(filepath.Join(t.TempDir(), name))
		if err != nil {
			t.Fatal(err)
		}
		stores[i] = db
		t.Cleanup(func() { db.Close() })
	}
	previous := rockDB.Data
	rockDB.Data = stores
	t.Cleanup(func() { rockDB.Data = previous })
	return stores
}

func archiveAnalysisSchedule(t *testing.T, end int64, complete, analysis bool) uint64 {
	t.Helper()
	schedule := &RecordingSchedule{FileName: t.Name() + "-" + strconv.FormatInt(end, 10) + ".ts", StartTime: end - 60, EndTime: end, Complete: complete}
	if _, err := scheduler.Insert(CurrentSlot, schedule); err != nil {
		t.Fatal(err)
	}
	var id uint64
	if err := rockDB.GetData(CurrentSlot).QueryRow(`SELECT id FROM schedule WHERE filename=? ORDER BY id DESC LIMIT 1`, schedule.FileName).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if !analysis {
		return id
	}
	columns := tsAnalysisColumns + "," + tsPlaybackColumns
	names := strings.Split(columns, ",")
	values := make([]any, len(names))
	for i, name := range names {
		switch name {
		case "schedule_id":
			values[i] = id
		case "file_name":
			values[i] = schedule.FileName
		case "fingerprint":
			values[i] = "test-hash"
		case "status":
			values[i] = "ready"
		case "cm_status":
			values[i] = "splice-candidates"
		case "streams_json":
			values[i] = `[{"pid":256,"streamType":2}]`
		case "seek_points_json":
			values[i] = `[{"offset":0,"timeNs":0}]`
		case "cm_candidates_json":
			values[i] = `[{"timeNs":500000000,"kind":"splice-out"}]`
		case "program_boundaries_json":
			values[i] = `[{"timeNs":800000000,"kind":"program-change","eventId":22,"previousEventId":21}]`
		case "manual_bookmarks_json":
			values[i] = `[1000,1500]`
		case "error":
			values[i] = "preserve-analysis-details"
		case "file_deleted":
			values[i] = 1
		default:
			values[i] = int64(100 + i)
		}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	if _, err := rockDB.GetData(CurrentSlot).Exec(`INSERT INTO recording_ts_analysis (`+columns+`) VALUES (`+placeholders+`)`, values...); err != nil {
		t.Fatal(err)
	}
	return id
}

func archivedAnalysisValues(t *testing.T, db *sql.DB, id uint64) []any {
	t.Helper()
	columns := tsAnalysisColumns + "," + tsPlaybackColumns
	values := make([]any, len(strings.Split(columns, ",")))
	pointers := make([]any, len(values))
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := db.QueryRow(`SELECT `+columns+` FROM recording_ts_analysis WHERE schedule_id=?`, id).Scan(pointers...); err != nil {
		t.Fatal(err)
	}
	return values
}

func assertArchiveRowCount(t *testing.T, db *sql.DB, table string, id uint64, want int) {
	t.Helper()
	key := "id"
	if table == "recording_ts_analysis" {
		key = "schedule_id"
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE `+key+`=?`, id).Scan(&count); err != nil || count != want {
		t.Fatalf("%s/%d count=%d want=%d: %v", table, id, count, want, err)
	}
}

func TestArchiveScheduleMovesAllAnalysisColumnsWithSameCutoff(t *testing.T) {
	stores := archiveAnalysisStores(t)
	cutoff := Now().Unix() - configure.CurrentProgramThreshold
	expired := archiveAnalysisSchedule(t, cutoff-120, true, true)
	incomplete := archiveAnalysisSchedule(t, cutoff-180, false, true)
	withoutAnalysis := archiveAnalysisSchedule(t, cutoff-240, true, false)
	retained := archiveAnalysisSchedule(t, cutoff+120, true, true)
	before := map[uint64][]any{}
	for _, id := range []uint64{expired, incomplete, retained} {
		before[id] = archivedAnalysisValues(t, stores[0], id)
	}
	// The archive must remove source metadata even without the optional trigger.
	if _, err := stores[0].Exec(`DROP TRIGGER schedule_delete_ts_analysis`); err != nil {
		t.Fatal(err)
	}
	if err := archiveScheduleContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uint64{expired, incomplete} {
		assertArchiveRowCount(t, stores[0], "schedule", id, 0)
		assertArchiveRowCount(t, stores[0], "recording_ts_analysis", id, 0)
		assertArchiveRowCount(t, stores[1], "schedule", id, 1)
		if got := archivedAnalysisValues(t, stores[1], id); !reflect.DeepEqual(got, before[id]) {
			t.Fatalf("analysis columns changed during move: %v vs %v", got, before[id])
		}
	}
	assertArchiveRowCount(t, stores[0], "schedule", withoutAnalysis, 0)
	assertArchiveRowCount(t, stores[1], "schedule", withoutAnalysis, 1)
	assertArchiveRowCount(t, stores[1], "recording_ts_analysis", withoutAnalysis, 0)
	assertArchiveRowCount(t, stores[0], "schedule", retained, 1)
	assertArchiveRowCount(t, stores[1], "schedule", retained, 0)
	assertArchiveRowCount(t, stores[1], "recording_ts_analysis", retained, 0)
	if got := archivedAnalysisValues(t, stores[0], retained); !reflect.DeepEqual(got, before[retained]) {
		t.Fatal("retained analysis changed")
	}
}

func TestArchiveScheduleAnalysisFailureRollsBackAndCanRetry(t *testing.T) {
	for _, failure := range []string{"archive-insert", "source-delete"} {
		t.Run(failure, func(t *testing.T) {
			stores := archiveAnalysisStores(t)
			id := archiveAnalysisSchedule(t, Now().Unix()-configure.CurrentProgramThreshold-120, true, true)
			before := archivedAnalysisValues(t, stores[0], id)
			db := stores[1]
			trigger := `CREATE TRIGGER fail_analysis BEFORE INSERT ON recording_ts_analysis BEGIN SELECT RAISE(ABORT,'test archive failure'); END`
			if failure == "source-delete" {
				db = stores[0]
				trigger = `CREATE TRIGGER fail_analysis BEFORE DELETE ON recording_ts_analysis BEGIN SELECT RAISE(ABORT,'test source failure'); END`
			}
			if _, err := db.Exec(trigger); err != nil {
				t.Fatal(err)
			}
			if err := archiveScheduleContext(context.Background()); err == nil {
				t.Fatal("archive succeeded despite injected failure")
			}
			assertArchiveRowCount(t, stores[0], "schedule", id, 1)
			assertArchiveRowCount(t, stores[1], "schedule", id, 0)
			assertArchiveRowCount(t, stores[1], "recording_ts_analysis", id, 0)
			if got := archivedAnalysisValues(t, stores[0], id); !reflect.DeepEqual(got, before) {
				t.Fatal("source analysis lost on failure")
			}
			if _, err := db.Exec(`DROP TRIGGER fail_analysis`); err != nil {
				t.Fatal(err)
			}
			if err := archiveScheduleContext(context.Background()); err != nil {
				t.Fatalf("retry: %v", err)
			}
			assertArchiveRowCount(t, stores[0], "schedule", id, 0)
			assertArchiveRowCount(t, stores[0], "recording_ts_analysis", id, 0)
			if got := archivedAnalysisValues(t, stores[1], id); !reflect.DeepEqual(got, before) {
				t.Fatal("retry changed analysis")
			}
		})
	}
}

func TestArchiveSchedulePreservesConcurrentPlaybackPosition(t *testing.T) {
	stores := archiveAnalysisStores(t)
	if _, err := stores[0].Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	id := archiveAnalysisSchedule(t, Now().Unix()-configure.CurrentProgramThreshold-120, true, true)
	blocker, err := stores[1].Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocker.Exec(`INSERT INTO migration(version) VALUES('test writer lock')`); err != nil {
		blocker.Rollback()
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan error, 1)
	finished := false
	go func() { done <- archiveScheduleContext(ctx) }()
	defer func() {
		blocker.Rollback()
		cancel()
		if !finished {
			<-done
		}
	}()
	deadline := time.Now().Add(3 * time.Second)
	for stores[1].Stats().InUse < 2 {
		if time.Now().After(deadline) {
			t.Fatal("archive transaction did not begin")
		}
		time.Sleep(time.Millisecond)
	}
	// WAL allows this update while the archive holds its source read snapshot.
	if _, err = stores[0].Exec(`UPDATE recording_ts_analysis SET resume_position_ms=987654 WHERE schedule_id=?`, id); err != nil {
		t.Fatal(err)
	}
	blocker.Rollback()
	err = <-done
	finished = true
	if err != nil {
		// A stale SQLite snapshot must abort rather than delete the newer position.
		assertArchiveRowCount(t, stores[0], "schedule", id, 1)
		assertArchiveRowCount(t, stores[1], "schedule", id, 0)
		if err = archiveScheduleContext(context.Background()); err != nil {
			t.Fatalf("retry after concurrent update: %v", err)
		}
	}
	var position int64
	if err = stores[1].QueryRow(`SELECT resume_position_ms FROM recording_ts_analysis WHERE schedule_id=?`, id).Scan(&position); err != nil || position != 987654 {
		t.Fatalf("concurrent playback position lost: %d %v", position, err)
	}
	assertArchiveRowCount(t, stores[0], "schedule", id, 0)
	assertArchiveRowCount(t, stores[0], "recording_ts_analysis", id, 0)
}
