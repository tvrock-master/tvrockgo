package tvrock

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type timeMachineCancelReader struct {
	cancel context.CancelFunc
	reads  int
}

func (r *timeMachineCancelReader) Read(p []byte) (int, error) {
	r.reads++
	switch r.reads {
	case 1:
		return copy(p, "100,200,0,1,0,0,first\n"), nil
	case 2:
		r.cancel()
		return copy(p, "200,300,0,2,0,0,second\n"), nil
	default:
		return 0, io.EOF
	}
}

func TestTimeMachineCanceledBeforeImport(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := TimeMachineTableScan(ctx, filepath.Join(t.TempDir(), "missing")); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v; want context.Canceled before accessing the input", err)
	}
}

func TestTimeMachineCancellationPreservesCommittedPrograms(t *testing.T) {
	sta := &TvStation{Tsp: uint(TuningSpaceTS), Sid: 61001}
	sid := sta.Tsp<<16 | sta.Sid
	t.Cleanup(func() { rockDB.GetProgram(1).Exec("DELETE FROM program WHERE sid=?", sid) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &timeMachineCancelReader{cancel: cancel}
	_, err := scanTimeMachinePrograms(ctx, reader, sta)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v; want context.Canceled", err)
	}
	var count int
	if err := rockDB.GetProgram(1).QueryRow("SELECT COUNT(*) FROM program WHERE sid=? AND title='first'", sid).Scan(&count); err != nil || count != 1 {
		t.Fatalf("committed program: count=%d, error=%v", count, err)
	}
	if err := rockDB.GetProgram(1).QueryRow("SELECT COUNT(*) FROM program WHERE sid=? AND title='second'", sid).Scan(&count); err != nil || count != 0 {
		t.Fatalf("program after cancellation: count=%d, error=%v", count, err)
	}
}

func TestTimeMachineImportsBothDirectories(t *testing.T) {
	dir := t.TempDir()
	stationName := "test-station"
	if err := os.WriteFile(filepath.Join(dir, "dtv.ini"), []byte("[TEST_SERVICE]\n"+stationName+"=61002\n[TEST_CHANNEL]\n"+stationName+"=16625\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for i, base := range []string{filepath.Join(dir, "timemachine"), dir} {
		stationDir := filepath.Join(base, stationName)
		if err := os.MkdirAll(stationDir, 0700); err != nil {
			t.Fatal(err)
		}
		line := "100,200,0,1,0,0,archive\n"
		if i == 1 {
			line = "200,300,0,2,0,0,recent\n"
		}
		if err := os.WriteFile(filepath.Join(stationDir, "program.txt"), []byte(line), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sid := uint(TuningSpaceBS)<<16 | 61002
	t.Cleanup(func() { rockDB.GetProgram(1).Exec("DELETE FROM program WHERE sid=?", sid) })
	if err := TimeMachineTableScan(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := rockDB.GetProgram(1).QueryRow("SELECT COUNT(*) FROM program WHERE sid=?", sid).Scan(&count); err != nil || count != 2 {
		t.Fatalf("imported programs: count=%d, error=%v", count, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dtv.ini.tmp.ini")); !os.IsNotExist(err) {
		t.Fatalf("temporary INI was not removed: %v", err)
	}
}

func TestTimeMachineReplaceCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := tvprogram.ReplaceContext(ctx, 1, &TvProgram{Sid: 61003})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v; want context.Canceled", err)
	}
}

func TestTimeMachineContentLineBreaks(t *testing.T) {
	sta := &TvStation{Tsp: uint(TuningSpaceTS), Sid: 61004}
	sid := sta.Tsp<<16 | sta.Sid
	t.Cleanup(func() { rockDB.GetProgram(1).Exec("DELETE FROM program WHERE sid=?", sid) })
	for _, tt := range []struct{ name, content, want string }{
		{"line breaks", "first+second", "first\r\nsecond"},
		// HTML is escaped when rendered; the database preserves the original text.
		{"empty lines and HTML", "+first++<second>&+", "\r\nfirst\r\n\r\n<second>&\r\n"},
		{"HTML entities", "&lt;second&gt;&amp;", "&lt;second&gt;&amp;"},
		{"no line breaks", "unchanged", "unchanged"},
		{"empty", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			const wantTitle = "title+<tag>&\"'&amp;"
			const wantSubtitle = "subtitle+<tag>&\"'&amp;"
			input := "100,200,0,1,0,0," + wantTitle + "," + wantSubtitle + "," + tt.content + "\n"
			if _, err := scanTimeMachinePrograms(context.Background(), strings.NewReader(input), sta); err != nil {
				t.Fatal(err)
			}
			var title, subtitle, content string
			err := rockDB.GetProgram(1).QueryRow("SELECT title,subtitle,content FROM program WHERE sid=? AND start=100", sid).Scan(&title, &subtitle, &content)
			if err != nil {
				t.Fatal(err)
			}
			if content != tt.want || title != wantTitle || subtitle != wantSubtitle {
				t.Fatalf("got title=%q, subtitle=%q, content=%q; want content=%q", title, subtitle, content, tt.want)
			}
		})
	}
}

func TestTimeMachineStationWorkers(t *testing.T) {
	previous := runtime.GOMAXPROCS(16)
	defer runtime.GOMAXPROCS(previous)
	for _, mode := range []string{"complete", "cancel", "error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stations := make(map[string]*TvStation)
			for i := range 12 {
				stations[fmt.Sprint(i)] = &TvStation{Sid: uint(i + 1)}
			}
			started := make(chan struct{}, len(stations))
			release := make(chan struct{})
			var active, peak, completed atomic.Int32
			failure := errors.New("station read failed")
			done := make(chan error, 1)
			go func() {
				done <- runTimeMachineStations(ctx, stations, func(ctx context.Context, sta *TvStation) error {
					n := active.Add(1)
					defer active.Add(-1)
					for p := peak.Load(); n > p; p = peak.Load() {
						if peak.CompareAndSwap(p, n) {
							break
						}
					}
					started <- struct{}{}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-release:
					}
					if mode == "error" {
						return failure
					}
					completed.Add(1)
					return nil
				})
			}()
			for range timeMachineMaxWorkers {
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("station readers did not start concurrently")
				}
			}
			if mode == "cancel" {
				cancel()
			} else {
				close(release)
			}
			err := <-done
			if (mode == "complete" && err != nil) || (mode == "cancel" && !errors.Is(err, context.Canceled)) || (mode == "error" && !errors.Is(err, failure)) {
				t.Fatalf("mode=%s, error=%v", mode, err)
			}
			if active.Load() != 0 || peak.Load() != timeMachineMaxWorkers {
				t.Fatalf("active=%d, peak=%d", active.Load(), peak.Load())
			}
			if mode == "complete" && completed.Load() != int32(len(stations)) {
				t.Fatalf("completed=%d; want %d", completed.Load(), len(stations))
			}
		})
	}
}

