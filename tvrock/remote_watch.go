package tvrock

// リモート視聴はレコーダーの録画状態とは無関係にレコーダーを利用する権利（リース）を確保するものです。
// 実行ファイルのパスや引数を保持するのはElectronクライアントのみです。
// 本プロトコルでやり取りされるのは、識別子・ポート番号およびステータス情報であり
// 実行コマンドそのものが送信されることはありません。
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type WatchStreamOptions struct {
	CurrentServiceOnly bool `json:"currentServiceOnly"`
	Caption            bool `json:"caption"`
	DataBroadcast      bool `json:"dataBroadcast"`
	OneSeg             bool `json:"oneSeg"`
}

type WatchCommand struct {
	Transport string              `json:"transport,omitempty"`
	SRT       *WatchSRTOptions    `json:"srt,omitempty"`
	Action    string              `json:"action"`
	Device    string              `json:"device"`
	Session   string              `json:"session"`
	Port      uint16              `json:"port"`
	Stream    *WatchStreamOptions `json:"stream,omitempty"`
	Schedule  string              `json:"schedule,omitempty"`
}

type SourceStreamStatus struct {
	SRTSupported     bool   `json:"srtSupported"`
	Transport        string `json:"transport"`
	MultiSupported   bool   `json:"multiSupported"`
	OptionsSupported bool   `json:"optionsSupported"`
	Service          uint16 `json:"service"`
	Flags            uint32 `json:"flags"`
	Session          string `json:"session"`
	Request          string `json:"request"`
	Supported        bool   `json:"supported"`
	Success          bool   `json:"success"`
	Enabled          bool   `json:"enabled"`
	Address          string `json:"address"`
	Port             uint16 `json:"port"`
	Bytes            uint64 `json:"bytes"`
	LastSendTick     uint64 `json:"lastSendTick"`
	PacedSupported   bool   `json:"pacedSupported"`
	Paced            bool   `json:"paced"`
	Error            uint32 `json:"error"`
}

type receiverStreamStatus struct {
	Epoch            uint64 `json:"epoch"`
	Ready            bool   `json:"ready"`
	Bytes            uint64 `json:"bytes"`
	AgeMS            uint64 `json:"ageMs"`
	Service          uint16 `json:"service"`
	QualitySupported bool   `json:"qualitySupported"`
	Drops            uint32 `json:"drops"`
	QualityAgeMS     uint64 `json:"qualityAgeMs"`
}

type watchSession struct {
	Transport           string              `json:"transport,omitempty"`
	SRT                 *WatchSRTOptions    `json:"-"`
	Schedule            string              `json:"schedule,omitempty"`
	Stream              WatchStreamOptions  `json:"stream"`
	Token               string              `json:"session"`
	Parent              string              `json:"device"`
	DeviceID            string              `json:"deviceId"`
	Address             string              `json:"address"`
	Port                uint16              `json:"port"`
	Phase               string              `json:"phase"`
	Error               string              `json:"error,omitempty"`
	Source              SourceStreamStatus  `json:"source"`
	Received            uint64              `json:"receivedBytes"`
	ReceiverService     uint16              `json:"receiverService"`
	GraceSeconds        int                 `json:"graceSeconds"`
	Owner               string              `json:"-"`
	Created             time.Time           `json:"-"`
	Touched             time.Time           `json:"-"`
	ReceiverSeen        time.Time           `json:"-"`
	SourceSeen          time.Time           `json:"-"`
	LastQuery           time.Time           `json:"-"`
	LastBoot            time.Time           `json:"-"`
	LastStart           time.Time           `json:"-"`
	LastTune            time.Time           `json:"-"`
	ReceiverQueue       chan RequestMessage `json:"-"`
	ReceiverConnection  string              `json:"-"`
	ReceiverReady       bool                `json:"-"`
	LastReceiveAge      uint64              `json:"-"`
	Attempts            int                 `json:"-"`
	Established         bool                `json:"-"`
	SourceLost          time.Time           `json:"-"`
	Stopping            bool                `json:"-"`
	StopStarted         time.Time           `json:"-"`
	StopAck             bool                `json:"-"`
	QualityService      uint                `json:"-"`
	QualitySourceErrors uint                `json:"-"`
	Quality             watchQuality        `json:"-"`
}

