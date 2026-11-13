package tvrock

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"log"
	"sync"
	"time"
)

type TunerType uint

const (
	TunerTypeTS TunerType = iota
	TunerTypeBSCS
	TunerType4K
)

const (
	DeviceBootOFF uint = iota
	DeviceBootTypeWatch
	DeviceBootTypeRecord
	DeviceBootTypeWatchOnly
	DeviceBootTypeOnesegWatch
	DeviceBootTypeOnesegRecord
	DeviceBootTypeOnesegWatchOnly
	DeviceBootTypeFetchProgramTS
	DeviceBootTypeFetchProgramBS
	DeviceBootTypeFetchProgramCS
	DeviceBootTypeFetchProgramOS
)

type DeviceState int

const (
	StateOFF DeviceState = iota
	StateBooting
	StateReady
	StateRecording
	StateShutdown
)

type RequestMessage struct {
	ID       string `json:"id"`
	Command  string `json:"command"`
	Message  string `json:"message"`
	shutdown *deviceShutdownRequest
}

type DeviceInfo struct {
	UnixTime  int64 `json:"unixtime"` // 放送波時刻
	Delay     int64 `json:"delay"`    // 放送波時刻とのずれ(NanoSecond)
	Latency   int64 `json:"latency"`  // 通信レイテンシ(NanoSecond)
	Events    uint  `json:"events"`   // 番組情報数
	SmartCard uint  `json:"scard"`    //
	Signal    uint  `json:"signal"`   //
	Bitrate   uint  `json:"bitrate"`  //
	Error     uint  `json:"error"`    //
	Scramble  uint  `json:"scramble"` //
	Ct        int   `json:"ct"`       // カウンタ
}

type RockDevice struct {
	ID               string              `yaml:"id"`
	RegistryCode     string              `yaml:"registrycode"`
	AppPath1         string              `yaml:"apppath1"`
	Option1          string              `yaml:"option1"`
	AppPath2         string              `yaml:"apppath2"`
	Option2          string              `yaml:"option2"`
	AppPath3         string              `yaml:"apppath3"`
	Option3          string              `yaml:"option3"`
	AppPath4         string              `yaml:"apppath4"`
	Option4          string              `yaml:"option4"`
	RecordPath       string              `yaml:"recordpath"`
	TunerType        TunerType           `yaml:"tunertype"`
	Hybrid           bool                `yaml:"hybrid"`
	Master           string              `yaml:"master"`
	Kind             string              `yaml:"-" json:"Kind,omitempty"`
	Playback         *PlaybackState      `yaml:"-" json:"Playback,omitempty"`
	Recording        bool                `yaml:"-"`
	CurrentService   uint                `yaml:"-"`
	CurrentProgram   *TvProgram          `yaml:"-"`
	NextProgram      *TvProgram          `yaml:"-"`
	State            DeviceState         `yaml:"-"`
	DeviceInfo       DeviceInfo          `yaml:"-"`
	PacketInfo       []PacketInfo        `yaml:"-"`
	EventsAt         int64               `yaml:"-"`
	Com              chan RequestMessage `json:"-" yaml:"-"`
	requestDone      <-chan struct{}
	processExited    bool
	connectionActive bool
}

type RecordingDevice struct {
	OrderTS          []string          `yaml:"orderts"`
	OrderBS          []string          `yaml:"orderbs"`
	OrderCS          []string          `yaml:"ordercs"`
	OrderOS          []string          `yaml:"orderos"`
	Assist           bool              `yaml:"assist"`
	DeviceAssistance map[string]string `yaml:"deviceassistance"`
}

type Slot struct {
	Device            map[string]*RockDevice
	IDs               []string
	connectedReady    map[string]bool
	connectionChanged chan struct{}
	sync.RWMutex
}

var slot Slot

func (p *Slot) Initialize(devices []RockDevice, assist map[string]string) error {
	p.Lock()
	defer p.Unlock()

	p.Device = map[string]*RockDevice{}
	p.IDs = []string{}
	p.connectedReady = map[string]bool{}
	p.connectionChanged = make(chan struct{})

	for _, dev := range devices {
		aid := ""
		if configure.RecordingDevice.Assist {
			aid = assist[dev.ID]
		}
		p.addLocked(newRockDevice(dev.ID, aid, dev))
	}

	return nil
}