func TestTimeMachineParallelImportBatches(t *testing.T) {
	dir := t.TempDir()
	stations := make(map[string]*TvStation)
	for i := range 3 {
		sta := &TvStation{Tsp: uint(TuningSpaceTS), Sid: uint(61010 + i), Name: fmt.Sprintf("station-%d", i)}
		stations[sta.Name] = sta
		sid := sta.Tsp<<16 | sta.Sid
		t.Cleanup(func() { rockDB.GetProgram(1).Exec("DELETE FROM program WHERE sid=?", sid) })
		path := filepath.Join(dir, sta.Name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		var data strings.Builder
		for n := range timeMachineBatchSize + 3 {
			fmt.Fprintf(&data, "%d,%d,0,%d,0,0,title,subtitle,first+second\n", n+1, n+2, n+1)
		}
		if err := os.WriteFile(filepath.Join(path, "program.txt"), []byte(data.String()), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := ScanTvProgram(context.Background(), dir, stations); err != nil {
		t.Fatal(err)
	}
	for _, sta := range stations {
		var count int
		err := rockDB.GetProgram(1).QueryRow("SELECT COUNT(*) FROM program WHERE sid=? AND content=?", sta.Tsp<<16|sta.Sid, "first\r\nsecond").Scan(&count)
		if err != nil || count != timeMachineBatchSize+3 {
			t.Fatalf("station=%s, count=%d, error=%v", sta.Name, count, err)
		}
	}
}

func TestTimeMachineBatchRollsBackOnError(t *testing.T) {
	const sid = 61020
	db := rockDB.GetProgram(1)
	_, err := db.Exec("CREATE TRIGGER timemachine_batch_fail BEFORE INSERT ON program WHEN NEW.sid=61020 AND NEW.eid=2 BEGIN SELECT RAISE(ABORT, 'batch failure'); END")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec("DROP TRIGGER timemachine_batch_fail")
		db.Exec("DELETE FROM program WHERE sid=?", sid)
	})
	err = tvprogram.ReplaceBatchContext(context.Background(), 1, []TvProgram{{Sid: sid, Eid: 1, Start: 1}, {Sid: sid, Eid: 2, Start: 2}})
	if err == nil {
		t.Fatal("batch write error was discarded")
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM program WHERE sid=?", sid).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial batch was committed: count=%d, error=%v", count, err)
	}
}

