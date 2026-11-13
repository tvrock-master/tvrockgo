package tvrock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	srt "github.com/datarhei/gosrt"
	"github.com/gin-gonic/gin"
)

type PlaybackState struct {
	ScheduleID        uint64             `json:"scheduleId"`
	Slot              int                `json:"slot"`
	Session           string             `json:"session"`
	Title             string             `json:"title"`
	Phase             string             `json:"phase"`
	PositionMS        int64              `json:"positionMs"`
	DurationMS        int64              `json:"durationMs"`
	Recording         bool               `json:"recording,omitempty"`
	WaitingForData    bool               `json:"waitingForData,omitempty"`
	Generation        uint32             `json:"generation"`
	Error             string             `json:"error,omitempty"`
	CMStatus          string             `json:"cmStatus,omitempty"`
	CMBoundaries      []playbackBoundary `json:"cmBoundaries,omitempty"`
	ManualBookmarks   []playbackBoundary `json:"manualBookmarks,omitempty"`
	ProgramBoundaries []playbackBoundary `json:"programBoundaries,omitempty"`
}

type playbackBoundary struct {
	PositionMS      int64  `json:"positionMs"`
	Kind            string `json:"kind"`
	EventID         uint32 `json:"eventId,omitempty"`
	PreviousEventID uint16 `json:"previousEventId,omitempty"`
}

type playbackCommand struct {
	Slot       int              `json:"slot"`
	Action     string           `json:"action"`
	Session    string           `json:"session"`
	Device     string           `json:"device"`
	Port       int              `json:"port"`
	SRT        *WatchSRTOptions `json:"srt"`
	Seconds    int64            `json:"seconds"`
	ScheduleID uint64           `json:"scheduleId"`
	Native     bool             `json:"native"`
	Error      string           `json:"error"`
	PositionMS *int64           `json:"positionMs"`
	Generation uint32           `json:"generation"`
}

type playbackClient struct {
	peer   string
	native bool
}

type playbackReceiver struct {
	service           uint32
	nextServiceSelect time.Time
	uid, peer, token  string
	port              int
	options           WatchSRTOptions
	queue             chan RequestMessage
	connected         bool
	ready             bool
	controlAck        uint32
	packets           chan []byte
	streaming         bool
	streamGeneration  uint32
	conn              srt.Conn
	cancel            context.CancelFunc
	lastSeen          time.Time
	graphSupported    bool
	idleSupported     bool
	idleDisabled      bool
	graphReady        bool
	graphDisabled     bool
	fastPaused        bool
	hold              bool
	pendingAction     string
	controlDeadline   time.Time
	fallback          bool
}

type playbackSession struct {
	stopRequested bool
	fileCancel    context.CancelFunc
	fileContext   context.Context
	nextFile      *playbackSelection
	sync.Mutex
	device    RockDevice
	state     PlaybackState
	schedule  RecordingSchedule
	receivers map[string]*playbackReceiver
	invited   map[string]bool
	ctx       context.Context
	cancel    context.CancelFunc
	commands  chan playbackCommand
	owner     string
}

type playbackManager struct {
	ports map[string]string
	sync.Mutex
	sessions map[string]*playbackSession
	clients  map[string]playbackClient
}

var playback = playbackManager{sessions: map[string]*playbackSession{}, clients: map[string]playbackClient{}}

func playbackBoundaries(index *playbackIndex) []playbackBoundary {
	boundaries := []playbackBoundary{}
	for _, candidate := range index.metrics.Candidates {
		if candidate.TimeNS >= 0 && candidate.TimeNS <= int64(index.duration) {
			boundaries = append(boundaries, playbackBoundary{PositionMS: candidate.TimeNS / int64(time.Millisecond), Kind: candidate.Kind})
		}
	}
	sort.SliceStable(boundaries, func(i, j int) bool { return boundaries[i].PositionMS < boundaries[j].PositionMS })
	return boundaries
}

func (s *playbackSession) enqueue(data []byte) {
	s.Lock()
	defer s.Unlock()
	for _, r := range s.receivers {
		if r.connected && r.ready && r.streaming && r.packets != nil && r.controlAck == s.state.Generation {
			select {
			case r.packets <- append([]byte(nil), data...):
			default:
				if r.cancel != nil {
					r.cancel()
				}
				r.ready = false
			}
		}
	}
}

func playbackActive() bool {
	playback.Lock()
	defer playback.Unlock()
	return len(playback.sessions) > 0
}

func playbackDefinitions() []RockDevice {
	devices := append([]RockDevice(nil), configure.PlaybackDevices...)
	if len(devices) == 0 {
		for i := 1; i <= 8; i++ {
			devices = append(devices, RockDevice{ID: fmt.Sprintf("PBK%02d", i)})
		}
	}

	for i := range devices {
		devices[i].Kind = "playback"
		if devices[i].RegistryCode == "" {
			hash := sha256.Sum256([]byte("TvRockGO/playback/" + devices[i].ID))
			devices[i].RegistryCode = hex.EncodeToString(hash[:16])
		}
	}

	return devices
}