type remoteWatchManager struct {
	sync.Mutex
	sessions  map[string]*watchSession
	boot      func(*LaunchMessage)
	byParent  map[string]string
	scheduled map[uint64]*scheduledWatch
}

type WatchSRTOptions struct {
	LatencyMS  int    `json:"latencyMs"`
	Passphrase string `json:"passphrase"`
}

// 品質監視は視聴セッションに紐付くものであり、録画タスクには紐付きません。
// 起動、チャンネル切り替え、再接続、およびソース側で発生したエラーはウォームアップを再開始させます。
const watchQualityWarmup = 10 * time.Second

type watchQuality struct {
	eligible                             bool
	seen                                 bool
	stableSince, lastReport, lastRequest time.Time
	service                              uint
	sourceErrors                         uint
	receiverService                      uint16
	epoch                                uint64
	drops                                uint32
	bytes                                uint64
	badIntervals                         int
	wanted                               bool
}

var remoteWatching = remoteWatchManager{sessions: map[string]*watchSession{}, byParent: map[string]string{}}

func normalizeRemoteWatchSetting(s *RemoteWatchSetting) {
	if s.MaxSessions <= 0 {
		s.MaxSessions = 32
	}
	if s.KeepBeforeScheduleSeconds <= 0 {
		s.KeepBeforeScheduleSeconds = 60
	}
	if s.DisconnectGraceSeconds > 0 && s.DisconnectGraceSeconds < 5 {
		s.DisconnectGraceSeconds = 5
	}
	if s.DisconnectGraceSeconds > 86400 {
		s.DisconnectGraceSeconds = 86400
	}
	if s.DisconnectGraceSeconds <= 0 {
		s.DisconnectGraceSeconds = 15
	}
	if s.StartupTimeoutSeconds <= 0 {
		s.StartupTimeoutSeconds = 60
	}
}

func watchSettings() RemoteWatchSetting {
	configure.mux.RLock()
	s := configure.RemoteWatch
	configure.mux.RUnlock()
	normalizeRemoteWatchSetting(&s)
	return s
}

func defaultWatchStreamOptions() WatchStreamOptions {
	return WatchStreamOptions{CurrentServiceOnly: true, Caption: true, DataBroadcast: true}
}

func (o WatchStreamOptions) flags() uint32 {
	var flags uint32
	if o.CurrentServiceOnly || o.OneSeg {
		flags |= 1
	}
	if o.Caption {
		flags |= 2
	}
	if o.DataBroadcast {
		flags |= 4
	}
	if o.OneSeg {
		flags |= 8
	}
	return flags
}

func watchToken() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

func validWatchToken(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32
}

func watchPeer(address string) string {
	host, _, e := net.SplitHostPort(address)
	if e != nil {
		return ""
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() {
		return ""
	}
	return ip.To4().String()
}

func sendWatchNotice(uid string, s interface{}) {
	b, e := json.Marshal(s)
	if e != nil {
		return
	}
	deviceController.RLock()
	q := deviceController.Handler[uid]
	deviceController.RUnlock()
	if q != nil {
		select {
		case <-q.Done:
			return
		default:
		}
		select {
		case q.Com <- "wat:" + string(b):
		default:
		}
	}
}

// レコーダーのコマンドキューが満杯の状態であってもWebSocketの読み取り処理やリースループをブロックしてはならない
func sendWatchRequest(parent, command string, payload interface{}) bool {
	d, ok := slot.GetByRID(parent)
	if !ok {
		return false
	}
	b, e := json.Marshal(payload)
	if e != nil {
		return false
	}
	slot.RLock()
	q, done, state := d.Com, d.requestDone, d.State
	slot.RUnlock()
	if state < StateReady || q == nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
	}
	select {
	case q <- RequestMessage{ID: parent, Command: command, Message: string(b)}:
		return true
	default:
		return false
	}
}

