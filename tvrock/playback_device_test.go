package tvrock

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func idlePlaybackManager(t *testing.T, samePC bool) (*playbackManager, string, string) {
	t.Helper()
	peer := "192.0.2.2"
	if samePC {
		peer = "192.0.2.1"
	}
	m := &playbackManager{sessions: map[string]*playbackSession{}, clients: map[string]playbackClient{"alice": {peer: "192.0.2.1", native: true}, "bob": {peer: peer, native: true}}}
	a, err := m.start(nil, "alice")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.start(nil, "bob")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		m.Lock()
		for _, s := range m.sessions {
			s.cancel()
		}
		m.Unlock()
		deadline := time.Now().Add(3 * time.Second)
		for len(m.devices()) > 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
	})
	return m, a, b
}

func TestPlaybackIdleDeviceOwnershipAndSelectedFile(t *testing.T) {
	m, a, b := idlePlaybackManager(t, false)
	if len(m.devicesFor("alice")) != 1 || len(m.devicesFor("bob")) != 1 || m.devicesFor("alice")[0].ID != "PBK01" || m.devicesFor("bob")[0].ID != "PBK01" {
		t.Fatal("devices must be private to each Rockbar")
	}
	schedule := tsAnalysisTestSchedule(t)
	m.selectFile("alice", playbackCommand{Session: a, ScheduleID: schedule.ID})
	wait := func(phase string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			m.Lock()
			s := m.sessions[a]
			m.Unlock()
			s.Lock()
			state := s.state
			s.Unlock()
			if state.Phase == phase && state.ScheduleID == schedule.ID {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("waiting %s: %+v", phase, state)
			}
			time.Sleep(time.Millisecond)
		}
	}
	wait("preparing")
	m.Lock()
	other := m.sessions[b]
	m.Unlock()
	other.Lock()
	phase := other.state.Phase
	other.Unlock()
	if phase != "idle" {
		t.Fatal("selecting a file affected another Rockbar")
	}
	m.command("bob", "", []byte(`{"action":"close","session":"`+a+`"}`))
	if len(m.devicesFor("alice")) != 1 {
		t.Fatal("other Rockbar closed private device")
	}
	m.command("alice", "", []byte(`{"action":"stop","session":"`+a+`"}`))
	wait("idle")
	if len(m.devicesFor("alice")) != 1 {
		t.Fatal("stop removed waiting device")
	}
	m.command("alice", "", []byte(`{"action":"close","session":"`+a+`"}`))
	deadline := time.Now().Add(2 * time.Second)
	for len(m.devicesFor("alice")) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(m.devicesFor("alice")) != 0 || len(m.devicesFor("bob")) != 1 {
		t.Fatal("power did not close only selected device")
	}
}

func TestPlaybackIdlePortReservation(t *testing.T) {
	m, a, b := idlePlaybackManager(t, true)
	for i, token := range []string{a, b} {
		uid := []string{"alice", "bob"}[i]
		m.Lock()
		s := m.sessions[token]
		m.Unlock()
		command, _ := json.Marshal(playbackCommand{Action: "join", Session: token, Device: s.device.RegistryCode, Port: 53001, SRT: &WatchSRTOptions{LatencyMS: 120}})
		m.command(uid, "192.0.2.1", command)
		s.Lock()
		count := len(s.receivers)
		s.Unlock()
		if count != 1-i {
			t.Fatal("same PC/port must not be reused across private devices")
		}
	}
}

