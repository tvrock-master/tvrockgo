package tvrock

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestWorkerGroupShutdown(t *testing.T) {
	var workers workerGroup
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	if !workers.start(func() { close(started); <-release }) {
		t.Fatal("worker rejected")
	}
	<-started
	workers.stop()
	if workers.start(func() { t.Error("worker started during shutdown") }) {
		t.Fatal("worker accepted during shutdown")
	}
	go func() { workers.wait(); close(finished) }()
	select {
	case <-finished:
		t.Fatal("wait returned before worker finished")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("wait did not finish")
	}
}

func TestSchedulerSendUnblocksOnShutdown(t *testing.T) {
	old := globalContext
	ctx, cancel := context.WithCancel(context.Background())
	globalContext = ctx
	defer func() { cancel(); globalContext = old }()
	p := &Scheduler{chUpdate: make(chan SchedulerRequest)}
	var sends sync.WaitGroup
	for _, fn := range []func(){p.UpdateScheduler, func() { p.Add() }, func() { p.Modify() }, func() { p.Stop(1) }, func() { p.Remove(1) }} {
		sends.Add(1)
		go func(fn func()) { defer sends.Done(); fn() }(fn)
	}
	cancel()
	done := make(chan struct{})
	go func() { sends.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler send blocked during shutdown")
	}
	// Late notifications must not enqueue work or panic after the receiver exits.
	p.queueMu.Lock()
	p.chUpdate = nil
	p.queueMu.Unlock()
	p.UpdateScheduler()
}

func TestFetchTaskCancellationBeforeBoot(t *testing.T) {
	old := globalContext
	ctx, cancel := context.WithCancel(context.Background())
	globalContext = ctx
	defer func() { cancel(); globalContext = old }()
	job := &RecordingJob{BootTime: time.Now().Add(time.Hour), Com: make(chan ScheduleJobCom)}
	done := make(chan struct{})
	go func() { job.ExecFetchProgram(&tasks); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("EPG task ignored shutdown before boot")
	}
}

func TestDeviceSendUnblocksOnDisconnect(t *testing.T) {
	disconnected := make(chan struct{})
	device := &RockDevice{ID: "test", State: StateReady, Com: make(chan RequestMessage), requestDone: disconnected}
	slots := &Slot{Device: map[string]*RockDevice{"test": device}}
	finished := make(chan int, 1)
	go func() { finished <- slots.Send(&RequestMessage{ID: "test", Command: "test"}) }()
	close(disconnected)
	select {
	case sent := <-finished:
		if sent != 0 {
			t.Fatalf("sent to disconnected device: %d", sent)
		}
	case <-time.After(time.Second):
		t.Fatal("sender remained blocked after disconnect")
	}
}

func TestArchiveCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, fn := range []func(context.Context) error{archiveContext, archiveScheduleContext, archiveProgramContext, archiveKeywordReserveContext, archiveCaptionContext, indexingArchiveProgramContext} {
		if err := fn(ctx); err != context.Canceled {
			t.Fatalf("expected cancellation, got %v", err)
		}
	}
}

func TestArchiveCancellationRollsBackAndCanRetry(t *testing.T) {
	oldDB := rockDB
	defer func() { rockDB = oldDB }()
	stores := make([]*sql.DB, 2)
	for i, name := range []string{"current.db", "archive.db"} {
		db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), name)+"?_pragma=busy_timeout(1000)")
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := CreateCaptionTable(db); err != nil {
			t.Fatal(err)
		}
		stores[i] = db
	}
	rockDB = RockDB{Caption: stores}
	insert := `INSERT INTO caption (id,sid,eid,start,pts,lang,clear,x,y,color,colorname,bgcolor,size,cap)
	 VALUES (1,1,1,1,0,0,0,0,0,0,'',0,'','caption')`
	if _, err := stores[0].Exec(insert); err != nil {
		t.Fatal(err)
	}
	// Hold the archive writer so cancellation happens during a real transfer.
	blocker, err := stores[1].Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if _, err := blocker.Exec(insert); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := archiveCaptionContext(ctx); err == nil {
		t.Fatal("transfer succeeded despite locked archive and cancellation")
	}
	if ctx.Err() == nil {
		t.Fatal("transfer returned before cancellation")
	}
	blocker.Rollback()
	var count int
	if err := stores[0].QueryRow("SELECT COUNT(*) FROM caption").Scan(&count); err != nil || count != 1 {
		t.Fatalf("source caption lost: count=%d err=%v", count, err)
	}
	// database/sql may already be rolling back on its cancellation goroutine.
	// In that case our deferred Rollback returns ErrTxDone before the connection
	// is returned to the pool, so wait for cleanup with a bounded deadline.
	deadline := time.Now().Add(5 * time.Second)
	for i, db := range stores {
		for db.Stats().InUse != 0 {
			if time.Now().After(deadline) {
				t.Fatalf("store %d: connection leaked after cancellation: %+v", i, db.Stats())
			}
			time.Sleep(time.Millisecond)
		}
	}
	if err := stores[1].QueryRow("SELECT COUNT(*) FROM caption").Scan(&count); err != nil || count != 0 {
		t.Fatalf("archive not rolled back: count=%d err=%v", count, err)
	}
	if err := archiveCaptionContext(context.Background()); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if err := stores[1].QueryRow("SELECT COUNT(*) FROM caption").Scan(&count); err != nil || count != 1 {
		t.Fatalf("retry did not preserve caption: count=%d err=%v", count, err)
	}
	if err := stores[0].QueryRow("SELECT COUNT(*) FROM caption").Scan(&count); err != nil || count != 0 {
		t.Fatalf("retry did not remove source caption: count=%d err=%v", count, err)
	}
}