func (m *remoteWatchManager) bootRecorder(message *LaunchMessage) {
	if m.boot != nil {
		m.boot(message)
	} else {
		launcher.Exec(message)
	}
}

func (m *remoteWatchManager) activeDevice(id string) bool {
	m.Lock()
	defer m.Unlock()
	for _, s := range m.sessions {
		if s.DeviceID == id && !s.Stopping {
			return true
		}
	}
	return false
}

func (m *remoteWatchManager) activeParent(parent string) bool {
	m.Lock()
	defer m.Unlock()
	return m.byParent[parent] != ""
}

// 同じ登録デバイスを視聴している受信側だけにキャプチャーを依頼する。
// キューが満杯でも録画側へフォールバックせず、他の受信側への送信を続ける。
func (m *remoteWatchManager) captureDevice(parent string) bool {
	m.Lock()
	defer m.Unlock()
	handled := false
	for _, s := range m.sessions {
		if s.Parent != parent || s.Stopping || !s.ReceiverReady || s.ReceiverQueue == nil {
			continue
		}
		handled = true
		select {
		case s.ReceiverQueue <- RequestMessage{Command: "Capture"}:
		default:
		}
	}
	return handled
}

func (m *remoteWatchManager) command(uid, peer string, raw []byte) {
	var c WatchCommand
	if len(raw) > 2048 || json.Unmarshal(raw, &c) != nil {
		sendWatchNotice(uid, map[string]string{"error": "視聴要求が不正です"})
		return
	}
	now := time.Now()
	m.Lock()
	defer m.Unlock()
	if c.Action == "start" {
		transport, srt, valid := watchTransport(c.Transport, c.SRT)
		if !valid {
			sendWatchNotice(uid, map[string]string{"error": "SRTの送信設定が不正です"})
			return
		}
		if peer == "" || c.Port == 0 {
			sendWatchNotice(uid, map[string]string{"error": "IPv4接続と受信ポートが必要です"})
			return
		}
		d, ok := slot.GetByRID(c.Device)
		if !ok {
			sendWatchNotice(uid, map[string]string{"error": "デバイスが見つかりません"})
			return
		}
		if c.Schedule != "" && !m.validSchedule(c.Schedule, c.Device) {
			return
		}
		count := 0
		for _, s := range m.sessions {
			if s.Parent == c.Device {
				count++
			}
		}
		if count >= watchSettings().MaxSessions {
			sendWatchNotice(uid, map[string]string{"error": "同時視聴数の上限に達しました"})
			return
		}
		for _, s := range m.sessions {
			if s.Address == peer && s.Port == c.Port {
				sendWatchNotice(uid, map[string]string{"error": "この受信ポートは使用中です"})
				return
			}
		}
		token := watchToken()
		if token == "" {
			return
		}
		options := defaultWatchStreamOptions()
		if c.Stream != nil {
			options = *c.Stream
		}
		if options.OneSeg {
			options.CurrentServiceOnly = true
		}
		s := &watchSession{Transport: transport, SRT: srt, Schedule: c.Schedule, Stream: options, Token: token, Parent: c.Device, DeviceID: d.ID, Address: peer, Port: c.Port, Owner: uid, Created: now, Touched: now, Phase: "preparing", GraceSeconds: watchSettings().DisconnectGraceSeconds}
		m.sessions[token] = s
		if m.byParent[c.Device] == "" {
			m.byParent[c.Device] = token
		}
		sendWatchNotice(uid, s)
		// This recorder belongs to this server, not the receiver's launcher.
		slot.RLock()
		off := d.State == StateOFF
		slot.RUnlock()
		if off {
			s.LastBoot = now
			m.bootRecorder(&LaunchMessage{RegistryID: d.RegistryCode, DeviceID: d.ID, BootType: DeviceBootTypeWatch})
		}
		return
	}
	s := m.sessions[c.Session]
	if s == nil || !validWatchToken(c.Session) || s.Address != peer {
		return
	}
	if c.Action == "resume" && !s.Stopping {
		if s.Owner != uid {
			s.Quality = watchQuality{}
		}
		s.Owner = uid
		s.Touched = now
		sendWatchNotice(uid, s)
		return
	}
	if s.Owner != uid {
		return
	}
	switch c.Action {
	case "adopt":
		if s.Schedule != "" && !s.Stopping && m.validSchedule(c.Schedule, s.Parent) {
			s.Schedule = c.Schedule
		}
	case "ping":
		s.Touched = now
	case "stop":
		s.Stopping = true
		if s.StopStarted.IsZero() {
			s.StopStarted = now
		}
		s.Phase = "stopping"
		sendWatchNotice(uid, s)
	case "capture":
		if s.ReceiverQueue != nil {
			select {
			case s.ReceiverQueue <- RequestMessage{Command: "Capture"}:
			default:
			}
		}
	}
}