// setConnectionStateは、実際のデバイス接続によって報告された準備完了状態を記録します。
// ランチャープロセスの通知だけでは、デバイスが再接続されたことにはなりません。
func (p *Slot) setConnectionState(dev *RockDevice, state DeviceState) {
	p.Lock()
	defer p.Unlock()
	p.setConnectionStateLocked(dev, state)
}

func (p *Slot) setConnectionStateLocked(dev *RockDevice, state DeviceState) {
	dev.State = state
	if state == StateOFF {
		dev.CurrentService = 0
		dev.CurrentProgram = nil
		dev.NextProgram = nil
	}
	if p.connectedReady == nil {
		p.connectedReady = make(map[string]bool)
	}
	p.connectedReady[dev.ID] = state == StateReady || state == StateRecording
	if p.connectionChanged != nil {
		close(p.connectionChanged)
	}
	p.connectionChanged = make(chan struct{})
}

// 状態通知前も接続は使用中。判定と登録を同じロックで行い、
// 古い接続の後始末が新しい接続をOFFにしないようキューで識別する。
func (p *Slot) attachConnection(dev *RockDevice, queue chan RequestMessage, done <-chan struct{}) (bool, bool) {
	p.Lock()
	defer p.Unlock()
	if dev.connectionActive {
		return false, false
	}
	if dev.requestDone != nil {
		select {
		case <-dev.requestDone:
		default:
			return false, false
		}
	} else if dev.State > StateBooting {
		return false, false
	}
	reconnect := dev.State == StateOFF
	dev.Com, dev.requestDone = queue, done
	dev.connectionActive = true
	dev.processExited = false
	dev.CurrentService = 0
	dev.CurrentProgram, dev.NextProgram = nil, nil
	p.setConnectionStateLocked(dev, StateBooting)
	return reconnect, true
}

func (p *Slot) detachConnection(dev *RockDevice, queue chan RequestMessage) {
	p.Lock()
	defer p.Unlock()
	if dev.Com == queue {
		dev.connectionActive = false
		p.setConnectionStateLocked(dev, StateOFF)
	}
}

// waitForDevices は、待機開始前に接続されたデバイスを含め、要求されたすべてのデバイスを待ちます。
// 起動の期限は呼び出し元が指定します。
func (p *Slot) waitForDevices(ctx context.Context, ids []string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.Lock()
		ready := true
		for _, id := range ids {
			if !p.connectedReady[id] {
				ready = false
				break
			}
		}
		if p.connectionChanged == nil {
			p.connectionChanged = make(chan struct{})
		}
		changed := p.connectionChanged
		p.Unlock()
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}

func newRockDevice(id, master string, dev RockDevice) *RockDevice {
	app3 := dev.AppPath3
	opt3 := dev.Option3
	app4 := dev.AppPath4
	opt4 := dev.Option4
	if dev.Hybrid {
		app3 = dev.AppPath1
		opt3 = dev.Option1
		app4 = dev.AppPath2
		opt4 = dev.Option2
	}
	return &RockDevice{
		ID:             id,
		RegistryCode:   dev.RegistryCode,
		AppPath1:       dev.AppPath1,
		Option1:        dev.Option1,
		AppPath2:       dev.AppPath2,
		Option2:        dev.Option2,
		AppPath3:       app3,
		Option3:        opt3,
		AppPath4:       app4,
		Option4:        opt4,
		RecordPath:     dev.RecordPath,
		Recording:      false,
		TunerType:      dev.TunerType,
		Hybrid:         dev.Hybrid,
		Master:         master,
		CurrentService: 0,
		CurrentProgram: nil,
		NextProgram:    nil,
		EventsAt:       0,
		State:          StateOFF,
	}
}

func (dev *RockDevice) BuildHash() {
	md5 := md5.Sum([]byte(dev.ID))
	dev.RegistryCode = hex.EncodeToString(md5[:])
}

