package tvrock

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	yaml "gopkg.in/yaml.v2"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecorderTSAnalysisSettingPersistsAndRollsBack(t *testing.T) {
	previous := CurrentRecorderSetting()
	t.Cleanup(func() { configure.mux.Lock(); configure.RecorderSetting = previous; configure.mux.Unlock() })
	t.Chdir(t.TempDir())
	setting := previous
	setting.AnalyzeTSAfterRecording = !previous.AnalyzeTSAfterRecording
	if _, err := updateRecorderSetting(setting); err == nil {
		t.Fatal("missing config directory should fail")
	}
	if CurrentRecorderSetting() != previous {
		t.Fatal("failed save changed live setting")
	}
	if err := os.Mkdir("conf", 0700); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		setting.AnalyzeTSAfterRecording = enabled
		if _, err := updateRecorderSetting(setting); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(TvRockConfigureFile)
		if err != nil {
			t.Fatal(err)
		}
		var saved TvRockConf
		if err = yaml.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.RecorderSetting.AnalyzeTSAfterRecording != enabled || CurrentRecorderSetting().AnalyzeTSAfterRecording != enabled {
			t.Fatal("analysis setting did not persist")
		}
	}
}

func tsAnalysisTestSchedule(t *testing.T) *RecordingSchedule {
	t.Helper()
	path := filepath.Join(t.TempDir(), "recording.ts")
	if err := os.WriteFile(path, playbackTestTS(t), 0600); err != nil {
		t.Fatal(err)
	}
	schedule := &RecordingSchedule{FileName: path, Sid: 1, Title: t.Name(), EndTime: time.Now().Unix() - 1, Complete: true}
	if _, err := scheduler.Insert(CurrentSlot, schedule); err != nil {
		t.Fatal(err)
	}
	if err := rockDB.GetData(CurrentSlot).QueryRow(`SELECT id FROM schedule WHERE filename=?`, path).Scan(&schedule.ID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = scheduler.Delete(CurrentSlot, schedule.ID) })
	return schedule
}