func TestCustomCommandHelperProcess(t *testing.T) {
	if os.Getenv("TVROCK_COMMAND_HELPER") != "1" {
		return
	}
	time.Sleep(time.Minute)
}

func TestCustomCommandCancellation(t *testing.T) {
	t.Setenv("TVROCK_COMMAND_HELPER", "1")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	manager := &CustomCommandManager{BusyList: make(map[string]*exec.Cmd)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := manager.execCustomCommandContext(ctx, &CustomCommand{
			AppPath: executable, Options: []string{"-test.run=^TestCustomCommandHelperProcess$"},
		}, "", "", "", 0, 0, 0, "", "")
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for manager.GetBusyCount() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("command did not start")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled command succeeded")
		}
		if manager.GetBusyCount() != 0 {
			t.Fatal("command remained in BusyList")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("command did not stop on cancellation")
	}
}

// Run Close in its own process because it permanently shuts down global stores.
// TestMain creates all six databases in a private temporary directory.
func TestShutdownClosesStoresAfterWorkers(t *testing.T) {
	if os.Getenv("TVROCK_SHUTDOWN_HELPER") != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestShutdownClosesStoresAfterWorkers$", "-test.v")
		cmd.Env = append(os.Environ(), "TVROCK_SHUTDOWN_HELPER=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("shutdown subprocess: %v\n%s", err, out)
		}
		return
	}

	globalContext, globalCancel = context.WithCancel(context.Background())
	cronManager = &CronManager{Cron: cron.New(), CronID: make(map[cron.EntryID]string)}
	cronManager.Start()
	var walFiles []string
	for _, db := range append(append(append([]*sql.DB{}, rockDB.Data...), rockDB.Program...), rockDB.Caption...) {
		if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec("CREATE TABLE shutdown_test (value INTEGER)"); err != nil {
			t.Fatal(err)
		}
		var seq int
		var name, path string
		if err := db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &path); err != nil {
			t.Fatal(err)
		}
		walFiles = append(walFiles, filepath.Clean(path)+"-wal", filepath.Clean(path)+"-shm")
	}
	started, release := make(chan struct{}), make(chan struct{})
	cleanupError := make(chan error, 1)
	serviceWorkers.start(func() {
		close(started)
		<-globalContext.Done()
		<-release
		_, err := rockDB.Data[0].Exec("INSERT INTO shutdown_test VALUES (1)")
		cleanupError <- err
	})
	<-started
	closed := make(chan struct{})
	go func() { Close(); close(closed) }()
	<-globalContext.Done()
	if err := rockDB.Data[0].Ping(); err != nil {
		t.Fatalf("DB closed before worker finished: %v", err)
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish")
	}
	if err := <-cleanupError; err != nil {
		t.Fatalf("worker cleanup used a closed DB: %v", err)
	}
	if err := rockDB.Data[0].Ping(); err == nil {
		t.Fatal("DB still open after shutdown")
	}
	for _, path := range walFiles {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("SQLite sidecar remains: %s (%v)", path, err)
		}
	}
	Close() // Repeated calls must be harmless.
}