// キューは接続に属しています。接続が切断された受信側は、
// サーバー自体が稼働し続けている場合であっても、送信側を解放しなければなりません。
func (p *Slot) sendDevice(dev *RockDevice, rc RequestMessage) bool {
	p.RLock()
	queue, done := dev.Com, dev.requestDone
	p.RUnlock()
	ctx := serviceContext()
	if ctx.Err() != nil {
		return false
	}
	select {
	case <-done:
		return false
	default:
	}
	select {
	case queue <- rc:
		return true
	case <-done:
		return false
	case <-ctx.Done():
		return false
	}
}

func (p *Slot) Send(rc *RequestMessage) int {
	devices := p.matchingDevices(func(dev *RockDevice) bool {
		return dev.State != StateOFF && dev.ID == rc.ID
	})
	sent := 0
	for _, dev := range devices {
		if p.sendDevice(dev, *rc) {
			sent++
		}
	}
	return sent
}

func (p *Slot) SendByRID(rc *RequestMessage) int {
	devices := p.matchingDevices(func(dev *RockDevice) bool {
		return dev.State != StateOFF && dev.RegistryCode == rc.ID
	})
	sent := 0
	for _, dev := range devices {
		if p.sendDevice(dev, *rc) {
			sent++
		}
	}
	return sent
}

func (p *Slot) Add(dev *RockDevice) error {
	p.Lock()
	defer p.Unlock()
	p.addLocked(dev)
	return nil
}

func (p *Slot) addLocked(dev *RockDevice) {
	if _, exists := p.Device[dev.ID]; exists {
		return
	}
	p.Device[dev.ID] = dev
	if !ContainsStr(p.IDs, dev.ID) {
		p.IDs = append(p.IDs, dev.ID)
	}
}

func (p *Slot) Get(id string) (*RockDevice, bool) {
	p.RLock()
	defer p.RUnlock()
	d, ext := p.Device[id]
	return d, ext
}

func (p *Slot) GetByRID(rid string) (*RockDevice, bool) {
	p.RLock()
	defer p.RUnlock()
	for _, v := range p.Device {
		if v.RegistryCode == rid {
			return v, true
		}
	}
	return nil, false
}

func (p *Slot) IsBooting(id string) bool {
	p.RLock()
	defer p.RUnlock()
	v, ext := p.Device[id]
	if ext && v.State > StateOFF {
		return true
	}
	return false
}

func (p *Slot) IsAnyRunning() bool {
	p.RLock()
	defer p.RUnlock()
	for _, dev := range p.Device {
		if dev.State > StateOFF {
			return true
		}
	}
	return false
}

func (p *Slot) IsBootingByRID(rid string) bool {
	p.RLock()
	defer p.RUnlock()
	for _, dev := range p.Device {
		if dev.RegistryCode == rid && dev.State == StateOFF {
			return false
		}
	}
	return true
}

func (p *Slot) IsReady(id string) bool {
	p.RLock()
	defer p.RUnlock()
	v, ext := p.Device[id]
	if ext && v.State >= StateReady {
		return true
	}
	return false
}

func (p *Slot) IsReadyByRID(rid string) bool {
	p.RLock()
	defer p.RUnlock()
	for _, dev := range p.Device {
		if dev.RegistryCode == rid && dev.State < StateReady {
			return false
		}
	}
	return true
}

func (p *Slot) IsCurrentService(id string, sid uint) bool {
	p.RLock()
	defer p.RUnlock()

	dev, ext := p.Device[id]
	if !ext || dev.State < StateReady || dev.CurrentService != sid {
		if dev == nil {
			log.Printf("debug: IS [%s, %d] %v, dev=nil\n", id, sid, ext)
		} else {
			log.Printf("debug: IS [%s, %d] %v,%v,%v\n", id, sid, ext, dev.State, dev.CurrentService)
		}
		return false
	}
	return true
}

func (p *Slot) IsCurentProgram(tvp *TvProgram) bool {
	if tvp == nil {
		return false
	}
	devices := p.matchingDevices(func(dev *RockDevice) bool {
		return dev.CurrentProgram != nil && dev.CurrentProgram.Sid == tvp.Sid && dev.CurrentProgram.Eid == tvp.Eid
	})
	return len(devices) > 0
}

func (p *Slot) matchingDevices(match func(*RockDevice) bool) []*RockDevice {
	p.RLock()
	defer p.RUnlock()

	devices := []*RockDevice{}
	for _, dev := range p.Device {
		if match(dev) && dev.Com != nil {
			devices = append(devices, dev)
		}
	}
	return devices
}

