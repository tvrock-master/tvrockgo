package tvrock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func watchTestManager(t *testing.T) (*remoteWatchManager, *RockDevice) {
	t.Helper()
	old := slot.Device
	oldCurrent := scheduler.CurrentRecording
	d := &RockDevice{ID: "WATCH-TEST", RegistryCode: strings.Repeat("a", 32), State: StateReady, CurrentService: 101, Com: make(chan RequestMessage, 32)}
	slot.Device = map[string]*RockDevice{d.ID: d}
	scheduler.CurrentRecording = map[string]*RecordingSchedule{}
	t.Cleanup(func() { slot.Device = old; scheduler.CurrentRecording = oldCurrent })
	return &remoteWatchManager{sessions: map[string]*watchSession{}, byParent: map[string]string{}}, d
}
func watchTestCommand(m *remoteWatchManager, uid, peer string, c WatchCommand) {
	b, _ := json.Marshal(c)
	m.command(uid, peer, b)
}

func TestRemoteWatchMaxSessions(t *testing.T) {
	configure.mux.RLock()
	old := configure.RemoteWatch
	configure.mux.RUnlock()
	t.Cleanup(func() {
		configure.mux.Lock()
		configure.RemoteWatch = old
		configure.mux.Unlock()
	})
	for _, tc := range []struct {
		name       string
		limit      int
		existing   int
		other      bool
		wantAccept bool
	}{
		{name: "below configured limit", limit: 1, wantAccept: true},
		{name: "at configured limit", limit: 1, existing: 1},
		{name: "other device is independent", limit: 1, existing: 1, other: true, wantAccept: true},
		{name: "above previous limit", limit: 33, existing: 32, wantAccept: true},
		{name: "unset uses default", existing: 32},
		{name: "negative uses default", limit: -1, existing: 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configure.mux.Lock()
			configure.RemoteWatch.MaxSessions = tc.limit
			configure.mux.Unlock()
			m, d := watchTestManager(t)
			parent := d.RegistryCode
			if tc.other {
				parent = "other-device"
			}
			for i := 0; i < tc.existing; i++ {
				m.sessions[strconv.Itoa(i)] = &watchSession{Parent: parent}
			}
			watchTestCommand(m, "owner", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
			if got := len(m.sessions) == tc.existing+1; got != tc.wantAccept {
				t.Fatalf("accepted = %v, want %v (sessions = %d)", got, tc.wantAccept, len(m.sessions))
			}
		})
	}
}

func TestRemoteWatchCaptureDevice(t *testing.T) {
	for _, tc := range []struct {
		name     string
		parent   string
		ready    bool
		stopping bool
		queue    bool
		full     bool
		want     bool
	}{
		{name: "watching", parent: "target", ready: true, queue: true, want: true},
		{name: "other device", parent: "other", ready: true, queue: true},
		{name: "starting", parent: "target", queue: true},
		{name: "stopping", parent: "target", ready: true, stopping: true, queue: true},
		{name: "disconnected", parent: "target", ready: true},
		{name: "full queue", parent: "target", ready: true, queue: true, full: true, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := watchTestManager(t)
			s := &watchSession{Parent: tc.parent, ReceiverReady: tc.ready, Stopping: tc.stopping}
			if tc.queue {
				s.ReceiverQueue = make(chan RequestMessage, 1)
			}
			if tc.full {
				s.ReceiverQueue <- RequestMessage{Command: "ReceiverService"}
			}
			m.sessions["test"] = s
			if got := m.captureDevice("target"); got != tc.want {
				t.Fatalf("handled = %v, want %v", got, tc.want)
			}
			if tc.full {
				if got := <-s.ReceiverQueue; got.Command != "ReceiverService" {
					t.Fatal("pending receiver command was overwritten")
				}
			} else if tc.want {
				if got := <-s.ReceiverQueue; got != (RequestMessage{Command: "Capture"}) {
					t.Fatalf("unexpected capture: %+v", got)
				}
			} else if len(s.ReceiverQueue) != 0 {
				t.Fatal("capture reached an unrelated or unavailable receiver")
			}
		})
	}
}