func TestTSAnalysisCacheRoundTripAndInvalidation(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	file, err := os.Open(schedule.FileName)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	index, err := analyzeRecordedTS(context.Background(), file, schedule, CurrentSlot, false)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := identifyTSFile(file)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := readTSAnalysis(context.Background(), rockDB.GetData(CurrentSlot), schedule, identity)
	if err != nil || cached.duration != index.duration || cached.pcrPID != index.pcrPID || len(cached.points) != len(index.points) {
		t.Fatalf("cache did not round trip: %v", err)
	}
	for i := range index.points {
		if index.points[i] != cached.points[i] {
			t.Fatal("seek positions changed")
		}
	}
	// A changed identity, SID, or analyzer version must not reuse old offsets.
	changed := identity
	changed.mtime++
	if _, err = readTSAnalysis(context.Background(), rockDB.GetData(CurrentSlot), schedule, changed); err == nil {
		t.Fatal("changed file reused cache")
	}
	other := *schedule
	other.Sid = 2
	if _, err = readTSAnalysis(context.Background(), rockDB.GetData(CurrentSlot), &other, identity); err == nil {
		t.Fatal("different service reused cache")
	}
	_, err = rockDB.GetData(CurrentSlot).Exec(`UPDATE recording_ts_analysis SET version=version+1 WHERE schedule_id=?`, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readTSAnalysis(context.Background(), rockDB.GetData(CurrentSlot), schedule, identity); err == nil {
		t.Fatal("old analyzer reused cache")
	}
}

func TestTSAnalysisOptionOffScansAndOnUsesCache(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	file, err := os.Open(schedule.FileName)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	index, err := analyzeRecordedTS(context.Background(), file, schedule, CurrentSlot, false)
	if err != nil {
		t.Fatal(err)
	}
	db := rockDB.GetData(CurrentSlot)
	// A valid, intentionally changed cached duration makes reuse observable.
	_, err = db.Exec(`UPDATE recording_ts_analysis SET duration_ns=duration_ns+1000000000 WHERE schedule_id=?`, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Even when another recording is being analyzed, a ready cache is immediate.
	tsAnalysisGate <- struct{}{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	cached, err := analyzeRecordedTS(ctx, file, schedule, CurrentSlot, true)
	cancel()
	<-tsAnalysisGate
	if err != nil || cached.duration != index.duration+time.Second {
		t.Fatalf("ON did not reuse index: %v", err)
	}
	fresh, err := analyzeRecordedTS(context.Background(), file, schedule, CurrentSlot, false)
	if err != nil || fresh.duration != index.duration {
		t.Fatalf("OFF did not scan file: %v", err)
	}
	_, _ = db.Exec(`UPDATE recording_ts_analysis SET seek_points_json='[{"offset":189,"timeNs":0}]' WHERE schedule_id=?`, schedule.ID)
	fresh, err = analyzeRecordedTS(context.Background(), file, schedule, CurrentSlot, true)
	if err != nil || fresh.points[0].offset%188 != 0 {
		t.Fatalf("invalid cached offset was not rebuilt: %v", err)
	}
}

func TestTSSpliceReferenceAndFragmentedSections(t *testing.T) {
	// Public interoperability vector from Comcast/scte35-go README (CRC example).
	// https://github.com/Comcast/scte35-go#crc_32-validation
	section, err := base64.StdEncoding.DecodeString("/DA4AAAAAAAAAP/wFAUABDEAf+//mWEhzP4Azf5gAQAAAAATAhFDVUVJAAAAAX+/AQIwNAEAAKeYO3Q=")
	if err != nil {
		t.Fatal(err)
	}
	id, pts, out, ok := parseTSSpliceInsert(section)
	if !ok || id != 0x43100 || pts != 0x1996121cc || !out {
		t.Fatalf("reference splice mismatch: %x %x %v %v", id, pts, out, ok)
	}
	// The reference intentionally has an invalid CRC; the assembler must reject it.
	packet := bytes.Repeat([]byte{0xff}, 188)
	packet[0] = 0x47
	packet[1] = 0x40
	packet[2] = 100
	packet[3] = 0x10
	packet[4] = 0
	copy(packet[5:], section)
	assembler := tsSectionAssembler{}
	emitted := 0
	assembler.feed(packet, func([]byte) { emitted++ })
	if emitted != 0 {
		t.Fatal("corrupt section accepted")
	}
	// Exercise a valid section spanning multiple TS packets.
	base := testSCTE35Section()
	long := make([]byte, len(base)+200)
	copy(long, base[:len(base)-4])
	binary.BigEndian.PutUint16(long[29:31], 200)
	length := len(long) - 3
	long[1] = 0x30 | byte(length>>8)
	long[2] = byte(length)
	binary.BigEndian.PutUint32(long[len(long)-4:], tsSectionCRC(long[:len(long)-4]))
	assembler = tsSectionAssembler{}
	copy(packet[5:], long[:183])
	assembler.feed(packet, func([]byte) { emitted++ })
	if emitted != 0 {
		t.Fatal("partial section emitted")
	}
	packet = bytes.Repeat([]byte{0xff}, 188)
	packet[0] = 0x47
	packet[1] = 0
	packet[2] = 100
	packet[3] = 0x11
	copy(packet[4:], long[183:])
	assembler.feed(packet, func(got []byte) {
		if !bytes.Equal(got, long) {
			t.Fatal("section changed")
		}
		emitted++
	})
	if emitted != 1 {
		t.Fatal("fragmented section not assembled")
	}
}

func TestTSAnalysisPostRecordingAndDeleteCleanup(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	configure.mux.Lock()
	previous := configure.RecorderSetting.AnalyzeTSAfterRecording
	configure.RecorderSetting.AnalyzeTSAfterRecording = true
	configure.mux.Unlock()
	defer func() {
		configure.mux.Lock()
		configure.RecorderSetting.AnalyzeTSAfterRecording = previous
		configure.mux.Unlock()
	}()
	queueRecordingTSAnalysis(CurrentSlot, schedule.ID)
	var status string
	db := rockDB.GetData(CurrentSlot)
	if err := db.QueryRow(`SELECT status FROM recording_ts_analysis WHERE schedule_id=?`, schedule.ID).Scan(&status); err != nil || status != "queued" {
		t.Fatalf("post recording not queued: %s/%v", status, err)
	}
	if err := runTSAnalysisJob(context.Background(), CurrentSlot, schedule); err != nil {
		t.Fatal(err)
	}
	var packets int64
	if err := db.QueryRow(`SELECT status,packet_count FROM recording_ts_analysis WHERE schedule_id=?`, schedule.ID).Scan(&status, &packets); err != nil || status != "ready" || packets == 0 {
		t.Fatalf("post recording did not save: %s/%d/%v", status, packets, err)
	}
	if err := scheduler.Delete(CurrentSlot, schedule.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM recording_ts_analysis WHERE schedule_id=?`, schedule.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("deleted schedule left orphaned analysis")
	}
}

func TestTSAnalysisArchiveCopiesIndex(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	file, _ := os.Open(schedule.FileName)
	defer file.Close()
	if _, err := analyzeRecordedTS(context.Background(), file, schedule, CurrentSlot, false); err != nil {
		t.Fatal(err)
	}
	// Copying analysis uses the already-copied destination schedule key.
	savePlaybackPosition(CurrentSlot, schedule, time.Second, 2*time.Second, false)
	if _, err := changeManualPlaybackBookmark(CurrentSlot, schedule, 1000, 2000, false); err != nil {
		t.Fatal(err)
	}
	destination := rockDB.GetData(1)
	_, err := destination.Exec(`INSERT INTO schedule(id,deviceid,title,customcomid,filename,validate,watchonly,recordonly,shutdown,complete,epgfollow,autoselect,starttime,endtime,idletime,readytime,tailtime,lastupdate,sid,eid,searchid,priority,exitmode,no,errors) SELECT ?, '', '', '', ?,0,0,0,0,1,0,0,0,0,0,0,0,0,?,0,0,0,0,0,0`, schedule.ID, schedule.FileName, schedule.Sid)
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Exec(`DELETE FROM schedule WHERE id=?`, schedule.ID)
	tx, err := destination.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = copyTSAnalysisForArchive(context.Background(), rockDB.GetData(CurrentSlot), tx, time.Now().Unix()); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	identity, _ := identifyTSFile(file)
	if _, err = readTSAnalysis(context.Background(), destination, schedule, identity); err != nil {
		t.Fatalf("archived index missing: %v", err)
	}
	if got := readPlaybackPosition(1, schedule, 2*time.Second); got != time.Second {
		t.Fatalf("archive lost progress: %v", got)
	}
	if got := loadManualPlaybackBookmarks(1, schedule, 2000); len(got) != 1 || got[0].PositionMS != 1000 {
		t.Fatalf("archive lost manual bookmark: %+v", got)
	}
}

func TestTSAnalysisWorkerResumesAndStops(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	configure.mux.Lock()
	previous := configure.RecorderSetting.AnalyzeTSAfterRecording
	configure.RecorderSetting.AnalyzeTSAfterRecording = true
	configure.mux.Unlock()
	defer func() {
		configure.mux.Lock()
		configure.RecorderSetting.AnalyzeTSAfterRecording = previous
		configure.mux.Unlock()
	}()
	queueRecordingTSAnalysis(CurrentSlot, schedule.ID)
	_, err := rockDB.GetData(CurrentSlot).Exec(`UPDATE recording_ts_analysis SET status='running' WHERE schedule_id=?`, schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); runTSAnalysisWorker(ctx) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(8 * time.Second)
	for {
		var status string
		if err = rockDB.GetData(CurrentSlot).QueryRow(`SELECT status FROM recording_ts_analysis WHERE schedule_id=?`, schedule.ID).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "ready" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("interrupted analysis did not resume: %s", status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("analysis worker did not stop")
	}
}

func testSCTE35Section() []byte {
	section := make([]byte, 35)
	section[0] = 0xfc
	section[1] = 0x30
	section[2] = 32
	section[10] = 0xff
	section[11] = 0xf0
	section[12] = 15
	section[13] = 5
	binary.BigEndian.PutUint32(section[14:18], 42)
	section[19] = 0xc0 // out_of_network + program splice
	section[20] = 0x80
	binary.BigEndian.PutUint32(section[21:25], 90000)
	crc := tsSectionCRC(section[:len(section)-4])
	binary.BigEndian.PutUint32(section[len(section)-4:], crc)
	return section
}

func TestTSOnlySpliceCandidatesAreNotConfirmedCM(t *testing.T) {
	if tsSectionCRC([]byte("123456789")) != 0x0376e6e7 {
		t.Fatal("MPEG-2 CRC vector failed")
	}
	section := testSCTE35Section()
	if tsSectionCRC(section) != 0 {
		t.Fatal("invalid test CRC")
	}
	metrics := tsAnalysisMetrics{Streams: []tsStreamInfo{{PID: 100, Type: 0x86}}}
	scanner := newTSMetadataScanner(&metrics, uint(metrics.ServiceID))
	packet := bytes.Repeat([]byte{0xff}, 188)
	packet[0] = 0x47
	packet[1] = 0x40
	packet[2] = 100
	packet[3] = 0x10
	packet[4] = 0
	copy(packet[5:], section)
	scanner.observe(packet, 188, 0, 0, true)
	scanner.observe(packet, 376, 0, 0, true) // retransmitted PSI must not create duplicates
	scanner.finish(2 * time.Second)
	if len(metrics.Candidates) != 1 || metrics.Candidates[0].TimeNS != int64(time.Second) || metrics.Candidates[0].ConfirmedCM || metrics.CMStatus != "splice-candidates" {
		t.Fatalf("bad splice candidate: %+v", metrics)
	}
	encoded, _ := json.Marshal(metrics.Candidates)
	if !bytes.Contains(encoded, []byte(`"confirmedCM":false`)) {
		t.Fatal("candidate labeled as confirmed CM")
	}
	// Packet corruption and PCR discontinuities alone are never CM markers.
	plain := tsAnalysisMetrics{}
	other := newTSMetadataScanner(&plain, uint(plain.ServiceID))
	packet[1] = 0x80
	packet[2] = 101
	other.observe(packet, 0, 0, 0, true)
	other.finish(time.Second)
	if len(plain.Candidates) != 0 || plain.TEI != 1 {
		t.Fatal("transport error became CM candidate")
	}
}

func TestPlaybackBoundariesExposeSortedCandidateTimes(t *testing.T) {
	index := &playbackIndex{duration: 30 * time.Second, metrics: tsAnalysisMetrics{Candidates: []tsBoundaryCandidate{
		{TimeNS: int64(20 * time.Second), Kind: "splice-in"},
		{TimeNS: int64(10500 * time.Millisecond), Kind: "splice-out"},
		{TimeNS: -1}, {TimeNS: int64(31 * time.Second)},
	}}}
	rows := playbackBoundaries(index)
	if len(rows) != 2 || rows[0].PositionMS != 10500 || rows[0].Kind != "splice-out" || rows[1].PositionMS != 20000 {
		t.Fatalf("invalid playback boundaries: %+v", rows)
	}
	data, err := json.Marshal(PlaybackState{CMStatus: "splice-candidates", CMBoundaries: rows})
	if err != nil || !bytes.Contains(data, []byte(`"cmBoundaries":[{"positionMs":10500,"kind":"splice-out"}`)) {
		t.Fatalf("missing candidate state: %s %v", data, err)
	}
}