func (m *remoteWatchManager) disconnect(uid string) {
	m.Lock()
	defer m.Unlock()
	for _, s := range m.sessions {
		if s.Owner == uid {
			s.Owner = ""
			s.Touched = time.Now()
		}
	}
}

func (m *remoteWatchManager) status(parent string, data []byte) {
	m.statusWithQuality(parent, data, 0, 0)
}

// デバイスの受信Goroutineから呼び出される。
// これらのサンプルは受信Goroutineから（変更可能な）DeviceInfoを読み取るのではなくSTRと並行して取得される。
func (m *remoteWatchManager) statusWithQuality(parent string, data []byte, service, sourceErrors uint) {
	var status SourceStreamStatus
	if len(data) > 4096 || json.Unmarshal(data, &status) != nil {
		return
	}
	m.Lock()
	defer m.Unlock()
	token := status.Request
	if token == "" {
		token = status.Session
	}
	if token == "" {
		token = m.byParent[parent]
	}
	s := m.sessions[token]
	if s != nil && s.Parent != parent {
		return
	}
	if s == nil {
		// DLLは親プロセスの再起動や長時間の制御通信途絶が発生した際、孤立したUDPリースを期限切れとして処理する
		// 未使用のレコーダーが再接続されたらそれを破棄する。
		if status.Error == 1460 && !status.Enabled {
			if d, ok := slot.GetByRID(parent); ok {
				id := d.ID
				serviceWorkers.start(func() { m.retireRecorder(id, parent) })
			}
		}
		return
	}

	// 終了したビューに属する応答（遅延した停止ACKを含む）は無視する
	if status.Request != "" && status.Request != s.Token {
		return
	}
	if status.Session != "" && status.Session != s.Token && status.Request == "" {
		return
	}
	s.QualityService, s.QualitySourceErrors = service, sourceErrors
	s.Source = status
	s.SourceSeen = time.Now()
	if s.Stopping && status.Request == s.Token && status.Success && !status.Enabled {
		s.StopAck = true
	}
}

func (m *remoteWatchManager) run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("info: 終了通知を検出")
			return
		case now := <-ticker.C:
			m.tick(now)
		}
	}
}