func validatePlaybackDefinitions(devices []RockDevice) error {
	ids, registries := map[string]bool{}, map[string]bool{}
	for _, device := range configure.Devices {
		ids[device.ID] = true
		registries[device.RegistryCode] = true
	}

	for _, device := range devices {
		if device.ID == "" || ids[device.ID] || registries[device.RegistryCode] {
			return errors.New("PlaybackデバイスのID・登録コードが空か重複しています")
		}
		ids[device.ID] = true
		registries[device.RegistryCode] = true
	}

	return nil
}

func (m *playbackManager) devices() []*RockDevice {
	m.Lock()
	sessions := make([]*playbackSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.Unlock()

	devices := []*RockDevice{}
	for _, s := range sessions {
		s.Lock()
		d := s.device
		state := s.state
		s.Unlock()
		d.State = StateReady
		d.Playback = &state
		devices = append(devices, &d)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })
	return devices
}

func playbackNotify(uid string, value any) {
	data, _ := json.Marshal(value)
	deviceController.NotifyTo(uid, "ply:", string(data))
}

func (m *playbackManager) publish() {
	m.Lock()
	uids := make([]string, 0, len(m.clients))
	for uid := range m.clients {
		uids = append(uids, uid)
	}
	m.Unlock()

	for _, uid := range uids {
		data, err := json.Marshal(m.devicesFor(uid))
		if err != nil {
			log.Printf("error: Playbackデバイス一覧の送信データ作成: %v", err)
			continue
		}
		deviceController.NotifyTo(uid, "pbs:", string(data))
	}
}

func openPlaybackRecording(schedule *RecordingSchedule, slots ...int) (*os.File, error) {
	if schedule == nil || schedule.WatchOnly || schedule.FileName == "" {
		return nil, errors.New("録画ファイルを選択してください")
	}
	if !recordingTSStillActive(schedule.ID) && !schedule.Complete && schedule.EndTime > Now().Unix() {
		return nil, errors.New("録画済みファイルを選択してください")
	}

	slotNo := CurrentSlot
	if len(slots) > 0 {
		slotNo = slots[0]
	}

	file, err := os.Open(schedule.FileName)
	if err != nil {
		if os.IsNotExist(err) && !recordingTSStillActive(schedule.ID) {
			markPlaybackFileDeleted(slotNo, schedule)
		}
		return nil, errors.New("録画ファイルを開けません")
	}

	stat, err := file.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("通常の録画ファイルを指定してください")
	}
	clearPlaybackFileDeleted(slotNo, schedule)

	return file, nil
}

func (m *playbackManager) start(schedule *RecordingSchedule, owner string) (string, error) {
	definitions := playbackDefinitions()
	if err := validatePlaybackDefinitions(definitions); err != nil {
		return "", err
	}
	var file *os.File
	if schedule != nil {
		var err error
		file, err = openPlaybackRecording(schedule)
		if err != nil {
			return "", err
		}
	}
	closeFile := func() {
		if file != nil {
			file.Close()
		}
	}

	m.Lock()
	invited := map[string]bool{}
	for uid, c := range m.clients {
		if (owner == "" || owner == uid) && c.native {
			invited[uid] = true
		}
	}
	if len(invited) == 0 {
		m.Unlock()
		closeFile()
		return "", errors.New("再生可能なElectron版Rockbarが接続されていません")
	}

	var device *RockDevice
	for _, d := range definitions {
		used := false
		for _, s := range m.sessions {
			if s.device.ID == d.ID && (owner == "" || s.owner == "" || s.owner == owner) {
				used = true
				break
			}
		}
		if !used {
			copy := d
			device = &copy
			break
		}
	}
	if device == nil {
		m.Unlock()
		closeFile()
		return "", errors.New("空いているPlaybackデバイスがありません")
	}

	ctx, cancel := context.WithCancel(serviceContext())
	token := watchToken()
	snapshot := RecordingSchedule{}
	if schedule != nil {
		snapshot = *schedule
	}
	phase := "idle"
	if file != nil {
		phase = "indexing"
	}
	s := &playbackSession{device: *device, state: PlaybackState{Session: token, Title: snapshot.Title, ScheduleID: snapshot.ID, Phase: phase, Generation: 1}, schedule: snapshot,
		receivers: map[string]*playbackReceiver{}, invited: invited, ctx: ctx, cancel: cancel, commands: make(chan playbackCommand, 32), owner: owner}
	m.sessions[token] = s
	requested := make([]string, 0, len(invited))
	for uid := range invited {
		requested = append(requested, uid)
	}
	m.Unlock()

	begin := make(chan struct{})
	if !serviceWorkers.start(func() { <-begin; m.run(s, file) }) {
		m.Lock()
		delete(m.sessions, token)
		m.Unlock()
		cancel()
		closeFile()
		return "", errors.New("サーバーを終了しています")
	}
	defer close(begin)
	for _, uid := range requested {
		playbackNotify(uid, map[string]any{"action": "request", "session": token, "deviceId": device.ID, "title": snapshot.Title})
	}
	m.offer(s)
	m.publish()

	return token, nil
}

