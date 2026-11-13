package tvrock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func testTSCMBoundaryRules() []TSCMBoundaryRule {
	return []TSCMBoundaryRule{
		{SID: 23608, PID: 0x075e, NoPacketSeconds: 3, EndBackPackets: 11},
		{SID: 1040, PID: 0x051f, NoPacketSeconds: 3},
		{SID: 1056, PID: 0x081a, NoPacketSeconds: 3, EndBackPackets: 3},
	}
}

func useTSCMBoundaryRulesForTest(t *testing.T, rules []TSCMBoundaryRule) {
	t.Helper()
	configure.mux.Lock()
	previous := configure.TSCMBoundaryRules
	configure.TSCMBoundaryRules = append([]TSCMBoundaryRule(nil), rules...)
	configure.mux.Unlock()
	t.Cleanup(func() {
		configure.mux.Lock()
		configure.TSCMBoundaryRules = previous
		configure.mux.Unlock()
	})
}

func TestTSPIDCMServiceRules(t *testing.T) {
	useTSCMBoundaryRulesForTest(t, testTSCMBoundaryRules())
	for _, rule := range []struct {
		sid  uint
		pid  uint16
		back int
	}{{23608, 0x075e, 11}, {1040, 0x051f, 0}, {1056, 0x081a, 3}} {
		t.Run(fmt.Sprint(rule.sid), func(t *testing.T) {
			metrics := tsAnalysisMetrics{ServiceID: uint16(rule.sid & 65535)}
			s := newTSMetadataScanner(&metrics, rule.sid)
			send := func(pid uint16, at time.Duration, offset int64) {
				s.observe(testSectionPacket(nil, pid, 0), offset, at, 0, true)
			}
			send(rule.pid+1, 0, 0)
			if len(metrics.Candidates) != 0 {
				t.Fatal("unrelated PID created a boundary")
			}
			for i := 0; i < 20; i++ {
				send(rule.pid, time.Second+time.Duration(i)*100*time.Millisecond, int64(i+1)*188)
			}
			send(0x1fff, 5899*time.Millisecond, 21*188)
			if len(metrics.Candidates) != 1 {
				t.Fatal("stopped before three seconds")
			}
			send(0x1fff, 5900*time.Millisecond, 22*188)
			s.finish(5900 * time.Millisecond)
			rows := metrics.Candidates
			if len(rows) != 2 || rows[0].Kind != "splice-out" || rows[0].TimeNS != int64(time.Second) || rows[0].Offset != 188 ||
				rows[1].Kind != "splice-in" || rows[1].TimeNS != int64(time.Second+time.Duration(19-rule.back)*100*time.Millisecond) || rows[1].Offset != int64(20-rule.back)*188 {
				t.Fatalf("wrong boundaries: %+v", rows)
			}
			if rows[1].Evidence != "service-pid-activity" || rows[1].ConfirmedCM || metrics.CMStatus != "splice-candidates" {
				t.Fatalf("lost heuristic evidence: %+v", metrics)
			}
			send(rule.pid, 6*time.Second, 23*188)
			send(0x1fff, 7*time.Second, 24*188)
			if len(metrics.Candidates) != 3 || metrics.Candidates[2].Kind != "splice-out" {
				t.Fatal(metrics.Candidates)
			}
		})
	}
}

func TestTSPIDCMIncompleteIntervalsAndClock(t *testing.T) {
	useTSCMBoundaryRulesForTest(t, testTSCMBoundaryRules())
	metrics := tsAnalysisMetrics{ServiceID: 1056}
	s := newTSMetadataScanner(&metrics, 1056)
	packet := testSectionPacket(nil, 0x081a, 0)
	s.observe(packet, 0, 0, 0, false)
	packet[1] |= 0x80
	s.observe(packet, 188, time.Second, 0, true)
	if len(metrics.Candidates) != 0 {
		t.Fatal("invalid packet became CM start")
	}
	packet[1] &^= 0x80
	s.observe(packet, 376, 2*time.Second, 0, true)
	s.finish(4 * time.Second)
	if len(metrics.Candidates) != 1 {
		t.Fatal("EOF invented CM end")
	}
	markers := playbackBoundaries(&playbackIndex{duration: 4 * time.Second, metrics: metrics})
	if len(markers) != 1 || markers[0].Kind != "splice-out" || markers[0].PositionMS != 2000 {
		t.Fatalf("unpaired CM start marker lost: %+v", markers)
	}
	s.observe(testSectionPacket(nil, 0x1fff, 0), 564, 5*time.Second, 0, true)
	if len(metrics.Candidates) != 1 {
		t.Fatal("insufficient packet history invented CM end")
	}
	plain := tsAnalysisMetrics{ServiceID: 1040}
	other := newTSMetadataScanner(&plain, 1040)
	packet = testSectionPacket(nil, 0x051f, 0)
	other.observe(packet, 0, time.Second, 0, true)
	other.observe(packet, 188, 4*time.Second, 0, true)
	if len(plain.Candidates) != 3 || plain.Candidates[1].TimeNS != int64(time.Second) || plain.Candidates[2].TimeNS != int64(4*time.Second) {
		t.Fatal(plain.Candidates)
	}
	for _, sid := range []uint{1, 40000, 65536 + 1041, 65536 + 1057} {
		if newTSPIDCMScanner(sid) != nil {
			t.Fatalf("rule leaked to SID %d", sid)
		}
	}
}