func TestPlaybackPublishedDeviceListIsJSONAndPrivate(t *testing.T) {
	m, alice, bob := idlePlaybackManager(t, false)
	queues := map[string]*DevConQueue{}
	for _, uid := range []string{"alice", "bob"} {
		queue := &DevConQueue{Com: make(chan string, 32), Done: make(chan struct{})}
		queues[uid] = queue
		deviceController.Lock()
		deviceController.Handler[uid] = queue
		deviceController.Unlock()
	}
	t.Cleanup(func() {
		deviceController.Lock()
		for uid, queue := range queues {
			delete(deviceController.Handler, uid)
			close(queue.Done)
		}
		deviceController.Unlock()
	})
	m.publish()
	for uid, token := range map[string]string{"alice": alice, "bob": bob} {
		select {
		case message := <-queues[uid].Com:
			if !strings.HasPrefix(message, "pbs:") {
				t.Fatalf("unexpected notification: %s", message)
			}
			var devices []RockDevice
			if err := json.Unmarshal([]byte(message[4:]), &devices); err != nil {
				t.Fatalf("published list is not JSON: %v", err)
			}
			if len(devices) != 1 || devices[0].Playback == nil || devices[0].Playback.Session != token || devices[0].Playback.Phase != "idle" {
				t.Fatalf("incorrect private standby device list: %+v", devices)
			}
		case <-time.After(time.Second):
			t.Fatal("device list was not pushed")
		}
	}
}

func TestPlaybackAppStartsSeparateDevicesOnAllRockbars(t *testing.T) {
	schedule := tsAnalysisTestSchedule(t)
	m, _, _ := idlePlaybackManager(t, false)
	tokens, err := m.startOnAllRockbars(schedule)
	if err != nil || len(tokens) != 2 {
		t.Fatalf("start all: %v %v", tokens, err)
	}
	owners := map[string]bool{}
	for _, token := range tokens {
		m.Lock()
		s := m.sessions[token]
		m.Unlock()
		s.Lock()
		owners[s.owner] = true
		if len(s.invited) != 1 || !s.invited[s.owner] || s.state.ScheduleID != schedule.ID {
			t.Error("app playback must have a private device for each Rockbar")
		}
		s.Unlock()
	}
	if !owners["alice"] || !owners["bob"] {
		t.Fatal("app playback omitted a connected Rockbar")
	}
}

func TestPlaybackRecordingListCompletedAndActive(t *testing.T) {
	complete := tsAnalysisTestSchedule(t)
	archived := *complete
	archived.ID = 0
	archived.FileName += ".archived"
	archived.StartTime = time.Now().Unix() - 200
	if _, err := scheduler.Insert(1, &archived); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = rockDB.GetData(1).Exec(`DELETE FROM schedule WHERE filename=?`, archived.FileName) })
	active := tsAnalysisTestSchedule(t)
	pending := tsAnalysisTestSchedule(t)
	for i, s := range []*RecordingSchedule{complete, active, pending} {
		s.StartTime = time.Now().Unix() - int64(100+i)
		s.Complete = i == 0
		if _, err := rockDB.GetData(CurrentSlot).Exec(`UPDATE schedule SET starttime=?,complete=? WHERE id=?`, s.StartTime, boolToInt(s.Complete), s.ID); err != nil {
			t.Fatal(err)
		}
	}
	scheduler.Lock()
	scheduler.CurrentRecording["playback-list-test"] = active
	scheduler.Unlock()
	defer func() { scheduler.Lock(); delete(scheduler.CurrentRecording, "playback-list-test"); scheduler.Unlock() }()
	router := gin.New()
	router.GET("/playback/recordings", APIPlaybackRecordings)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("GET", "/playback/recordings", nil))
	if response.Code != 200 {
		t.Fatal(response.Body.String())
	}
	var list []playbackRecording
	if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	found := map[uint64]playbackRecording{}
	for _, row := range list {
		if row.Slot != CurrentSlot || row.FileName == archived.FileName {
			t.Fatal("archive recording was included")
		}
		found[row.ID] = row
	}
	if _, ok := found[complete.ID]; !ok {
		t.Fatal("completed recording missing")
	}
	if row, ok := found[active.ID]; !ok || !row.Recording {
		t.Fatal("active recording missing flag")
	}
	if _, ok := found[pending.ID]; ok {
		t.Fatal("unfinished inactive schedule was included")
	}
	if found[complete.ID].FileName != complete.FileName {
		t.Fatal("schedule details changed")
	}
}