func (m *playbackManager) command(uid, peer string, data []byte) {
	var c playbackCommand
	if json.Unmarshal(data, &c) != nil {
		return
	}

	if c.Action == "hello" {
		m.Lock()
		m.clients[uid] = playbackClient{peer, c.Native}
		m.Unlock()
		playbackNotify(uid, map[string]any{"action": "devices", "devices": playbackDefinitions()})
		return
	}

	if c.Action == "create" {
		if _, err := m.start(nil, uid); err != nil {
			playbackNotify(uid, map[string]string{"error": err.Error()})
		}
		return
	}

	if c.Action == "load" {
		m.selectFile(uid, c)
		return
	}

	if c.Action == "start" {
		schedule, err := scheduler.GetRecordingSchedule(CurrentSlot, c.ScheduleID)
		if err == nil && schedule == nil {
			err = errors.New("録画ログを取得できません")
		}
		if err == nil {
			_, err = m.start(schedule, uid)
		}
		if err != nil {
			playbackNotify(uid, map[string]string{"error": err.Error()})
		}
		return
	}
	m.Lock()
	s := m.sessions[c.Session]
	client, connected := m.clients[uid]
	m.Unlock()
	if s == nil || !connected {
		return
	}
	s.Lock()
	allowed := s.invited[uid] || s.owner == "" && (c.Action == "close" || c.Action == "stop" ||
		c.Action == "pause" || c.Action == "play" || c.Action == "seek" || c.Action == "seek-to" ||
		c.Action == "seek-start" || c.Action == "seek-end")
	if !allowed {
		s.Unlock()
		return
	}

	if c.Action == "bookmark-add" || c.Action == "bookmark-delete" {
		if c.PositionMS == nil || c.Generation != s.state.Generation || (s.state.Phase != "playing" && s.state.Phase != "paused") || *c.PositionMS < 0 || *c.PositionMS > s.state.DurationMS {
			s.Unlock()
			playbackNotify(uid, map[string]string{"error": "再生位置を確認できません。状態更新後にしおりを追加してください"})
			return
		}
		schedule, slotNo, duration := s.schedule, s.state.Slot, s.state.DurationMS
		s.Unlock()
		_, err := changeManualPlaybackBookmark(slotNo, &schedule, *c.PositionMS, duration, c.Action == "bookmark-delete")
		if err != nil {
			playbackNotify(uid, map[string]string{"error": "しおりを保存できません: " + err.Error()})
			return
		}
		m.updateManualBookmarks(slotNo, &schedule)
		return
	}

	if c.Action == "join" {
		_, options, valid := watchTransport("srt", c.SRT)
		if !valid || c.Port < 1 || c.Port > 65535 || peer != client.peer || c.Device != s.device.RegistryCode || s.receivers[uid] != nil {
			s.Unlock()
			return
		}
		for _, r := range s.receivers {
			if r.peer == peer && r.port == c.Port {
				s.Unlock()
				playbackNotify(uid, map[string]string{"error": "受信ポートが重複しています"})
				return
			}
		}
		key := net.JoinHostPort(peer, strconv.Itoa(c.Port))
		m.Lock()
		if m.ports == nil {
			m.ports = map[string]string{}
		}
		reservation := c.Session + "/" + uid
		if previous := m.ports[key]; previous != "" && previous != reservation {
			m.Unlock()
			s.Unlock()
			playbackNotify(uid, map[string]string{"error": "このPCの受信ポートは別の再生デバイスが使用中です"})
			return
		}
		m.ports[key] = reservation
		m.Unlock()
		r := &playbackReceiver{uid: uid, peer: peer, token: watchToken(), port: c.Port, options: *options, lastSeen: time.Now()}
		s.receivers[uid] = r
		service := s.schedule.Sid & 65535
		s.Unlock()
		playbackNotify(uid, map[string]any{"action": "launch", "session": c.Session, "receiver": r.token, "device": s.device.RegistryCode, "deviceId": s.device.ID, "port": r.port, "service": service})
		return
	}

	if c.Action == "decline" {
		delete(s.invited, uid)
		if r := s.receivers[uid]; r != nil {
			if r.queue != nil {
				select {
				case r.queue <- RequestMessage{Command: "PowerOff"}:
				default:
				}
			}
			if r.cancel != nil {
				r.cancel()
			}
			if r.conn != nil {
				r.conn.Close()
			}
			delete(s.receivers, uid)
		}
		if len(s.invited) == 0 {
			s.cancel()
		}
		s.Unlock()
		m.releasePorts(c.Session, uid)
		if c.Error != "" {
			playbackNotify(uid, map[string]string{"action": "status", "session": c.Session, "deviceId": s.device.ID, "phase": "failed", "error": c.Error})
		}
		return
	}

	s.Unlock()

	if c.Action == "close" {
		s.cancel()
		return
	}

	if c.Action == "stop" {
		s.Lock()
		s.stopRequested = true
		if s.nextFile != nil {
			s.nextFile.file.Close()
		}
		s.nextFile = nil
		if s.fileCancel != nil {
			s.fileCancel()
		}
		s.Unlock()
		return
	}

	select {
	case s.commands <- c:
	default:
		playbackNotify(uid, map[string]string{"error": "再生操作が混み合っています"})
	}
}