func TestTSPIDCMStartForwardPackets(t *testing.T) {
	for _, forward := range []int{0, 1, 3} {
		t.Run(fmt.Sprint(forward), func(t *testing.T) {
			useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{{SID: 77, PID: 0x123, NoPacketSeconds: 3, StartForwardPackets: forward, EndBackPackets: 1}})
			metrics := tsAnalysisMetrics{}
			s := newTSMetadataScanner(&metrics, 77)
			for i := 0; i < 6; i++ {
				at := time.Duration(i+1) * time.Second
				// 別PIDとTEIパケットは「何個後」の個数に含めない。
				s.observe(testSectionPacket(nil, 0x124, 0), int64(i*3)*188, at, 0, true)
				bad := testSectionPacket(nil, 0x123, 0)
				bad[1] |= 0x80
				s.observe(bad, int64(i*3+1)*188, at, 0, true)
				s.observe(testSectionPacket(nil, 0x123, byte(i)), int64(i*3+2)*188, at, 0, true)
				want := 0
				if i >= forward {
					want = 1
				}
				if len(metrics.Candidates) != want {
					t.Fatalf("packet %d: unexpected starts %+v", i, metrics.Candidates)
				}
			}
			s.observe(testSectionPacket(nil, 0x1fff, 0), 18*188, 9*time.Second, 0, true)
			s.finish(9 * time.Second)
			rows := metrics.Candidates
			if len(rows) != 2 || rows[0].Kind != "splice-out" || rows[0].TimeNS != int64(time.Duration(forward+1)*time.Second) || rows[0].Offset != int64(forward*3+2)*188 || rows[1].Kind != "splice-in" || rows[1].TimeNS != int64(5*time.Second) {
				t.Fatalf("incorrect start/end offsets: %+v", rows)
			}
		})
	}
}

func TestTSPIDCMStartForwardResetsAndRetainsUnpairedStart(t *testing.T) {
	useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{{SID: 77, PID: 0x123, NoPacketSeconds: 3, StartForwardPackets: 2, EndBackPackets: 3}})
	metrics := tsAnalysisMetrics{}
	s := newTSMetadataScanner(&metrics, 77)
	packet := testSectionPacket(nil, 0x123, 0)
	s.observe(packet, 0, time.Second, 0, true)
	s.observe(packet, 188, 2*time.Second, 0, true)
	s.finish(2 * time.Second)
	if len(metrics.Candidates) != 0 {
		t.Fatal("EOF invented a start before the configured packet")
	}
	s.observe(packet, 376, 5*time.Second, 0, true) // 3秒の空白後は新しい区間。
	s.observe(packet, 564, 6*time.Second, 0, true)
	if len(metrics.Candidates) != 0 {
		t.Fatal("pending start count leaked across intervals")
	}
	s.observe(packet, 752, 7*time.Second, 0, true)
	s.finish(8 * time.Second)
	markers := playbackBoundaries(&playbackIndex{duration: 8 * time.Second, metrics: metrics})
	if len(markers) != 1 || markers[0].Kind != "splice-out" || markers[0].PositionMS != 7000 {
		t.Fatalf("unpaired shifted start lost: %+v", markers)
	}
}