func TestRemoteWatchCaptureDeviceReachesAllViewers(t *testing.T) {
	m, d := watchTestManager(t)
	for _, token := range []string{"first", "second", "busy"} {
		q := make(chan RequestMessage, 1)
		if token == "busy" {
			q <- RequestMessage{Command: "ReceiverService"}
		}
		m.sessions[token] = &watchSession{Parent: d.RegistryCode, ReceiverReady: true, ReceiverQueue: q}
	}
	if !m.captureDevice(d.RegistryCode) {
		t.Fatal("capture was not handled")
	}
	for _, token := range []string{"first", "second"} {
		select {
		case r := <-m.sessions[token].ReceiverQueue:
			if r.Command != "Capture" {
				t.Fatal(r)
			}
		default:
			t.Fatalf("viewer %s did not receive capture", token)
		}
	}
	if len(d.Com) != 0 {
		t.Fatal("capture reached the recording application")
	}
	if m.captureDevice("unknown") {
		t.Fatal("device without viewers must use normal device routing")
	}
}

func TestRemoteWatchScheduleKeepWindow(t *testing.T) {
	now := time.Unix(1700000000, 0)
	cases := []struct {
		name            string
		start, end      int64
		valid, complete bool
		device          string
		want            bool
	}{
		{"boundary", 60, 3600, true, false, "A", true},
		{"outside", 61, 3600, true, false, "A", false},
		{"running", -3600, 60, true, false, "A", true},
		{"ended", -3600, 0, true, false, "A", false},
		{"disabled", 10, 3600, false, false, "A", false},
		{"complete", 10, 3600, true, true, "A", false},
		{"other device", 10, 3600, true, false, "B", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := &RecordingSchedule{DeviceID: c.device, Validate: c.valid, Complete: c.complete, StartTime: now.Unix() + c.start, EndTime: now.Unix() + c.end}
			if got := scheduleKeepsWatchDevice(s, "A", now, 60); got != c.want {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}
	s := &RecordingSchedule{DeviceID: "A", Validate: true, StartTime: now.Unix() - 100, EndTime: now.Unix() - 5, TailTime: 10}
	if !scheduleKeepsWatchDevice(s, "A", now, 60) {
		t.Fatal("active tail must keep device")
	}
	var cfg RemoteWatchSetting
	normalizeRemoteWatchSetting(&cfg)
	if cfg.KeepBeforeScheduleSeconds != 60 || cfg.DisconnectGraceSeconds != 15 {
		t.Fatal(cfg)
	}
}
func TestRemoteWatchWaitsForBothSidesAndKeepsRecordingState(t *testing.T) {
	m, d := watchTestManager(t)
	watchTestCommand(m, "owner", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
	token := m.byParent[d.RegistryCode]
	s := m.sessions[token]
	if !validWatchToken(token) {
		t.Fatal("missing session")
	}
	now := time.Now()
	s.SourceSeen = now
	s.Source.Supported = true
	m.tick(now)
	for len(d.Com) > 0 {
		if (<-d.Com).Command == "StreamStart" {
			t.Fatal("started without receiver readiness")
		}
	}
	s.ReceiverReady = true
	s.ReceiverQueue = make(chan RequestMessage, 8)
	s.ReceiverSeen = now
	m.tick(now.Add(time.Second))
	found := false
	for len(d.Com) > 0 {
		r := <-d.Com
		if r.Command == "StreamStart" {
			found = true
			var p map[string]interface{}
			_ = json.Unmarshal([]byte(r.Message), &p)
			if p["address"] != "192.0.2.2" || p["session"] != token {
				t.Fatal(p)
			}
		}
	}
	if !found {
		t.Fatal("did not start after both sides ready")
	}
	b, _ := json.Marshal(SourceStreamStatus{Session: token, Request: token, Supported: true, Success: true, Enabled: true, Address: s.Address, Port: s.Port})
	m.status(d.RegistryCode, b)
	s.Received = 188
	s.LastReceiveAge = 0
	s.ReceiverService = 101
	d.State = StateRecording
	m.tick(now.Add(2 * time.Second))
	if !s.Established || s.Phase != "receiving" || d.State != StateRecording {
		t.Fatal("view must not replace recording state", s.Phase, d.State)
	}
	d.CurrentService = 202
	m.tick(now.Add(5 * time.Second))
	r := <-s.ReceiverQueue
	if r.Command != "ReceiverService" || r.Message != "202" {
		t.Fatalf("reservation tuning did not follow: %+v", r)
	}
}
func TestRemoteWatchResumeAndStaleStops(t *testing.T) {
	m, d := watchTestManager(t)
	watchTestCommand(m, "old", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
	token := m.byParent[d.RegistryCode]
	s := m.sessions[token]
	m.disconnect("old")
	watchTestCommand(m, "new", "192.0.2.3", WatchCommand{Action: "resume", Session: token})
	if s.Owner != "" {
		t.Fatal("different peer resumed a view")
	}
	watchTestCommand(m, "new", "192.0.2.2", WatchCommand{Action: "resume", Session: token})
	watchTestCommand(m, "old", "192.0.2.2", WatchCommand{Action: "stop", Session: token})
	watchTestCommand(m, "new", "192.0.2.2", WatchCommand{Action: "stop", Session: strings.Repeat("f", 64)})
	if s.Stopping || s.Owner != "new" {
		t.Fatal("old owner or old token stopped the resumed view")
	}
	oldStatus, _ := json.Marshal(SourceStreamStatus{Request: strings.Repeat("f", 64), Success: true})
	m.status(d.RegistryCode, oldStatus)
	if !s.SourceSeen.IsZero() {
		t.Fatal("stale response changed new session")
	}
	watchTestCommand(m, "new", "192.0.2.2", WatchCommand{Action: "stop", Session: token})
	m.tick(time.Now())
	found := false
	for len(d.Com) > 0 {
		r := <-d.Com
		if r.Command == "StreamStop" {
			found = strings.Contains(r.Message, token)
		}
	}
	if !s.Stopping || !found {
		t.Fatal("current owner could not stop view")
	}
}
func TestRemoteWatchRejectsPortConflictsAndUntrustedAddress(t *testing.T) {
	m, d := watchTestManager(t)
	m.command("owner", "192.0.2.2", []byte(`{"action":"start","device":"`+d.RegistryCode+`","port":1234,"address":"192.0.2.99","executable":"evil.exe","options":"& evil"}`))
	s := m.sessions[m.byParent[d.RegistryCode]]
	if s == nil || s.Address != "192.0.2.2" {
		t.Fatal("network payload redirected destination")
	}
	watchTestCommand(m, "other", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
	if len(m.sessions) != 1 {
		t.Fatal("duplicate destination acquired recorder")
	}
	for _, a := range []string{"192.0.2.1:99", "[::ffff:192.0.2.1]:99"} {
		if watchPeer(a) != "192.0.2.1" {
			t.Fatal(a)
		}
	}
	if watchPeer("[::1]:99") != "" || watchPeer("bad") != "" {
		t.Fatal("invalid IPv4 accepted")
	}
}
func TestRemoteWatchSiblingCannotOverwriteRecorder(t *testing.T) {
	m, d := watchTestManager(t)
	// Test through a real loopback websocket, without launching applications.
	watchTestCommand(m, "owner", "127.0.0.1", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
	token := m.byParent[d.RegistryCode]
	r := gin.New()
	r.GET("/dev", m.receiver)
	server := httptest.NewServer(r)
	defer server.Close()
	h := http.Header{"Authorization": []string{"Bearer " + token}, HEADER_CLIENT_TVROCK_DTV_PARENT: []string{d.RegistryCode}}
	c, _, e := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/dev", h)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	var ready RequestMessage
	if c.ReadJSON(&ready) != nil || ready.Command != "Ready" {
		t.Fatal("receiver was not acknowledged")
	}
	_ = c.WriteMessage(websocket.TextMessage, []byte("STA:3"))
	_ = c.WriteMessage(websocket.TextMessage, []byte(`RCV:{"ready":true,"bytes":188,"ageMs":0,"service":101}`))
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		m.Lock()
		seen := m.sessions[token].Received == 188
		m.Unlock()
		if seen {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	m.Lock()
	seen := m.sessions[token].Received == 188
	m.Unlock()
	if !seen || d.State != StateReady || d.CurrentService != 101 {
		t.Fatal("sibling telemetry overwrote recorder or was not received")
	}
	watchTestCommand(m, "owner", "127.0.0.1", WatchCommand{Action: "capture", Session: token})
	var capture RequestMessage
	if c.ReadJSON(&capture) != nil || capture.Command != "Capture" {
		t.Fatal("capture was not routed to receiver")
	}
	if !m.captureDevice(d.RegistryCode) {
		t.Fatal("device capture was not handled")
	}
	if c.ReadJSON(&capture) != nil || capture.Command != "Capture" {
		t.Fatal("device capture was not routed to receiver")
	}
}

func TestRemoteWatchRetriesBootAfterPreviousProcessIsStillExiting(t *testing.T) {
	m, d := watchTestManager(t)
	d.State = StateOFF
	boots := 0
	m.boot = func(message *LaunchMessage) {
		boots++
		if message.RegistryID != d.RegistryCode {
			t.Fatal(message)
		}
	}
	watchTestCommand(m, "owner", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
	s := m.sessions[m.byParent[d.RegistryCode]]
	if boots != 1 {
		t.Fatal("first boot missing")
	}
	now := time.Now().Add(20 * time.Second)
	s.Touched = now
	s.ReceiverSeen = now
	m.tick(now)
	if boots != 2 || s.Stopping || !s.SourceLost.IsZero() {
		t.Fatalf("boot retry/startup deadline failed: boots=%d stopping=%v", boots, s.Stopping)
	}
}

func TestRemoteWatchKeepWindowUsesScheduleBootIncludingPreparation(t *testing.T) {
	now := time.Unix(1700000000, 0)
	s := &RecordingSchedule{DeviceID: "A", Validate: true, StartTime: now.Unix() + 90, IdleTime: 40, EndTime: now.Unix() + 300}
	if !scheduleKeepsWatchDevice(s, "A", now, 60) {
		t.Fatal("preparation starts within keep window")
	}
	s.StartTime = now.Unix() + 101
	if scheduleKeepsWatchDevice(s, "A", now, 60) {
		t.Fatal("preparation starts outside keep window")
	}
}

func TestRemoteWatchRetiredSessionCannotPowerOffNewOwner(t *testing.T) {
	m, d := watchTestManager(t)
	watchTestCommand(m, "new", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
	if m.sendRetirement(d.RegistryCode) || len(d.Com) != 0 {
		t.Fatal("retired session powered off a newly acquired recorder")
	}
	delete(m.sessions, m.byParent[d.RegistryCode])
	delete(m.byParent, d.RegistryCode)
	if !m.sendRetirement(d.RegistryCode) || (<-d.Com).Command != "PowerOff" {
		t.Fatal("unused recorder was not retired")
	}
}

func TestRemoteWatchStreamOptionsAndOneSegReceiver(t *testing.T) {
	m, d := watchTestManager(t)
	options := WatchStreamOptions{OneSeg: true, Caption: true}
	watchTestCommand(m, "owner", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234, Stream: &options})
	token := m.byParent[d.RegistryCode]
	s := m.sessions[token]
	now := time.Now()
	if s.Stream.flags() != 11 {
		t.Fatal(s.Stream)
	}
	s.SourceSeen = now
	s.Source.Supported = true
	s.Source.OptionsSupported = true
	s.ReceiverReady = true
	s.ReceiverSeen = now
	s.ReceiverQueue = make(chan RequestMessage, 8)
	m.tick(now.Add(time.Second))
	found := false
	for len(d.Com) > 0 {
		r := <-d.Com
		if r.Command == "StreamStart" {
			var payload map[string]interface{}
			json.Unmarshal([]byte(r.Message), &payload)
			if payload["flags"] != float64(11) {
				t.Fatal(payload)
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing start")
	}
	s.Source = SourceStreamStatus{Session: token, Supported: true, OptionsSupported: true, Enabled: true, Address: s.Address, Port: s.Port, Flags: 11, Service: 501}
	m.tick(now.Add(4 * time.Second))
	select {
	case request := <-s.ReceiverQueue:
		if request.Command != "ReceiverService" || request.Message != "501" {
			t.Fatal(request)
		}
	default:
		t.Fatal("receiver did not follow one-seg service")
	}
	s.Source.Error = 1168
	m.tick(now.Add(5 * time.Second))
	if !s.Stopping || s.Error == "" {
		t.Fatal("missing one-seg not reported")
	}
}
func TestRemoteWatchNondefaultOptionsRejectLegacySource(t *testing.T) {
	m, d := watchTestManager(t)
	options := WatchStreamOptions{CurrentServiceOnly: true}
	watchTestCommand(m, "owner", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234, Stream: &options})
	s := m.sessions[m.byParent[d.RegistryCode]]
	s.SourceSeen = time.Now()
	s.Source.Supported = true
	s.ReceiverReady = true
	s.ReceiverSeen = time.Now()
	s.ReceiverQueue = make(chan RequestMessage, 8)
	m.tick(time.Now())
	if !s.Stopping || s.Error == "" {
		t.Fatal("legacy source accepted unsupported options")
	}
}

func TestWatchSRTTransport(t *testing.T) {
	for _, tc := range []struct {
		transport string
		options   *WatchSRTOptions
		valid     bool
	}{
		{"", nil, true}, {"udp", nil, true}, {"srt", nil, true}, {"tcp", nil, false},
		{"srt", &WatchSRTOptions{LatencyMS: 0}, true},
		{"srt", &WatchSRTOptions{LatencyMS: 60000, Passphrase: "replace-this-secret"}, true},
		{"srt", &WatchSRTOptions{LatencyMS: -1}, false},
		{"srt", &WatchSRTOptions{LatencyMS: 60001}, false},
		{"srt", &WatchSRTOptions{Passphrase: "short"}, false},
		{"srt", &WatchSRTOptions{Passphrase: strings.Repeat("a", 80)}, false},
		{"srt", &WatchSRTOptions{Passphrase: "secret\nwith-newline"}, false},
		{"srt", &WatchSRTOptions{Passphrase: "日本語のパスフレーズ"}, false},
	} {
		_, _, valid := watchTransport(tc.transport, tc.options)
		if valid != tc.valid {
			t.Errorf("validation for %q: got %v", tc.transport, valid)
		}
	}
	s := &watchSession{Transport: "srt", Token: "test", SRT: &WatchSRTOptions{LatencyMS: 240, Passphrase: "replace-this-secret"}, Port: 3230, Address: "127.0.0.1"}
	command, payload := watchStartRequest(s, 15)
	if command != "StreamStartSRT" || payload["latencyMs"] != 240 || payload["passphrase"] != s.SRT.Passphrase {
		t.Fatal("SRT start request did not preserve options")
	}
	b, err := json.Marshal(s)
	if err != nil || strings.Contains(string(b), "replace-this-secret") || strings.Contains(string(b), "passphrase") {
		t.Fatal("session status leaked passphrase")
	}
	if watchTransportMatches("srt", "") || watchTransportMatches("srt", "udp") || !watchTransportMatches("srt", "srt") || !watchTransportMatches("", "") {
		t.Fatal("transport matching allowed a protocol mismatch")
	}
}

func TestRemoteWatchSRTCapabilityAndStart(t *testing.T) {
	for _, supported := range []bool{false, true} {
		m, d := watchTestManager(t)
		watchTestCommand(m, "owner", "127.0.0.1", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 3230, Transport: "srt", SRT: &WatchSRTOptions{LatencyMS: 240, Passphrase: "replace-this-secret"}})
		var s *watchSession
		for _, session := range m.sessions {
			s = session
		}
		if s == nil {
			t.Fatal("SRT session was not created")
		}
		now := time.Now()
		s.SourceSeen = now
		s.Source = SourceStreamStatus{Supported: true, SRTSupported: supported, MultiSupported: true, OptionsSupported: true}
		s.ReceiverReady = true
		s.ReceiverQueue = make(chan RequestMessage, 8)
		m.tick(now)
		if !supported {
			if !s.Stopping || !strings.Contains(s.Error, "録画アプリ本体") || !strings.Contains(s.Error, "TvRockDTV") {
				t.Fatal("legacy source did not produce an explicit SRT error")
			}
			continue
		}
		found := false
		for len(d.Com) > 0 {
			request := <-d.Com
			if request.Command != "StreamStartSRT" {
				continue
			}
			found = true
			var payload map[string]interface{}
			if json.Unmarshal([]byte(request.Message), &payload) != nil || payload["passphrase"] != "replace-this-secret" || payload["latencyMs"] != float64(240) {
				t.Fatal("SRT request lost its validated options")
			}
		}
		if !found {
			t.Fatal("SRT start request was not routed to the source")
		}
		s.Source = SourceStreamStatus{Supported: true, SRTSupported: true, MultiSupported: true, OptionsSupported: true, Enabled: true, Session: s.Token, Address: s.Address, Port: s.Port, Flags: s.Stream.flags(), Transport: "udp"}
		m.tick(now.Add(time.Second))
		if s.Phase == "streaming" || s.Phase == "receiving" {
			t.Fatal("UDP source was accepted for an SRT session")
		}
		s.Source.Transport = "srt"
		s.Received = 188
		m.tick(now.Add(2 * time.Second))
		if s.Phase != "receiving" || !s.Established {
			t.Fatal("SRT reception was not established")
		}
	}
}

func TestWatchQualityIgnoresStartupAndNeedsSustainedLoss(t *testing.T) {
	var q watchQuality
	r := receiverStreamStatus{Service: 101}
	start := time.Unix(1700000000, 0)
	for i := 0; i < 12; i++ {
		r.Drops += 100
		r.Bytes += 18800
		if q.observe(start.Add(time.Duration(i)*time.Second), r, 101, 0, true) {
			t.Fatalf("startup loss triggered at %d", i)
		}
	}
	r.Bytes += 18800 // A clean interval cancels the earlier loss streak.
	if q.observe(start.Add(12*time.Second), r, 101, 0, true) {
		t.Fatal("isolated loss triggered")
	}
	for i := 13; i <= 15; i++ {
		r.Drops++
		r.Bytes += 18800
		got := q.observe(start.Add(time.Duration(i)*time.Second), r, 101, 0, true)
		if got != (i == 15) {
			t.Fatalf("sustained loss %d: %v", i, got)
		}
	}
	q.lastRequest = start.Add(15 * time.Second)
	if q.observe(start.Add(16*time.Second), r, 101, 0, true) {
		t.Fatal("retried too soon")
	}
	if !q.observe(start.Add(18*time.Second), r, 101, 0, true) {
		t.Fatal("lost control request was not retried")
	}
}

func TestWatchQualityWarmupRestartsOnDiscontinuities(t *testing.T) {
	cases := []string{"source-error", "source-service", "receiver-service", "receiver-epoch", "counter-reset", "reconnect-gap", "not-ready"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			var q watchQuality
			r := receiverStreamStatus{Service: 101, Drops: 500, Bytes: 18800}
			start := time.Unix(1700000000, 0)
			q.observe(start, r, 101, 0, true)
			q.observe(start.Add(10*time.Second), r, 101, 0, true) // Report gap resets the baseline.
			service, errors := uint(101), uint(0)
			eligible := true
			switch name {
			case "source-error":
				errors = 1
			case "source-service":
				service = 202
			case "receiver-service":
				r.Service = 202
			case "receiver-epoch":
				r.Epoch = 1
			case "counter-reset":
				r.Drops = 0
			case "reconnect-gap":
				start = start.Add(5 * time.Second)
			case "not-ready":
				eligible = false
			}
			for i := 11; i < 20; i++ {
				r.Drops++
				r.Bytes += 18800
				if q.observe(start.Add(time.Duration(i)*time.Second), r, service, errors, eligible) {
					t.Fatal("retune/source/startup loss triggered")
				}
			}
		})
	}
}

func TestWatchQualityOnlyChangesTheAffectedLiveSession(t *testing.T) {
	m, d := watchTestManager(t)
	s := &watchSession{Token: watchToken(), Parent: d.RegistryCode, Address: "192.0.2.2", Port: 1234, Established: true}
	s.QualityService = 101
	s.Source = SourceStreamStatus{PacedSupported: true, Enabled: true, Session: s.Token, Address: s.Address, Port: s.Port}
	r := receiverStreamStatus{QualitySupported: true, Ready: true, Service: 101}
	start := time.Now()
	for i := 0; i < 14; i++ {
		r.Drops++
		r.Bytes += 18800
		s.SourceSeen = start.Add(time.Duration(i) * time.Second)
		m.observeWatchQuality(s, r, s.SourceSeen)
	}
	if len(d.Com) != 1 {
		t.Fatalf("commands=%d", len(d.Com))
	}
	request := <-d.Com
	if request.Command != "StreamPace" {
		t.Fatal(request)
	}
	s.Source.Paced = true
	for i := 14; i < 30; i++ {
		r.Drops++
		r.Bytes += 18800
		s.SourceSeen = start.Add(time.Duration(i) * time.Second)
		m.observeWatchQuality(s, r, s.SourceSeen)
	}
	if len(d.Com) != 0 {
		t.Fatal("paced session kept requesting changes")
	}
	s.Source.Paced = false
	s.Stopping = true
	m.observeWatchQuality(s, r, start.Add(31*time.Second))
	if len(d.Com) != 0 {
		t.Fatal("retired session sent a command")
	}
}

func TestWatchQualityRequiresFreshMatchingCapableEndpoints(t *testing.T) {
	for _, name := range []string{"old-receiver", "old-source", "stale-quality", "stale-source", "wrong-session", "wrong-endpoint", "wrong-service", "source-errors"} {
		t.Run(name, func(t *testing.T) {
			m, d := watchTestManager(t)
			s := &watchSession{Token: watchToken(), Parent: d.RegistryCode, Address: "192.0.2.2", Port: 1234, Established: true}
			s.QualityService = 101
			s.Source = SourceStreamStatus{PacedSupported: true, Enabled: true, Session: s.Token, Address: s.Address, Port: s.Port}
			r := receiverStreamStatus{QualitySupported: true, Ready: true, Service: 101}
			start := time.Now()
			for i := 0; i < 25; i++ {
				now := start.Add(time.Duration(i) * time.Second)
				s.SourceSeen = now
				r.Drops++
				r.Bytes += 18800
				switch name {
				case "old-receiver":
					r.QualitySupported = false
				case "old-source":
					s.Source.PacedSupported = false
				case "stale-quality":
					r.QualityAgeMS = 2500
				case "stale-source":
					s.SourceSeen = now.Add(-6 * time.Second)
				case "wrong-session":
					s.Source.Session = watchToken()
				case "wrong-endpoint":
					s.Source.Port = 4321
				case "wrong-service":
					r.Service = 202
				case "source-errors":
					s.QualitySourceErrors++
				}
				m.observeWatchQuality(s, r, now)
			}
			if len(d.Com) > 0 {
				t.Fatal("ineligible feedback changed transport")
			}
		})
	}
}

func TestWatchQualityRockbarReconnectRestartsWarmup(t *testing.T) {
	m, d := watchTestManager(t)
	watchTestCommand(m, "first", "192.0.2.2", WatchCommand{Action: "start", Device: d.RegistryCode, Port: 1234})
	token := m.byParent[d.RegistryCode]
	s := m.sessions[token]
	s.Quality.seen = true
	s.Quality.badIntervals = 2
	m.disconnect("first")
	watchTestCommand(m, "second", "192.0.2.2", WatchCommand{Action: "resume", Session: token})
	if s.Quality.seen || s.Quality.badIntervals != 0 {
		t.Fatal("reconnect retained the old drop streak")
	}
}