func (m *playbackManager) disconnect(uid string) {
	m.Lock()
	delete(m.clients, uid)
	ss := []*playbackSession{}
	for _, s := range m.sessions {
		ss = append(ss, s)
	}
	m.Unlock()

	for _, s := range ss {
		s.Lock()
		delete(s.invited, uid)
		if r := s.receivers[uid]; r != nil {
			if r.cancel != nil {
				r.cancel()
			}
			delete(s.receivers, uid)
		}
		if len(s.invited) == 0 {
			s.cancel()
		}
		s.Unlock()
		m.releasePorts(s.state.Session, uid)
	}
}

func (s *playbackSession) control(r *playbackReceiver, action string) bool {
	if r.queue == nil {
		return false
	}

	data, _ := json.Marshal(map[string]any{"generation": s.state.Generation, "action": action})
	select {
	case r.queue <- RequestMessage{Command: "PlaybackControl", Message: string(data)}:
		r.pendingAction = action
		r.controlDeadline = time.Time{}
		if action == "pause-direct" || action == "resume-direct" {
			r.controlDeadline = time.Now().Add(time.Second)
		}
		return true
	default:
		return false
	}
}

// セッションロックを保持した状態で呼び出される。レガシーなレシーバーには新しいアクションが渡されることはない
func (s *playbackSession) legacyControl(r *playbackReceiver, action string) {
	if r.cancel != nil {
		r.cancel()
	}
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
	r.packets = nil
	r.streaming = false
	r.fastPaused = false
	r.hold = false
	r.fallback = false
	s.control(r, action)
}

func (s *playbackSession) fastResumeReady() bool {
	if len(s.receivers) == 0 {
		return false
	}

	for _, r := range s.receivers {
		if !r.connected || !r.fastPaused || !r.graphReady || r.graphDisabled || r.controlAck != s.state.Generation {
			return false
		}
	}
	return true
}

func (s *playbackSession) fallbackControls(now time.Time, paused bool) {
	for _, r := range s.receivers {
		if !r.fallback && (r.controlDeadline.IsZero() || now.Before(r.controlDeadline)) {
			continue
		}
		r.graphDisabled = true
		s.legacyControl(r, map[bool]string{true: "pause", false: "reset"}[paused])
	}
}

func (s *playbackSession) acknowledgeControl(r *playbackReceiver, generation uint32, action string, success *bool) {
	direct := r.pendingAction == "pause-direct" || r.pendingAction == "resume-direct"
	idle := r.pendingAction == "idle"
	if generation != s.state.Generation || action != r.pendingAction && !(action == "" && !direct && !idle) {
		return
	}
	if success != nil && !*success || (direct || idle) && success == nil {
		if idle {
			r.idleSupported = false
			r.idleDisabled = true
			s.legacyControl(r, "reset")
			return
		}
		r.fallback = true
		return
	}
	r.controlAck = generation
	r.controlDeadline = time.Time{}
	if action == "resume-direct" {
		r.hold = false
		r.fastPaused = false
	}
}