// PFによって省略された詳細情報を同一の放送に限り保持する。
// これには既存のEPGイベントの開始時刻や終了時刻がPFによって修正される場合も含まれる。
func inheritPFDetails(pf, source *TvProgram) {
	if pf == nil || source == nil || pf.Sid != source.Sid || pf.Eid != source.Eid ||
		pf.Start >= source.End || source.Start >= pf.End {
		return
	}
	if pf.Subtitle == "" {
		pf.Subtitle = source.Subtitle
	}
	if pf.Content == "" {
		pf.Content = source.Content
	}
}

func (dev *RockDevice) updatePFProgram(pf *TvProgram, bef, aft int64) (bool, error) {
	if pf.Sid == 0 {
		return false, nil
	}
	// GetDBD用に不明なPF時刻をデバイス上に保持するが、
	// 予約の有効なタイマーを -1（またはその他の無効な間隔）で置き換えてはならない。
	if pf.Start < bef || pf.Start >= aft || pf.End < bef || pf.End >= aft || pf.End <= pf.Start {
		return false, nil
	}

	tvp, err := tvprogram.getStoredProgram(CurrentSlot, pf.Sid, pf.Eid)
	if err != nil {
		return false, err
	}
	// 番組情報がある場合、PFにない詳細を補ってから更新する。
	if len(tvp) > 0 {
		inheritPFDetails(pf, tvp[0])
		if len(pf.Title) > 0 {
			tvp[0].Title = pf.Title
		}
		if len(pf.Subtitle) > 0 {
			tvp[0].Subtitle = pf.Subtitle
		}
		if len(pf.Content) > 0 {
			tvp[0].Content = pf.Content
		}
		tvp[0].Start = pf.Start
		tvp[0].End = pf.End
		tvp[0].CommonSid, tvp[0].CommonEid = pf.CommonSid, pf.CommonEid

		if err := tvprogram.Insert(CurrentSlot, tvp[0]); err != nil {
			return false, err
		}
		return true, nil
	} else {
		// 無い場合は登録
		ntp := &TvProgram{
			Sid:       pf.Sid,
			Eid:       pf.Eid,
			Title:     pf.Title,
			Subtitle:  pf.Subtitle,
			Content:   pf.Content,
			Start:     pf.Start,
			End:       pf.End,
			CnibA:     pf.CnibA,
			CnibB:     pf.CnibB,
			CommonSid: pf.CommonSid,
			CommonEid: pf.CommonEid,
		}
		if err := tvprogram.Insert(CurrentSlot, ntp); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (dev *RockDevice) UpdatePFRecordings(current, next *TvProgram) (bool, error) {
	update := false
	cur := Now()
	bef := cur.Add(-48 * time.Hour).Unix()
	aft := cur.Add(48 * time.Hour).Unix()
	var err error
	var changed bool

	inheritPFDetails(current, dev.CurrentProgram)
	inheritPFDetails(next, dev.NextProgram)

	if dev.CurrentProgram == nil || !dev.CurrentProgram.IsMatchProgram(current) {
		// 未定番組
		if current.Eid >= 0xfff0 && current.Eid <= 0xffff {
			log.Println("info: 緊急番組の検出", current.Sid, current.Eid, current.Title)
		} else {
			changed, err = dev.updatePFProgram(current, bef, aft)
			if err != nil {
				return false, err
			}
			update = update || changed
		}
		dev.CurrentProgram = current
	}

	if dev.NextProgram == nil || !dev.NextProgram.IsMatchProgram(next) {
		// 未定番組
		if next.Eid >= 0xfff0 && next.Eid <= 0xffff {
			log.Println("info: 緊急番組の検出", next.Sid, next.Eid, next.Title)
		} else {
			changed, err = dev.updatePFProgram(next, bef, aft)
			if err != nil {
				return false, err
			}
			update = update || changed
		}
		dev.NextProgram = next
	}

	if update {
		scheduler.UpdateScheduler()
		deviceController.Notify(ControllerUpdateTimeTable, "")
		deviceController.Notify(ControllerSendDevice, dev.RegistryCode)
	}

	return update, err
}