func TestTimeMachineReimportUpdatesParentStation(t *testing.T) {
	const parentSID = uint(1<<16 | 62000)
	const childSID = parentSID + 1
	station.Lock()
	previous := station.Cache
	station.Cache = map[uint]*TvStation{
		parentSID: {Sid: parentSID, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1},
		childSID:  {Sid: childSID, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1, Parent: parentSID},
	}
	station.Unlock()
	t.Cleanup(func() {
		station.Lock()
		station.Cache = previous
		station.Unlock()
		rockDB.GetProgram(1).Exec("DELETE FROM program WHERE sid IN (?,?)", parentSID, childSID)
	})
	for sid, content := range map[uint]string{parentSID: "old++content", childSID: "child content"} {
		if err := tvprogram.Replace(1, &TvProgram{Sid: sid, Eid: 1, Start: 100, End: 200, Content: content}); err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dtv.ini"), []byte("[TUNER1 CHANNEL]\nlegacy-station=27\n"), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "timemachine", "legacy-station")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "program.tm"), []byte("100,200,0,1,0,0,title,subtitle,new++content\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := TimeMachineTableScan(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	for sid, want := range map[uint]string{parentSID: "new\r\n\r\ncontent", childSID: "child content"} {
		var got string
		if err := rockDB.GetProgram(1).QueryRow("SELECT content FROM program WHERE sid=? AND start=100", sid).Scan(&got); err != nil || got != want {
			t.Fatalf("sid=%d: got %q, error=%v; want %q", sid, got, err, want)
		}
	}
}

func TestTimeMachineOfflineImportOnlyOpensRequiredStores(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Mkdir("conf", 0700); err != nil {
		t.Fatal(err)
	}
	conf := &TvRockConf{
		DBFile:               filepath.Join(dir, "stations.db"),
		ArchiveDBFile:        filepath.Join(dir, "archive.db"),
		ProgramDBFile:        filepath.Join(dir, "unused", "program.db"),
		CaptionDBFile:        filepath.Join(dir, "unused", "captions.db"),
		PastDBFile:           filepath.Join(dir, "unused", "past.db"),
		CaptionArchiveDBFile: filepath.Join(dir, "unused", "caption-archive.db"),
		BleveFile:            filepath.Join(dir, "unused", "search"),
		Host:                 "invalid-host-for-offline-import",
		BootWebServer:        true,
		BootLauncher:         true,
		BootScheduler:        true,
		BootArchiver:         true,
		Bleve:                true,
		PowerManagerSetting:  PowerManagerSetting{PowerManager: true},
		SqliteSetting:        []string{"_pragma=busy_timeout(5000)"},
	}
	if err := conf.Save(TvRockConfigureFile); err != nil {
		t.Fatal(err)
	}
	beforeConfig, err := os.ReadFile(TvRockConfigureFile)
	if err != nil {
		t.Fatal(err)
	}
	data, err := sql.Open("sqlite", conf.DBFile)
	if err != nil {
		t.Fatal(err)
	}
	defer data.Close()
	if err := CreateTable(data); err != nil {
		t.Fatal(err)
	}
	stations := &Station{db: &RockDB{Data: []*sql.DB{data, nil}}}
	if err := stations.InsertStation(0, &TvStation{Sid: 62001, Tsp: uint(TuningSpaceTS), Ch: 14, View: 1, Name: "test-station"}); err != nil {
		t.Fatal(err)
	}
	archive, err := sql.Open("sqlite", conf.ArchiveDBFile)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	if err := CreateProgramTable(archive); err != nil {
		t.Fatal(err)
	}
	store := &TvProgramStore{db: &RockDB{Program: []*sql.DB{nil, archive}}}
	if err := store.Replace(1, &TvProgram{Sid: 62002, Start: 1, End: 2, Title: "preserve old program"}); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "input")
	path := filepath.Join(source, "timemachine", "test-station")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "dtv.ini"), []byte("[TUNER1 CHANNEL]\ntest-station=27\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "program.tm"), []byte("100,200,0,1,0,0,title,subtitle,first++second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	beforeCron, beforeContext, beforeHost := cronManager, globalContext, configure.Host
	if err := ImportTimeMachine(context.Background(), source); err != nil {
		t.Fatal(err)
	}
	if cronManager != beforeCron || globalContext != beforeContext || configure.Host != beforeHost {
		t.Fatal("offline import modified normal service state")
	}
	if _, err := os.Stat(filepath.Join(dir, "unused")); !os.IsNotExist(err) {
		t.Fatalf("unrelated databases/search index were initialized: %v", err)
	}
	afterConfig, err := os.ReadFile(TvRockConfigureFile)
	if err != nil || string(afterConfig) != string(beforeConfig) {
		t.Fatalf("offline import rewrote configuration: %v", err)
	}
	var content string
	if err := archive.QueryRow("SELECT content FROM program WHERE sid=? AND start=100", uint(TuningSpaceTS)<<16|62001).Scan(&content); err != nil || content != "first\r\n\r\nsecond" {
		t.Fatalf("imported content=%q, error=%v", content, err)
	}
	var count, no int
	if err := archive.QueryRow("SELECT COUNT(*) FROM program WHERE start=1").Scan(&count); err != nil || count != 1 {
		t.Fatalf("old archive program was removed: count=%d, error=%v", count, err)
	}
	if err := data.QueryRow("SELECT no FROM station WHERE sid=62001").Scan(&no); err != nil || no != 0 {
		t.Fatalf("station metadata was rewritten: no=%d, error=%v", no, err)
	}
}