func (m *playbackManager) playFile(s *playbackSession, file *os.File) {
	defer file.Close()
	ctx := s.fileContext
	index, err := loadOrAnalyzePlaybackTS(ctx, file, &s.schedule, s.state.Slot)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		s.Lock()
		s.state.Error = err.Error()
		s.Unlock()
		return
	}
	growing := index.tail != nil
	position := readPlaybackPosition(s.state.Slot, &s.schedule, index.duration, growing)
	if position > 0 {
		point := index.seek(position)
		if _, err = file.Seek(point.offset, io.SeekStart); err != nil {
			return
		}
		position = point.time
	}
	ended := false
	defer func() { savePlaybackPosition(s.state.Slot, &s.schedule, position, index.duration, ended, growing) }()
	lastSave := time.Now()
	playbackBookmarkMu.Lock()
	manualBookmarks := loadManualPlaybackBookmarks(s.state.Slot, &s.schedule, index.duration.Milliseconds())
	activeRecording := recordingTSStillActive(s.schedule.ID)
	s.Lock()
	s.state.PositionMS = position.Milliseconds()
	s.state.DurationMS = index.duration.Milliseconds()
	s.state.Recording = activeRecording
	s.state.WaitingForData = false
	s.state.CMStatus = index.metrics.CMStatus
	s.state.CMBoundaries = playbackBoundaries(index)
	s.state.ProgramBoundaries = programPlaybackBoundaries(index)
	s.state.ManualBookmarks = manualBookmarks
	s.state.Phase = "preparing"
	for _, r := range s.receivers {
		if r.queue != nil {
			select {
			case r.queue <- RequestMessage{Command: "ReceiverService", Message: strconv.FormatUint(uint64(s.schedule.Sid&65535), 10)}:
			default:
			}
		}
	}
	s.Unlock()
	playbackBookmarkMu.Unlock()
	m.publish()
	paused := false
	started := false
	var previous int64
	seenPCR := false
	baseWall := time.Now()
	basePosition := position
	lastPublish := time.Now()
	waitingSince := time.Now()
	preparingSince := time.Now()
	var pendingCommand *playbackCommand
	packet := make([]byte, 188)
	batch := make([]byte, 0, 1316)
	packetBytes := 0
	var tailWait playbackTailWait
	lastIndexUpdate := time.Now()
	refreshIndex := func() error {
		if !growing {
			return nil
		}
		if err := index.extend(ctx, file); err != nil {
			return err
		}
		active := recordingTSStillActive(s.schedule.ID)
		s.Lock()
		s.state.DurationMS = index.duration.Milliseconds()
		s.state.Recording = active
		s.Unlock()
		lastIndexUpdate = time.Now()
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if growing && time.Since(lastIndexUpdate) >= time.Second {
			if err := refreshIndex(); err != nil {
				s.Lock()
				s.state.Error = err.Error()
				s.Unlock()
				return
			}
		}
		command := pendingCommand
		pendingCommand = nil
		if command == nil {
			select {
			case c := <-s.commands:
				command = &c
			default:
			}
		}
		if command != nil {
			c := *command
			if growing && (c.Action == "play" || strings.HasPrefix(c.Action, "seek")) {
				if err := refreshIndex(); err != nil {
					s.Lock()
					s.state.Error = err.Error()
					s.Unlock()
					return
				}
			}
			s.Lock()
			fastResume := c.Action == "play" && paused && s.fastResumeReady()
			s.Unlock()
			if c.Action == "stop" {
				return
			}
			if c.Action == "pause" {
				paused = true
			}
			if c.Action == "play" {
				if paused && !fastResume {
					point := index.seek(position)
					if _, err = file.Seek(point.offset, io.SeekStart); err != nil {
						return
					}
					position = point.time
					seenPCR = false
					packetBytes = 0
					batch = batch[:0]
				}
				paused = false
			}
			if c.Action == "seek" || c.Action == "seek-to" || c.Action == "seek-start" || c.Action == "seek-end" {
				if c.Action == "seek" && (c.Seconds < -3600 || c.Seconds > 3600) {
					continue
				}
				target := position + time.Duration(c.Seconds)*time.Second
				if c.Action == "seek-to" {
					// Clamp before converting to Duration to avoid overflow from client input.
					seconds := min(max(c.Seconds, 0), int64(index.duration/time.Second))
					target = time.Duration(seconds) * time.Second
				}
				if c.Action == "seek-start" {
					target = 0
				}
				if c.Action == "seek-end" {
					target = index.duration
				}
				if target < 0 {
					target = 0
				}
				if target > index.duration {
					target = index.duration
				}
				point := index.seek(target)
				if _, err = file.Seek(point.offset, io.SeekStart); err != nil {
					return
				}
				position = point.time
				seenPCR = false
				packetBytes = 0
				batch = batch[:0]
			}
			if c.Action == "pause" || c.Action == "play" || c.Action == "seek" || c.Action == "seek-to" || c.Action == "seek-start" || c.Action == "seek-end" {
				s.Lock()
				s.state.PositionMS = position.Milliseconds()
				if paused {
					s.state.Phase = "paused"
				} else {
					s.state.Phase = "preparing"
				}
				s.state.Generation++
				for _, r := range s.receivers {
					if c.Action == "pause" && r.connected && r.graphSupported && r.graphReady && !r.graphDisabled && r.streaming && r.conn != nil {
						r.hold = true
						r.fastPaused = true
						if !s.control(r, "pause-direct") {
							r.fallback = true
						}
					} else if fastResume {
						if !s.control(r, "resume-direct") {
							r.fallback = true
						}
					} else {
						s.legacyControl(r, map[bool]string{true: "pause", false: "reset"}[paused])
					}
				}
				s.Unlock()
				baseWall = time.Now()
				basePosition = position
				s.Lock()
				statusRecipients := make([]string, 0, len(s.receivers))
				for _, r := range s.receivers {
					statusRecipients = append(statusRecipients, r.uid)
				}
				statusGeneration := s.state.Generation
				s.Unlock()
				for _, uid := range statusRecipients {
					phase := "preparing"
					if paused {
						phase = "paused"
					}
					playbackNotify(uid, map[string]any{"action": "status", "session": s.state.Session, "deviceId": s.device.ID, "phase": phase, "generation": statusGeneration})
				}
				m.publish()
			}
		}
		s.Lock()
		s.fallbackControls(time.Now(), paused)
		ready := 0
		count := len(s.invited)
		for _, r := range s.receivers {
			if !paused && r.connected && r.ready && r.service != uint32(s.schedule.Sid&65535) && time.Now().After(r.nextServiceSelect) && r.queue != nil {
				select {
				case r.queue <- RequestMessage{Command: "ReceiverService", Message: strconv.FormatUint(uint64(s.schedule.Sid&65535), 10)}:
					r.nextServiceSelect = time.Now().Add(time.Second)
				default:
				}
			}
			if r.connected && time.Since(r.lastSeen) > 15*time.Second {
				if r.cancel != nil {
					r.cancel()
				}
				continue
			}
			if !paused && r.connected && r.ready && r.controlAck == s.state.Generation && r.packets == nil {
				ctx, cancel := context.WithCancel(ctx)
				r.cancel = cancel
				r.packets = make(chan []byte, 256)
				r.streamGeneration = s.state.Generation
				packets, generation := r.packets, r.streamGeneration
				if !serviceWorkers.start(func() { m.send(ctx, s, r, packets, generation) }) {
					cancel()
				}
			}
			if r.connected && r.ready && r.streaming && r.controlAck == s.state.Generation {
				ready++
			}
		}
		// 共有ファイルのクロックを開始する前に、招待されたすべての視聴者が接続を完了するための時間を設ける。
		// 接続できないPCが1台あったとしてもそれが他の視聴者の進行を妨げてはならない。
		if !started && ready < count && time.Since(preparingSince) < 5*time.Second {
			ready = 0
		}
		if !started && ready > 0 {
			started = true
			baseWall = time.Now()
			basePosition = position
		}
		if ready > 0 || paused {
			waitingSince = time.Now()
		}
		if paused {
			s.state.Phase = "paused"
		} else if ready == 0 {
			s.state.Phase = "preparing"
		} else {
			s.state.Phase = "playing"
		}
		s.state.PositionMS = position.Milliseconds()
		s.Unlock()
		if count == 0 {
			return
		}
		if time.Since(waitingSince) > 30*time.Second {
			s.Lock()
			s.state.Error = "再生受信側の準備が完了しませんでした"
			s.Unlock()
			return
		}
		if time.Since(lastPublish) > time.Second {
			m.publish()
			lastPublish = time.Now()
		}
		if !paused && ready > 0 && time.Since(lastSave) >= playbackProgressInterval {
			savePlaybackPosition(s.state.Slot, &s.schedule, position, index.duration, false, growing)
			lastSave = time.Now()
		}
		if paused || ready == 0 {
			baseWall = time.Now()
			basePosition = position
			select {
			case <-ctx.Done():
				return
			case c := <-s.commands:
				pendingCommand = &c
			case <-time.After(20 * time.Millisecond):
			}
			continue
		}
		n, readErr := io.ReadFull(file, packet[packetBytes:])
		packetBytes += n
		err = readErr
		if growing && (err == io.EOF || err == io.ErrUnexpectedEOF) {
			info, statErr := file.Stat()
			if statErr != nil {
				s.Lock()
				s.state.Error = statErr.Error()
				s.Unlock()
				return
			}
			active := recordingTSStillActive(s.schedule.ID)
			if !tailWait.finished(active, info.Size(), time.Now()) {
				if len(batch) > 0 {
					s.enqueue(batch)
					batch = batch[:0]
				}
				s.Lock()
				changed := !s.state.WaitingForData
				s.state.WaitingForData = true
				s.state.Recording = active
				s.state.PositionMS = position.Milliseconds()
				s.Unlock()
				if changed {
					m.publish()
				}
				baseWall = time.Now()
				basePosition = position
				select {
				case <-ctx.Done():
					return
				case c := <-s.commands:
					pendingCommand = &c
				case <-time.After(200 * time.Millisecond):
				}
				continue
			}
			if packetBytes > 0 {
				err = io.ErrUnexpectedEOF
			} else {
				err = io.EOF
			}
		}
		if err != nil {
			if err == io.EOF {
				ended = true
				if len(batch) > 0 {
					s.enqueue(batch)
				}
				// ビューアーを閉じる前にキューに入っているTS（タイムスタンプ）およびネゴシエーション済みの受信レイテンシをすべて処理し終えるようにする。
				// なお、停止／シャットダウン操作は引き続き即座にキャンセル可能とする。
				s.Lock()
				drain := 500 * time.Millisecond
				for _, r := range s.receivers {
					if d := time.Duration(r.options.LatencyMS)*time.Millisecond + 500*time.Millisecond; d > drain {
						drain = d
					}
				}
				s.Unlock()
				select {
				case <-ctx.Done():
				case <-time.After(drain):
				}
			}
			if err != io.EOF {
				s.Lock()
				s.state.Error = "TSの読み出しに失敗しました"
				s.Unlock()
			}
			return
		}
		packetBytes = 0
		if growing {
			s.Lock()
			s.state.WaitingForData = false
			s.Unlock()
		}
		if pcr, ok := playbackPCR(packet, index.pcrPID); ok {
			if seenPCR {
				position += playbackPCRDelta(previous, pcr)
			}
			previous = pcr
			seenPCR = true
			wait := baseWall.Add(position - basePosition).Sub(time.Now())
			if wait > 0 {
				select {
				case <-ctx.Done():
					return
				case c := <-s.commands:
					pendingCommand = &c
				case <-time.After(wait):
				}
			}
		}
		batch = append(batch, packet...)
		if len(batch) == 1316 {
			s.enqueue(batch)
			batch = batch[:0]
		}
	}
}