func TestTSCMBoundaryRulesMatchRecordingServiceID(t *testing.T) {
	useTSCMBoundaryRulesForTest(t, testTSCMBoundaryRules())
	for _, rule := range testTSCMBoundaryRules() {
		for _, band := range []TuningSpace{TuningSpaceTS, TuningSpaceBS, TuningSpaceCS, TuningSpaceOS} {
			sid := uint(band)<<16 | rule.SID
			if got, ok := currentTSCMBoundaryRule(sid); !ok || got != rule {
				t.Fatalf("recording SID %d did not match service %d: %+v", sid, rule.SID, got)
			}
		}
	}
	fullSID := uint(TuningSpaceTS)<<16 | 1040
	base := TSCMBoundaryRule{SID: 1040, PID: 0x051f, NoPacketSeconds: 3}
	exact := TSCMBoundaryRule{SID: fullSID, PID: 0x0700, NoPacketSeconds: 5}
	useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{base, exact})
	if got, ok := currentTSCMBoundaryRule(fullSID); !ok || got != exact {
		t.Fatal("raw rule hid exact service rule")
	}
	if got, ok := currentTSCMBoundaryRule(uint(TuningSpaceBS)<<16 | 1040); !ok || got != base {
		t.Fatal("exact rule leaked into another band")
	}
	useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{exact})
	if _, ok := currentTSCMBoundaryRule(1040); ok {
		t.Fatal("band-specific rule applied to raw service")
	}
}

func TestTSCMBoundaryRecordingEndJobUsesRawServiceRule(t *testing.T) {
	useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{{SID: 1, PID: 256, NoPacketSeconds: 3}})
	schedule := tsAnalysisTestSchedule(t)
	schedule.Sid = uint(TuningSpaceTS)<<16 | 1
	db := rockDB.GetData(CurrentSlot)
	if _, err := db.Exec(`UPDATE schedule SET sid=? WHERE id=?`, schedule.Sid, schedule.ID); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := runTSAnalysisJob(ctx, CurrentSlot, schedule); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(schedule.FileName)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	identity, err := identifyTSFile(f)
	if err != nil {
		t.Fatal(err)
	}
	index, err := readTSAnalysis(ctx, db, schedule, identity)
	if err != nil {
		t.Fatal(err)
	}
	rows := index.metrics.Candidates
	if len(rows) != 1 || rows[0].Kind != "splice-out" || rows[0].Evidence != "service-pid-activity" {
		t.Fatalf("recording-end job did not save CM marker: %+v", rows)
	}
	if markers := playbackBoundaries(index); len(markers) != 1 || markers[0].Kind != "splice-out" {
		t.Fatalf("saved CM marker was not exposed for playback: %+v", markers)
	}
}

func TestTSCMBoundaryRulesTemplateAndRoundTrip(t *testing.T) {
	var conf TvRockConf
	if err := conf.Load(filepath.Join("..", "conf", "tvrock.org.yml")); err != nil {
		t.Fatal(err)
	}
	want := []TSCMBoundaryRule{
		{SID: 1040, PID: 0x051f, NoPacketSeconds: 10},
		{SID: 1056, PID: 0x081a, NoPacketSeconds: 5},
		{SID: 1064, PID: 0x1005, NoPacketSeconds: 5, StartForwardPackets: 3},
		{SID: 23608, PID: 0x075e, NoPacketSeconds: 5, StartForwardPackets: 3, EndBackPackets: 8},
	}
	if !reflect.DeepEqual(conf.TSCMBoundaryRules, want) {
		t.Fatalf("unexpected public rules: %+v", conf.TSCMBoundaryRules)
	}
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := conf.Save(path); err != nil {
		t.Fatal(err)
	}
	var loaded TvRockConf
	if err := loaded.Load(path); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(conf.TSCMBoundaryRules, loaded.TSCMBoundaryRules) {
		t.Fatal("rule fields changed after YAML round trip")
	}
}