func (m *remoteWatchManager) tick(now time.Time) {
	jobs := scheduledWatchJobs()
	cfg := watchSettings()
	m.Lock()
	defer m.Unlock()
	if jobs != nil {
		m.updateScheduled(jobs)
	}

	for token, s := range m.sessions {
		grace := time.Duration(cfg.DisconnectGraceSeconds) * time.Second
		expired := now.Sub(s.Touched) > grace || (!s.ReceiverSeen.IsZero() && now.Sub(s.ReceiverSeen) > grace)
		startupExpired := !s.Established && now.Sub(s.Created) > time.Duration(cfg.StartupTimeoutSeconds)*time.Second
		if expired || startupExpired {
			s.Stopping = true
			if s.Error == "" {
				s.Error = "視聴端末の応答または受信を確認できません"
			}
		}

		if s.Stopping {
			if s.StopStarted.IsZero() {
				s.StopStarted = now
				s.Phase = "stopping"
			}
			if !s.StopAck && now.Sub(s.LastQuery) >= time.Second {
				if sendWatchRequest(s.Parent, "StreamStop", map[string]string{"session": token}) {
					s.LastQuery = now
				}
			}
			if s.ReceiverQueue != nil {
				select {
				case s.ReceiverQueue <- RequestMessage{Command: "PowerOff"}:
				default:
				}
			}
			if s.StopAck || now.Sub(s.StopStarted) > grace {
				s.Phase = "stopped"
				sendWatchNotice(s.Owner, s)
				if m.byParent[s.Parent] == token {
					delete(m.byParent, s.Parent)
					for otherToken, other := range m.sessions {
						if otherToken != token && other.Parent == s.Parent {
							m.byParent[s.Parent] = otherToken
							break
						}
					}
				}
				delete(m.sessions, token)
				// ビューを閉じても実行中または実行予定のジョブを終了させて​​はならない
				id, parent := s.DeviceID, s.Parent
				serviceWorkers.start(func() { m.retireRecorder(id, parent) })
			}
			continue
		}

		d, ok := slot.GetByRID(s.Parent)
		if !ok {
			s.Stopping = true
			continue
		}

		slot.RLock()
		state, service := d.State, d.CurrentService
		slot.RUnlock()
		if s.Established && (state < StateReady || (!s.SourceSeen.IsZero() && now.Sub(s.SourceSeen) > grace)) {
			if s.SourceLost.IsZero() {
				s.SourceLost = now
			}
			if now.Sub(s.SourceLost) > grace {
				s.Error = "送信デバイスとの接続が失われました"
				s.Stopping = true
				continue
			}
		} else {
			s.SourceLost = time.Time{}
		}
		if state == StateOFF && now.Sub(s.LastBoot) >= 2*time.Second {
			s.LastBoot = now
			m.bootRecorder(&LaunchMessage{RegistryID: s.Parent, DeviceID: s.DeviceID, BootType: DeviceBootTypeWatch})
		}
		if state < StateReady {
			s.Phase = "waiting-source"
			sendWatchNotice(s.Owner, s)
			continue
		}
		if now.Sub(s.LastQuery) >= 3*time.Second {
			if sendWatchRequest(s.Parent, "GetStreamStatus", map[string]string{"session": token}) {
				s.LastQuery = now
			}
		}
		if !s.SourceSeen.IsZero() && !s.Source.Supported {
			s.Error = "このデバイスは配信制御に対応していません"
			s.Stopping = true
			continue
		}
		if !s.SourceSeen.IsZero() && s.Transport == "srt" && !s.Source.SRTSupported {
			s.Error = "SRT配信にはDLL配信モード対応の録画アプリ本体（TSTask／RecTest等）とTvRockDTVが必要です。両方を更新してください"
			s.Stopping = true
			continue
		}
		if !s.ReceiverReady || s.ReceiverQueue == nil || service == 0 {
			s.Phase = "waiting-receiver"
			sendWatchNotice(s.Owner, s)
			continue
		}
		if s.SourceSeen.IsZero() || now.Sub(s.SourceSeen) > grace {
			s.Phase = "checking-source"
			sendWatchNotice(s.Owner, s)
			continue
		}
		if !s.SourceSeen.IsZero() && !s.Source.MultiSupported && m.byParent[s.Parent] != token {
			s.Error = "複数PCへの送信に対応した録画アプリとTvRockDTVへ更新してください"
			s.Stopping = true
			continue
		}
		if !s.SourceSeen.IsZero() && s.Stream.flags() != 7 && !s.Source.OptionsSupported {
			s.Error = "送信内容の設定に対応した録画アプリとTvRockDTVへ更新してください"
			s.Stopping = true
			continue
		}
		if s.Stream.OneSeg && s.Source.Enabled && s.Source.Error == 1168 {
			s.Error = "この放送ではワンセグサービスを確認できません"
			s.Stopping = true
			continue
		}
		correct := s.Source.Enabled && s.Source.Session == token && s.Source.Address == s.Address && s.Source.Port == s.Port && (!s.Source.OptionsSupported || s.Source.Flags == s.Stream.flags()) && watchTransportMatches(s.Transport, s.Source.Transport)
		if !correct && now.Sub(s.LastStart) >= 5*time.Second {
			if s.Attempts >= 3 {
				s.Error = "配信を開始できません"
				s.Stopping = true
				continue
			}
			command, payload := watchStartRequest(s, cfg.DisconnectGraceSeconds)
			if sendWatchRequest(s.Parent, command, payload) {
				s.LastStart = now
				s.Attempts++
				s.Phase = "starting"
			}
		}
		if correct {
			if s.Stream.OneSeg {
				service = uint(s.Source.Service)
			}
			s.Phase = "streaming"
			s.Attempts = 0
			if s.LastReceiveAge < 3000 && s.Received > 0 {
				s.Phase = "receiving"
				s.Established = true
			}
			if service > 0 && (s.ReceiverService != uint16(service) || s.LastReceiveAge >= 3000) && now.Sub(s.LastTune) >= 2*time.Second {
				select {
				case s.ReceiverQueue <- RequestMessage{Command: "ReceiverService", Message: strconv.FormatUint(uint64(service&65535), 10)}:
					s.LastTune = now
				default:
				}
			}
		}
		sendWatchNotice(s.Owner, s)
	}
}