func (m *playbackManager) send(ctx context.Context, s *playbackSession, r *playbackReceiver, packets <-chan []byte, generation uint32) {
	if ctx.Err() != nil {
		return
	}
	playbackNotify(r.uid, map[string]any{"action": "status", "session": s.state.Session, "deviceId": s.device.ID, "phase": "waiting-srt", "generation": generation})
	config := srt.DefaultConfig()
	config.PayloadSize = 1316
	config.Latency = time.Duration(r.options.LatencyMS) * time.Millisecond
	config.Passphrase = r.options.Passphrase
	config.ConnectionTimeout = 5 * time.Second
	conn, err := srt.Dial("srt", net.JoinHostPort(r.peer, strconv.Itoa(r.port)), config)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		playbackNotify(r.uid, map[string]any{"action": "status", "session": s.state.Session, "deviceId": s.device.ID, "phase": "failed", "error": "SRT接続に失敗しました", "generation": generation})
		s.Lock()
		r.ready = false
		s.Unlock()
		return
	}
	defer conn.Close()
	s.Lock()
	if ctx.Err() != nil || r.streamGeneration != generation {
		s.Unlock()
		return
	}
	r.conn = conn
	r.streaming = true
	s.Unlock()
	playbackNotify(r.uid, map[string]any{"action": "status", "session": s.state.Session, "deviceId": s.device.ID, "phase": "playing", "generation": generation})
	defer func() {
		s.Lock()
		if r.streamGeneration == generation {
			r.streaming = false
			r.conn = nil
			r.packets = nil
			if ctx.Err() == nil && (r.fastPaused || r.pendingAction == "resume-direct") {
				r.fallback = true
			}
		}
		s.Unlock()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-packets:
			// 直接的な一時停止ではキュー内のTSおよびSRT接続が維持されます。
			// 再開時には視聴者が再生中であることを確認した後にのみキューが解放されます。
			for {
				s.Lock()
				hold := r.hold
				s.Unlock()
				if !hold {
					break
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
			if ctx.Err() != nil {
				return
			}
			conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
			if _, err := conn.Write(data); err != nil {
				if ctx.Err() == nil {
					playbackNotify(r.uid, map[string]any{"action": "status", "session": s.state.Session, "deviceId": s.device.ID, "phase": "failed", "error": "SRT送信に失敗しました", "generation": generation})
				}
				return
			}
		}
	}
}