func TestTSCMBoundaryRulesValidationOnLoad(t *testing.T) {
	for _, row := range []string{
		"sid: 0, pid: 1, nopacketseconds: 3, endbackpackets: 0",
		"sid: 1, pid: 0, nopacketseconds: 3, endbackpackets: 0",
		"sid: 1, pid: 8191, nopacketseconds: 3, endbackpackets: 0",
		"sid: 1, pid: 65536, nopacketseconds: 3, endbackpackets: 0",
		"sid: 1, pid: 1, nopacketseconds: 0, endbackpackets: 0",
		"sid: 1, pid: 1, nopacketseconds: 86401, endbackpackets: 0",
		"sid: 1, pid: 1, nopacketseconds: 3, startforwardpackets: -1, endbackpackets: 0",
		"sid: 1, pid: 1, nopacketseconds: 3, startforwardpackets: 65536, endbackpackets: 0",
		"sid: 1, pid: 1, nopacketseconds: 3, endbackpackets: -1",
		"sid: 1, pid: 1, nopacketseconds: 3, endbackpackets: 65536",
	} {
		t.Run(row, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yml")
			if err := os.WriteFile(path, []byte("tscmboundaryrules:\n  - {"+row+"}\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var conf TvRockConf
			if err := conf.Load(path); err == nil {
				t.Fatal("invalid CM rule accepted")
			}
		})
	}
	rules := testTSCMBoundaryRules()
	if err := validateTSCMBoundaryRules(append(rules, rules[0])); err == nil {
		t.Fatal("duplicate SID accepted")
	}
}

func TestTSCMBoundaryRulesCustomTimeoutAndDisabled(t *testing.T) {
	useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{{SID: 77, PID: 0x123, NoPacketSeconds: 5, EndBackPackets: 1}})
	metrics := tsAnalysisMetrics{}
	s := newTSMetadataScanner(&metrics, 77)
	s.observe(testSectionPacket(nil, 0x123, 0), 0, time.Second, 0, true)
	s.observe(testSectionPacket(nil, 0x123, 1), 188, 2*time.Second, 0, true)
	s.observe(testSectionPacket(nil, 0x1fff, 0), 376, 6*time.Second, 0, true)
	if len(metrics.Candidates) != 1 {
		t.Fatal("ignored configured five-second timeout")
	}
	s.observe(testSectionPacket(nil, 0x1fff, 0), 564, 7*time.Second, 0, true)
	if len(metrics.Candidates) != 2 || metrics.Candidates[1].Offset != 0 {
		t.Fatal("custom PID/back count not applied")
	}
	if newTSPIDCMScanner(1040) != nil {
		t.Fatal("hard-coded rule survived configuration replacement")
	}
	useTSCMBoundaryRulesForTest(t, nil)
	if newTSPIDCMScanner(77) != nil {
		t.Fatal("empty table did not disable PID detection")
	}
}

func TestTSCMBoundaryRulesInvalidateOnlyAffectedCache(t *testing.T) {
	base := TSCMBoundaryRule{SID: 1, PID: 0x123, NoPacketSeconds: 3}
	useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{base})
	schedule := tsAnalysisTestSchedule(t)
	f, err := os.Open(schedule.FileName)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ctx := context.Background()
	index, err := analyzeRecordedTS(ctx, f, schedule, CurrentSlot, false)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := identifyTSFile(f)
	if err != nil {
		t.Fatal(err)
	}
	db := rockDB.GetData(CurrentSlot)
	useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{{SID: 2, PID: 99, NoPacketSeconds: 5}, base})
	if _, err := readTSAnalysis(ctx, db, schedule, identity); err != nil {
		t.Fatalf("unrelated service invalidated cache: %v", err)
	}
	for _, changed := range []TSCMBoundaryRule{
		{SID: 1, PID: 0x124, NoPacketSeconds: 3},
		{SID: 1, PID: 0x123, NoPacketSeconds: 4},
		{SID: 1, PID: 0x123, NoPacketSeconds: 3, StartForwardPackets: 1},
		{SID: 1, PID: 0x123, NoPacketSeconds: 3, EndBackPackets: 1},
	} {
		useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{changed})
		// 保存時も、現在の設定ではなく解析開始時に使ったルールを保持する。
		if err := saveTSAnalysis(ctx, db, schedule, identity, index, time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := readTSAnalysis(ctx, db, schedule, identity); err == nil {
			t.Fatalf("changed rule reused cache: %+v", changed)
		}
	}
	useTSCMBoundaryRulesForTest(t, nil)
	if _, err := readTSAnalysis(ctx, db, schedule, identity); err == nil {
		t.Fatal("removing rule reused cache")
	}
	useTSCMBoundaryRulesForTest(t, []TSCMBoundaryRule{base})
	if _, err := readTSAnalysis(ctx, db, schedule, identity); err != nil {
		t.Fatalf("original rule could not reuse cache: %v", err)
	}
}