// 最終的なシャットダウン処理のEnqueueを新規ビューアによる取得操作と競合しないよう直列化する
func (m *remoteWatchManager) sendRetirement(parent string) bool {
	m.Lock()
	defer m.Unlock()
	if m.byParent[parent] != "" {
		return false
	}
	return sendWatchRequest(parent, "PowerOff", "")
}

func (m *remoteWatchManager) retireRecorder(id, parent string) {
	cfg := watchSettings()
	deadline := time.Now().Add(time.Duration(cfg.StartupTimeoutSeconds+cfg.DisconnectGraceSeconds) * time.Second)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		if keepWatchDevice(id, Now()) {
			return
		}
		d, ok := slot.GetByRID(parent)
		if !ok {
			return
		}
		slot.RLock()
		state := d.State
		slot.RUnlock()
		launcher.RLock()
		booting := launcher.Booting[parent]
		launcher.RUnlock()
		if state >= StateReady && m.sendRetirement(parent) {
			return
		}
		if state == StateOFF && !booting {
			return
		}
		select {
		case <-serviceContext().Done():
			return
		case <-ticker.C:
		}
	}
}

// 設定された期間内にスケジュールが実行中、または開始される場合にのみ真となる
// データベースの読み取りエラーが発生した場合は、安全側に倒してレコーダーの動作を継続させる
func scheduleKeepsWatchDevice(s *RecordingSchedule, id string, now time.Time, seconds int) bool {
	return s != nil && s.DeviceID == id && s.Validate && !s.Complete && s.EndTime+s.TailTime > now.Unix() && s.StartTime-s.IdleTime <= now.Unix()+int64(seconds)
}

func keepWatchDevice(id string, now time.Time) bool {
	if remoteWatching.activeDevice(id) {
		return true
	}
	d, ok := slot.Get(id)
	if ok {
		slot.RLock()
		recording := d.State == StateRecording
		slot.RUnlock()
		if recording {
			return true
		}
	}
	if _, ok := scheduler.GetCurrentRecording(id); ok {
		return true
	}
	if scheduler.db == nil {
		return false
	}
	schedules, e := scheduler.GetScheduleAll()
	if e != nil {
		return true
	}
	cfg := watchSettings()
	for _, s := range schedules {
		if scheduleKeepsWatchDevice(s, id, now, cfg.KeepBeforeScheduleSeconds) {
			return true
		}
	}
	return false
}

