package tvrock

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/asticode/go-astits"
	"github.com/datarhei/gosrt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func playbackTestTS(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	mux := astits.NewMuxer(context.Background(), &output, astits.MuxerOptTablesRetransmitPeriod(1))
	if err := mux.AddElementaryStream(astits.PMTElementaryStream{ElementaryPID: 256, StreamType: astits.StreamTypeMPEG2Video}); err != nil {
		t.Fatal(err)
	}
	mux.SetPCRPID(256)
	for i := 0; i <= 20; i++ {
		_, err := mux.WriteData(&astits.MuxerData{PID: 256, AdaptationField: &astits.PacketAdaptationField{HasPCR: true, PCR: &astits.ClockReference{Base: int64(i) * 9000}},
			PES: &astits.PESData{Header: &astits.PESHeader{StreamID: 0xe0, OptionalHeader: &astits.PESOptionalHeader{PTSDTSIndicator: astits.PTSDTSIndicatorOnlyPTS, PTS: &astits.ClockReference{Base: int64(i) * 9000}}}, Data: bytes.Repeat([]byte{byte(i)}, 300)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	return output.Bytes()
}

func TestPlaybackIndexAndClock(t *testing.T) {
	data := playbackTestTS(t)
	path := filepath.Join(t.TempDir(), "recording.ts")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	index, err := indexPlaybackTS(context.Background(), file, 1)
	if err != nil {
		t.Fatal(err)
	}
	if index.pcrPID != 256 || index.duration != 2*time.Second {
		t.Fatalf("unexpected index: %+v", index)
	}
	point := index.seek(1500 * time.Millisecond)
	if point.time > 1500*time.Millisecond || point.offset%188 != 0 {
		t.Fatalf("bad seek point: %+v", point)
	}
	if data[point.offset] != 0x47 || data[point.offset+1]&31 != 0 || data[point.offset+2] != 0 {
		t.Fatal("seek must start with original PAT")
	}
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := io.ReadAll(file)
	if !bytes.Equal(data, after) {
		t.Fatal("recording was changed")
	}
	const wrap = (int64(1) << 33) * 300
	if delta := playbackPCRDelta(wrap-27000000, 0); delta != time.Second {
		t.Fatal(delta)
	}
	if delta := playbackPCRDelta(27000000, 0); delta != 0 {
		t.Fatalf("backward discontinuity: %v", delta)
	}
	if _, ok := playbackPCR(make([]byte, 188), 256); ok {
		t.Fatal("accepted invalid TS packet")
	}
}

func TestPlaybackIndexIgnoresUnrelatedBroadcastEIT(t *testing.T) {
	// An unrelated EIT has a short-event name length exceeding its descriptor.
	// The old full-stream demuxer rejects it before reaching the valid PAT/PMT.
	section := []byte{0x4e, 0xb0, 0, 0, 1, 0xc1, 0, 0, 0, 1, 0, 1, 0, 0x4e,
		0, 1, 0xea, 0x60, 0, 0, 0, 0, 1, 0, 0x80, 7, 0x4d, 5, 'j', 'p', 'n', 0xff, 0, 0, 0, 0, 0}
	length := len(section) - 3
	section[1] = 0xb0 | byte(length>>8)
	section[2] = byte(length)
	bad := make([]byte, 0, 376)
	for cc := byte(0); cc < 2; cc++ {
		packet := bytes.Repeat([]byte{0xff}, 188)
		copy(packet, []byte{0x47, 0x40, 0x12, 0x10 | cc, 0})
		copy(packet[5:], section)
		bad = append(bad, packet...)
	}
	data := append(bad, playbackTestTS(t)...)
	old := astits.NewDemuxer(context.Background(), bytes.NewReader(data), astits.DemuxerOptPacketSize(188))
	if _, err := old.NextData(); err == nil || !strings.Contains(err.Error(), "Short Event") {
		t.Fatalf("fixture does not reproduce the original EIT error: %v", err)
	}
	path := filepath.Join(t.TempDir(), "broadcast.ts")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	index, err := indexPlaybackTS(context.Background(), file, 1)
	if err != nil {
		t.Fatal(err)
	}
	if index.pcrPID != 256 || index.duration != 2*time.Second {
		t.Fatalf("unexpected index: %+v", index)
	}
	file.Seek(0, io.SeekStart)
	after, _ := io.ReadAll(file)
	if !bytes.Equal(after, data) {
		t.Fatal("broadcast packets were modified")
	}
}

func TestPlaybackIndexReportsMissingService(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.ts")
	if err := os.WriteFile(path, playbackTestTS(t), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	_, err = indexPlaybackTS(context.Background(), file, 999)
	if err == nil || !strings.Contains(err.Error(), "999") || !strings.Contains(err.Error(), "PAT") {
		t.Fatalf("missing service diagnostic: %v", err)
	}
}

func TestPlaybackOnlyActiveDevicesAndOwnerControls(t *testing.T) {
	m := &playbackManager{sessions: map[string]*playbackSession{}, clients: map[string]playbackClient{"owner": {peer: "127.0.0.1", native: true}, "other": {peer: "127.0.0.1", native: true}}}
	if len(m.devices()) != 0 {
		t.Fatal("unused playback devices must be hidden")
	}
	definitions := playbackDefinitions()
	if len(definitions) != 8 {
		t.Fatal("default PBK01-PBK08 missing")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &playbackSession{device: definitions[0], state: PlaybackState{Session: "test", Generation: 1}, owner: "owner", invited: map[string]bool{"owner": true}, receivers: map[string]*playbackReceiver{}, ctx: ctx, cancel: cancel, commands: make(chan playbackCommand, 4)}
	m.sessions["test"] = s
	if devices := m.devices(); len(devices) != 1 || devices[0].Kind != "playback" || devices[0].ID != "PBK01" {
		t.Fatal(devices)
	}
	m.command("other", "127.0.0.1", []byte(`{"action":"stop","session":"test"}`))
	if ctx.Err() != nil {
		t.Fatal("non-participant stopped private playback")
	}
	fileCtx, fileCancel := context.WithCancel(ctx)
	s.fileCancel = fileCancel
	m.command("owner", "127.0.0.1", []byte(`{"action":"stop","session":"test"}`))
	if ctx.Err() != nil || fileCtx.Err() == nil {
		t.Fatal("stop must end only the file")
	}
	m.command("owner", "127.0.0.1", []byte(`{"action":"close","session":"test"}`))
	if ctx.Err() == nil {
		t.Fatal("power must close the device")
	}

}

func TestPlaybackCaptionIsPrivateTransientAndGenerationChecked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	token := strings.Repeat("a", 64)
	uid := "playback-caption-test"
	r := &playbackReceiver{uid: uid, peer: "127.0.0.1", token: token}
	s := &playbackSession{device: RockDevice{ID: "PBK01", RegistryCode: strings.Repeat("b", 32)}, state: PlaybackState{Session: "playback", Generation: 2, Phase: "playing"}, receivers: map[string]*playbackReceiver{uid: r}, ctx: ctx, cancel: cancel}
	m := &playbackManager{sessions: map[string]*playbackSession{"playback": s}}
	queue := &DevConQueue{Com: make(chan string, 32), Captions: make(chan string, 8), Done: make(chan struct{})}
	deviceController.Lock()
	deviceController.Handler[uid] = queue
	deviceController.Unlock()
	defer func() {
		close(queue.Done)
		deviceController.Lock()
		delete(deviceController.Handler, uid)
		deviceController.Unlock()
	}()
	router := gin.New()
	router.GET("/dev", func(g *gin.Context) {
		if !m.receiver(g) {
			g.Status(http.StatusForbidden)
		}
	})
	server := httptest.NewServer(router)
	defer server.Close()
	headers := http.Header{"Authorization": []string{"Bearer " + token}, HEADER_CLIENT_TVROCK_DTV_PARENT: []string{s.device.RegistryCode}}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/dev", headers)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	var control RequestMessage
	if err := conn.ReadJSON(&control); err != nil {
		t.Fatal(err)
	}
	if control.Command != "PlaybackControl" {
		t.Fatal(control)
	}
	for _, message := range []string{`PBA:{"generation":2}`, `RCV:{"ready":true,"playback":true}`, `CAP:{"Text":"must not enter normal path"}`, `PBC:{"generation":1,"caption":{"Text":"stale"}}`, `EIT:[]`, `PBC:{"generation":2,"caption":{"Text":"再生字幕","HistoryID":123}}`} {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case message := <-queue.Captions:
		payload, ok := strings.CutPrefix(message, "ply:")
		if !ok {
			t.Fatalf("unexpected caption notification: %q", message)
		}
		var value struct {
			Action  string        `json:"action"`
			Caption PacketCaption `json:"caption"`
		}
		if err := json.Unmarshal([]byte(payload), &value); err != nil {
			t.Fatalf("invalid playback caption notification %q: %v", message, err)
		}
		if value.Action != "caption" || value.Caption.Text != "再生字幕" || value.Caption.HistoryID != 0 {
			t.Fatal(message)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("playback caption not delivered")
	}
	select {
	case message := <-queue.Captions:
		t.Fatalf("unexpected caption notification: %q", message)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPlaybackSRTSendsOriginalPackets(t *testing.T) {
	config := srt.DefaultConfig()
	config.Latency = 20 * time.Millisecond
	listener, err := srt.Listen("srt", "127.0.0.1:0", config)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	result := make(chan []byte, 1)
	data := bytes.Repeat([]byte{0x47, 1, 2, 3}, 329) // one 1316-byte SRT payload
	go func() {
		req, err := listener.Accept2()
		if err != nil {
			result <- nil
			return
		}
		conn, err := req.Accept()
		if err != nil {
			result <- nil
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		buffer := make([]byte, len(data))
		_, err = io.ReadFull(conn, buffer)
		if err != nil {
			result <- nil
			return
		}
		result <- buffer
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	packets := make(chan []byte, 1)
	packets <- data
	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	r := &playbackReceiver{peer: "127.0.0.1", port: port, options: WatchSRTOptions{LatencyMS: 20}, streamGeneration: 1}
	s := &playbackSession{}
	done := make(chan struct{})
	go func() { defer close(done); (&playbackManager{}).send(ctx, s, r, packets, 1) }()
	select {
	case received := <-result:
		if !bytes.Equal(data, received) {
			t.Fatal("SRT changed the original TS payload")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("SRT transfer timed out")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("sender did not stop")
	}
}

func TestPlaybackSessionStreamsWholeFileAndReleasesDevice(t *testing.T) {
	testPlaybackWholeFile(t, false)
}
func TestPlaybackDirectPauseResumeKeepsSRTAndWholeFile(t *testing.T) {
	testPlaybackWholeFile(t, true)
}
func testPlaybackWholeFile(t *testing.T, direct bool) {
	data := playbackTestTS(t)
	path := filepath.Join(t.TempDir(), "recorded.ts")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	uid := "playback-session-test"
	queue := &DevConQueue{Com: make(chan string, 128), Done: make(chan struct{})}
	deviceController.Lock()
	deviceController.Handler[uid] = queue
	deviceController.Unlock()
	defer func() {
		close(queue.Done)
		deviceController.Lock()
		delete(deviceController.Handler, uid)
		deviceController.Unlock()
	}()
	m := &playbackManager{sessions: map[string]*playbackSession{}, clients: map[string]playbackClient{uid: {peer: "127.0.0.1", native: true}}}
	token, err := m.start(&RecordingSchedule{ID: 1, Title: "録画テスト", FileName: path, Sid: 1, EndTime: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	m.Lock()
	session := m.sessions[token]
	m.Unlock()
	defer session.cancel()
	deadline := time.After(5 * time.Second)
	requestSeen := false
waitOffer:
	for {
		select {
		case message := <-queue.Com:
			if strings.HasPrefix(message, "ply:") {
				var value map[string]any
				json.Unmarshal([]byte(message[4:]), &value)
				if value["action"] == "request" {
					requestSeen = true
					if value["deviceId"] != "PBK01" {
						t.Fatal("request did not identify PBK")
					}
				}
				if value["action"] == "offer" {
					if !requestSeen {
						t.Fatal("request receipt must be sent before analysis/launch offer")
					}
					break waitOffer
				}
			}
		case <-deadline:
			t.Fatal("no Playback offer")
		}
	}
	config := srt.DefaultConfig()
	config.Latency = 20 * time.Millisecond
	listener, err := srt.Listen("srt", "127.0.0.1:0", config)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	join, _ := json.Marshal(playbackCommand{Action: "join", Session: token, Device: session.device.RegistryCode, Port: port, SRT: &WatchSRTOptions{LatencyMS: 20}})
	m.command(uid, "127.0.0.1", join)
	session.Lock()
	receiver := session.receivers[uid]
	session.Unlock()
	if receiver == nil {
		t.Fatal("no receiver reservation")
	}
	router := gin.New()
	router.GET("/dev", func(g *gin.Context) {
		if !m.receiver(g) {
			g.Status(http.StatusForbidden)
		}
	})
	server := httptest.NewServer(router)
	defer server.Close()
	headers := http.Header{"Authorization": []string{"Bearer " + receiver.token}, HEADER_CLIENT_TVROCK_DTV_PARENT: []string{session.device.RegistryCode}}
	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/dev", headers)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	var request RequestMessage
	if err := ws.ReadJSON(&request); err != nil {
		t.Fatal(err)
	}
	if request.Command != "PlaybackControl" {
		t.Fatal(request)
	}
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`PBA:{"generation":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`RCV:{"ready":true,"playback":true}`)); err != nil {
		t.Fatal(err)
	}
	if direct {
		if err := ws.WriteMessage(websocket.TextMessage, []byte(`RCV:{"ready":true,"playback":true,"controlVersion":2,"graphPauseSupported":true,"graphPauseReady":true}`)); err != nil {
			t.Fatal(err)
		}
	}
	ackDone := make(chan struct{})
	go func() {
		defer close(ackDone)
		for {
			var control RequestMessage
			if ws.ReadJSON(&control) != nil {
				return
			}
			if control.Command != "PlaybackControl" {
				continue
			}
			var value struct {
				Generation uint32 `json:"generation"`
				Action     string `json:"action"`
			}
			json.Unmarshal([]byte(control.Message), &value)
			ack, _ := json.Marshal(map[string]any{"generation": value.Generation, "action": value.Action, "success": true})
			if ws.WriteMessage(websocket.TextMessage, append([]byte("PBA:"), ack...)) != nil {
				return
			}
		}
	}()
	defer func() { ws.Close(); <-ackDone }()
	firstRead := make(chan struct{})
	result := make(chan []byte, 1)
	go func() {
		req, err := listener.Accept2()
		if err != nil {
			result <- nil
			return
		}
		conn, err := req.Accept()
		if err != nil {
			result <- nil
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(time.Now().Add(8 * time.Second))
		received := make([]byte, len(data))
		_, err = io.ReadFull(conn, received[:1316])
		close(firstRead)
		if err == nil {
			_, err = io.ReadFull(conn, received[1316:])
		}
		if err != nil {
			result <- nil
			return
		}
		result <- received
	}()
	if direct {
		select {
		case <-firstRead:
		case <-time.After(5 * time.Second):
			t.Fatal("no initial TS")
		}
		session.Lock()
		original := receiver.conn
		streamGeneration := receiver.streamGeneration
		session.Unlock()
		m.command(uid, "", []byte(`{"action":"pause","session":"`+token+`"}`))
		waitState := func(phase string, generation uint32) PlaybackState {
			t.Helper()
			deadline := time.Now().Add(2 * time.Second)
			for {
				session.Lock()
				state := session.state
				ack := receiver.controlAck
				session.Unlock()
				if state.Phase == phase && state.Generation == generation && ack == generation {
					return state
				}
				if time.Now().After(deadline) {
					t.Fatalf("waiting for %s/%d: %+v ack=%d", phase, generation, state, ack)
				}
				time.Sleep(5 * time.Millisecond)
			}
		}
		paused := waitState("paused", 2)
		time.Sleep(100 * time.Millisecond)
		session.Lock()
		still := session.state.PositionMS
		same := receiver.conn == original
		held := receiver.hold
		session.Unlock()
		if still != paused.PositionMS || !same || !held {
			t.Fatal("pause advanced position or replaced SRT")
		}
		m.command(uid, "", []byte(`{"action":"play","session":"`+token+`"}`))
		waitState("playing", 3)
		session.Lock()
		same = receiver.conn == original && receiver.streamGeneration == streamGeneration && !receiver.hold
		session.Unlock()
		if !same {
			t.Fatal("resume reconnected SRT")
		}
	}
	select {
	case received := <-result:
		if !bytes.Equal(received, data) {
			t.Fatalf("whole-file streaming changed/lost TS (%d/%d)", len(received), len(data))
		}
	case <-time.After(10 * time.Second):
		t.Fatal("whole-file transfer timed out")
	}
	for deadline := time.Now().Add(3 * time.Second); ; {
		session.Lock()
		idle := session.state.Phase == "idle"
		session.Unlock()
		if idle {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("PBK did not return to idle after EOF")
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func TestPlaybackPauseSeekResumeAndStop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recording.ts")
	if err := os.WriteFile(path, playbackTestTS(t), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	packets := make(chan []byte, 256)
	r := &playbackReceiver{uid: "owner", connected: true, ready: true, streaming: true, controlAck: 1, packets: packets, queue: make(chan RequestMessage, 8), lastSeen: time.Now()}
	s := &playbackSession{device: RockDevice{ID: "PBK01", Kind: "playback"}, state: PlaybackState{Session: "control", Generation: 1}, schedule: RecordingSchedule{Sid: 1}, owner: "owner", invited: map[string]bool{"owner": true}, receivers: map[string]*playbackReceiver{"owner": r}, ctx: ctx, cancel: cancel, commands: make(chan playbackCommand, 8)}
	m := &playbackManager{sessions: map[string]*playbackSession{"control": s}, clients: map[string]playbackClient{"owner": {native: true}}}
	ackDone := make(chan struct{})
	go func() {
		defer close(ackDone)
		for {
			select {
			case <-ctx.Done():
				return
			case request := <-r.queue:
				if request.Command != "PlaybackControl" {
					continue
				}
				var control struct {
					Generation uint32 `json:"generation"`
					Action     string `json:"action"`
				}
				json.Unmarshal([]byte(request.Message), &control)
				s.Lock()
				r.controlAck = control.Generation
				r.packets = packets
				r.streaming = control.Action == "reset"
				s.Unlock()
			}
		}
	}()
	done := make(chan struct{})
	go func() { defer close(done); m.run(s, file) }()
	defer func() { cancel(); <-done; <-ackDone }()
	wait := func(phase string, generation uint32) PlaybackState {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for {
			s.Lock()
			state := s.state
			s.Unlock()
			if state.Phase == phase && state.Generation == generation {
				return state
			}
			if time.Now().After(deadline) {
				t.Fatalf("waiting %s/%d: %+v", phase, generation, state)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait("playing", 1)
	m.command("owner", "", []byte(`{"action":"pause","session":"control"}`))
	paused := wait("paused", 2)
	time.Sleep(80 * time.Millisecond)
	s.Lock()
	position := s.state.PositionMS
	s.Unlock()
	if position != paused.PositionMS {
		t.Fatal("paused file position advanced")
	}
	m.command("owner", "", []byte(`{"action":"seek","session":"control","seconds":1}`))
	seeked := wait("paused", 3)
	if seeked.PositionMS <= paused.PositionMS {
		t.Fatal("forward seek did not move file position")
	}
	m.command("owner", "", []byte(`{"action":"seek-start","session":"control"}`))
	if state := wait("paused", 4); state.PositionMS != 0 {
		t.Fatal("seek-start did not return to the beginning")
	}
	m.command("owner", "", []byte(`{"action":"seek-end","session":"control"}`))
	if state := wait("paused", 5); state.PositionMS < 1500 {
		t.Fatal("seek-end did not move to the final indexed interval")
	}
	m.command("owner", "", []byte(`{"action":"seek-to","session":"control","seconds":1}`))
	if state := wait("paused", 6); state.PositionMS < 500 || state.PositionMS > 1000 {
		t.Fatalf("absolute seek did not return to one second: %+v", state)
	}
	m.command("owner", "", []byte(`{"action":"seek-to","session":"control","seconds":9223372036854775807}`))
	if state := wait("paused", 7); state.PositionMS < 1500 {
		t.Fatal("absolute seek overflowed instead of clamping to end")
	}
	m.command("owner", "", []byte(`{"action":"play","session":"control"}`))
	wait("playing", 8)
	m.command("owner", "", []byte(`{"action":"stop","session":"control"}`))
	wait("idle", 9)
	if ctx.Err() != nil || len(m.devices()) != 1 {
		t.Fatal("stop removed waiting device")
	}
	m.command("owner", "", []byte(`{"action":"close","session":"control"}`))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop did not end the session")
	}
	if len(m.devices()) != 0 {
		t.Fatal("stopped PBK is still visible")
	}
}