// このエンドポイントは、デバイス、EPG、ステーション、または字幕のデータベースに関する通常の処理パスには一切入りません
func (m *playbackManager) receiver(g *gin.Context) bool {
	parent := g.GetHeader(HEADER_CLIENT_TVROCK_DTV_PARENT)
	token := strings.TrimPrefix(g.GetHeader("Authorization"), "Bearer ")

	m.Lock()
	var session *playbackSession
	var receiver *playbackReceiver
	for _, s := range m.sessions {
		s.Lock()
		for _, r := range s.receivers {
			if r.token == token && s.device.RegistryCode == parent && r.peer == watchPeer(g.Request.RemoteAddr) && !r.connected {
				session = s
				receiver = r
				break
			}
		}
		s.Unlock()
		if session != nil {
			break
		}
	}
	m.Unlock()

	if session == nil {
		return false
	}
	s, r := session, receiver
	c, err := upgrader.Upgrade(g.Writer, g.Request, nil)
	if err != nil {
		return true
	}
	defer c.Close()
	c.SetReadLimit(64 * 1024)
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	s.Lock()
	if r.connected {
		s.Unlock()
		return true
	}
	r.connected = true
	r.lastSeen = time.Now()
	r.queue = make(chan RequestMessage, 16)
	r.graphSupported, r.graphReady, r.graphDisabled = false, false, false
	r.idleSupported = false
	r.idleDisabled = false
	r.hold, r.fastPaused, r.fallback = false, false, false
	r.controlAck = 0
	r.controlDeadline = time.Time{}
	s.control(r, "reset")
	s.Unlock()

	defer func() {
		s.Lock()
		r.connected = false
		r.ready = false
		r.controlAck = 0
		r.hold, r.fastPaused, r.fallback = false, false, false
		r.controlDeadline = time.Time{}
		r.queue = nil
		if r.cancel != nil {
			r.cancel()
		}
		if r.conn != nil {
			r.conn.Close()
			r.conn = nil
		}
		r.packets = nil
		r.streaming = false
		s.Unlock()
	}()

	if !serviceWorkers.start(func() {
		defer cancel()
		for {
			_, data, err := c.ReadMessage()
			if err != nil {
				return
			}

			if strings.HasPrefix(string(data), "RCV:") {
				var report struct {
					Ready               bool   `json:"ready"`
					Playback            bool   `json:"playback"`
					ControlVersion      int    `json:"controlVersion"`
					IdleSupported       bool   `json:"idleSupported"`
					GraphPauseSupported bool   `json:"graphPauseSupported"`
					GraphPauseReady     bool   `json:"graphPauseReady"`
					Service             uint32 `json:"service"`
				}
				if json.Unmarshal(data[4:], &report) == nil {
					s.Lock()
					r.ready = report.Ready && report.Playback
					r.lastSeen = time.Now()
					r.graphSupported = report.ControlVersion >= 2 && report.GraphPauseSupported
					r.graphReady = r.graphSupported && report.GraphPauseReady
					idleSupported := !r.idleDisabled && report.ControlVersion >= 3 && report.IdleSupported
					if idleSupported && !r.idleSupported && s.state.Phase == "idle" {
						s.legacyControl(r, "idle")
					}
					r.idleSupported = idleSupported
					r.service = report.Service
					s.Unlock()
				}
			}

			if strings.HasPrefix(string(data), "PBA:") {
				var report struct {
					Generation uint32 `json:"generation"`
					Action     string `json:"action"`
					Success    *bool  `json:"success"`
				}
				if json.Unmarshal(data[4:], &report) == nil {
					s.Lock()
					s.acknowledgeControl(r, report.Generation, report.Action, report.Success)
					s.Unlock()
				}
			}

			if strings.HasPrefix(string(data), "PBC:") {
				var cap struct {
					Generation uint32        `json:"generation"`
					Caption    PacketCaption `json:"caption"`
				}
				if json.Unmarshal(data[4:], &cap) == nil {
					s.Lock()
					valid := cap.Generation == s.state.Generation && r.controlAck == s.state.Generation && s.state.Phase == "playing"
					session := s.state.Session
					s.Unlock()
					if valid {
						cap.Caption.HistoryID = 0
						playbackNotify(r.uid, map[string]any{"action": "caption", "session": session, "device": s.device.ID, "generation": cap.Generation, "caption": cap.Caption})
					}
				}
			}
		}
	}) {
		return true
	}
	for {
		select {
		case <-ctx.Done():
			if s.ctx.Err() != nil {
				c.SetWriteDeadline(time.Now().Add(time.Second))
				_ = c.WriteJSON(RequestMessage{Command: "PowerOff"})
			}
			return true
		case req := <-r.queue:
			c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if c.WriteJSON(req) != nil {
				return true
			}
		}
	}
}

func APIPlaybackStart(c *gin.Context) {
	var request struct {
		ScheduleID uint64 `json:"scheduleId"`
	}

	if c.ShouldBindJSON(&request) != nil || request.ScheduleID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "録画ログを指定してください"})
		return
	}

	schedule, err := scheduler.GetRecordingSchedule(CurrentSlot, request.ScheduleID)
	if err != nil || schedule == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "録画ログを取得できません"})
		return
	}
	sessions, err := playback.startOnAllRockbars(schedule)

	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"session": sessions[0], "sessions": sessions})
}