// Sibling（関連インスタンス）はビューごとに発行される一時的なトークンを使用して認証を行います
// レコーダーの登録コードが使用されることは決してありません
// また、Siblingからのメッセージによって録画、EPG、あるいはデバイスの状態が変更されることはありません
func (m *remoteWatchManager) receiver(g *gin.Context) {
	parent := g.GetHeader(HEADER_CLIENT_TVROCK_DTV_PARENT)
	token := strings.TrimPrefix(g.GetHeader("Authorization"), "Bearer ")
	peer := watchPeer(g.Request.RemoteAddr)
	connection := watchToken()
	m.Lock()
	s := m.sessions[token]
	allowed := s != nil && !s.Stopping && s.Parent == parent && s.Address == peer && validWatchToken(token) && connection != "" && s.ReceiverConnection == ""
	m.Unlock()
	if !allowed {
		g.AbortWithStatus(http.StatusForbidden)
		return
	}
	c, e := upgrader.Upgrade(g.Writer, g.Request, nil)
	if e != nil {
		return
	}
	defer c.Close()
	c.SetReadLimit(4096)
	ctx, cancel := context.WithCancel(serviceContext())
	defer cancel()
	q := make(chan RequestMessage, 16)
	m.Lock()
	s = m.sessions[token]
	if s == nil || s.Stopping || s.ReceiverConnection != "" {
		m.Unlock()
		return
	}
	s.Quality = watchQuality{}
	s.ReceiverQueue = q
	s.ReceiverConnection = connection
	s.ReceiverSeen = time.Now()
	m.Unlock()
	defer func() {
		m.Lock()
		if s = m.sessions[token]; s != nil && s.ReceiverConnection == connection {
			s.ReceiverQueue = nil
			s.ReceiverConnection = ""
			s.ReceiverReady = false
		}
		m.Unlock()
	}()
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if c.WriteJSON(RequestMessage{Command: "Ready", Message: "0"}) != nil {
		return
	}

	serviceWorkers.start(func() {
		defer cancel()
		for {
			_ = c.SetReadDeadline(time.Now().Add(time.Duration(watchSettings().DisconnectGraceSeconds) * time.Second))
			mt, b, e := c.ReadMessage()
			if e != nil {
				return
			}
			if mt != websocket.TextMessage || !strings.HasPrefix(string(b), "RCV:") {
				continue
			}
			var r receiverStreamStatus
			if json.Unmarshal(b[4:], &r) != nil {
				continue
			}
			m.Lock()
			s = m.sessions[token]
			if s == nil || s.ReceiverConnection != connection {
				m.Unlock()
				return
			}
			{
				m.observeWatchQuality(s, r, time.Now())
				s.ReceiverSeen = time.Now()
				s.ReceiverReady = r.Ready
				s.Received = r.Bytes
				s.LastReceiveAge = r.AgeMS
				s.ReceiverService = r.Service
			}
			m.Unlock()
		}
	})

	for {
		select {
		case <-ctx.Done():
			return
		case request := <-q:
			_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if c.WriteJSON(request) != nil {
				return
			}
		}
	}
}

// パスフレーズは送信開始要求にだけ含め、視聴セッションの状態通知には含めない。
func watchTransport(transport string, srt *WatchSRTOptions) (string, *WatchSRTOptions, bool) {
	if transport == "" || transport == "udp" {
		return "udp", nil, true
	}
	if transport != "srt" {
		return "", nil, false
	}
	options := WatchSRTOptions{LatencyMS: 120}
	if srt != nil {
		options = *srt
	}
	if options.LatencyMS < 0 || options.LatencyMS > 60000 {
		return "", nil, false
	}
	if options.Passphrase != "" {
		if len(options.Passphrase) < 10 || len(options.Passphrase) > 79 {
			return "", nil, false
		}
		for _, c := range []byte(options.Passphrase) {
			if c < 32 || c > 126 {
				return "", nil, false
			}
		}
	}
	return "srt", &options, true
}

func watchTransportMatches(requested, actual string) bool {
	if requested == "srt" {
		return actual == "srt"
	}
	return actual == "" || actual == "udp"
}

func watchStartRequest(s *watchSession, lease int) (string, map[string]interface{}) {
	payload := map[string]interface{}{"session": s.Token, "address": s.Address, "port": s.Port, "leaseSeconds": lease, "flags": s.Stream.flags()}
	if s.Transport == "srt" && s.SRT != nil {
		payload["latencyMs"] = s.SRT.LatencyMS
		payload["passphrase"] = s.SRT.Passphrase
		return "StreamStartSRT", payload
	}
	return "StreamStart", payload
}

func (q *watchQuality) observe(now time.Time, r receiverStreamStatus, service, sourceErrors uint, eligible bool) bool {
	reset := !q.seen || !q.eligible || !eligible || service != q.service || sourceErrors != q.sourceErrors ||
		r.Service != q.receiverService || r.Epoch != q.epoch || r.Drops < q.drops || r.Bytes < q.bytes ||
		now.Sub(q.lastReport) > 2500*time.Millisecond
	if q.seen && now.Sub(q.lastReport) < 750*time.Millisecond {
		return false // Bursts of reports must not simulate sustained loss.
	}
	previousDrops, previousBytes := q.drops, q.bytes
	q.seen, q.eligible, q.lastReport = true, eligible, now
	q.service, q.sourceErrors, q.receiverService, q.epoch = service, sourceErrors, r.Service, r.Epoch
	q.drops, q.bytes = r.Drops, r.Bytes
	if reset {
		q.stableSince, q.badIntervals = now, 0
		return false
	}
	if !q.wanted {
		if now.Sub(q.stableSince) <= watchQualityWarmup {
			q.badIntervals = 0
			return false
		}
		if r.Drops > previousDrops && r.Bytes > previousBytes {
			q.badIntervals++
		} else {
			q.badIntervals = 0
		}
		if q.badIntervals < 3 {
			return false
		}
		q.wanted = true
	}
	return q.lastRequest.IsZero() || now.Sub(q.lastRequest) >= 3*time.Second
}

// watch-managerのロックを保持した状態で呼び出される。また、認証済みの現在のレシーバー接続に対してのみ実行される。
// 古いセッションからは新しいセンダーを要求することはできない。
func (m *remoteWatchManager) observeWatchQuality(s *watchSession, r receiverStreamStatus, now time.Time) {
	if s.Stopping || s.Transport == "srt" {
		return
	}
	service, sourceErrors := s.QualityService, s.QualitySourceErrors
	expected := uint16(service)
	if s.Stream.OneSeg {
		expected = s.Source.Service
	}
	eligible := r.QualitySupported && r.QualityAgeMS < 2500 && r.Ready && r.AgeMS < 3000 &&
		r.Bytes > 0 && expected != 0 && r.Service == expected && s.Established &&
		s.Source.PacedSupported && !s.Source.Paced && s.Source.Enabled && s.Source.Session == s.Token &&
		s.Source.Address == s.Address && s.Source.Port == s.Port &&
		!s.SourceSeen.IsZero() && now.Sub(s.SourceSeen) < 5*time.Second
	if s.Quality.observe(now, r, service, sourceErrors, eligible) &&
		sendWatchRequest(s.Parent, "StreamPace", map[string]string{"session": s.Token}) {
		if s.Quality.lastRequest.IsZero() {
			log.Printf("info: 受信ドロップの継続によりUDP送信の平準化を要求 device=%s", s.DeviceID)
		}
		s.Quality.lastRequest = now
	}
}
