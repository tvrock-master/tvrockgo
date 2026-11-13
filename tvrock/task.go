package tvrock

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

type ScheduleJobCom uint32

const (
	JobNone ScheduleJobCom = iota
	JobBoot
	JobStreamChange
	JobRecordStart
	JobRecordStop
	JobUpdate
	JobExit
	JobForceExit
)
const (
	schChangeNone   int = iota
	schChangeParam      = 0x01
	schChangeTitle      = 0x02
	schChangeTimer      = 0x04
	schChangeDevice     = 0x08
)

type RecordingJob struct {
	ID          uint64         `json:"id"`       // スケジュールID
	DeviceID    string         `json:"deviceid"` // デバイスID
	Sid         uint           `json:"sid"`      // サービスID
	Eid         uint           `json:"eid"`      // イベントID
	Priority    uint           `json:"priority"` // 優先度
	Title       string         `json:"title"`
	BootType    uint           `json:"boottype"`
	Status      ScheduleJobCom `json:"jobno"`
	ChLock      bool           `json:"-"`
	Complete    bool           `json:"complate"`
	Validate    bool           `json:"validate"`
	ExitMode    uint           `json:"exitmode"`
	CustomComID string         `json:"customcomid"`

	BootTime         time.Time
	StreamChangeTime time.Time
	RecordStartTime  time.Time
	RecordStopTime   time.Time
	ExitTime         time.Time

	deviceLocked bool // RecordingJob.RWMutexによって保護されており、タスク終了まで固定されたままとなる

	Events []func() bool `json:"-"`

	Com          chan ScheduleJobCom `json:"-"`
	sync.RWMutex `json:"-"`
}

type RefreshTaskInterval struct {
	uxtm int64
	rs   *RecordingSchedule
	exec *time.Timer
	sync.RWMutex
}

type DBDInterval struct {
	dbd map[string]*time.Timer
	sync.RWMutex
}

type ShutdownTimer struct {
	shutdownDevice map[string]*time.Timer
	sync.RWMutex
}

type TaskArray struct {
	Job                map[uint64]*RecordingJob
	DisplacedRecording map[string]map[uint64]*RecordingJob
	Launcher           *Launcher
	sync.RWMutex
}

var refreshTaskInterval RefreshTaskInterval
var dbdInterval DBDInterval
var shutdownTimer ShutdownTimer
var tasks TaskArray

func (p *RecordingJob) getStatus() ScheduleJobCom {
	return ScheduleJobCom(atomic.LoadUint32((*uint32)(&p.Status)))
}

func (p *RecordingJob) setStatus(st ScheduleJobCom) {
	atomic.StoreUint32((*uint32)(&p.Status), uint32(st))
}

func (p *RecordingJob) shouldComplete(now time.Time) bool {
	return p.Validate && !now.Before(p.ExitTime)
}

func (p *RecordingJob) MarshalJSON() ([]byte, error) {
	p.RLock()
	defer p.RUnlock()

	type Alias RecordingJob
	return json.Marshal(&struct {
		Status uint32 `json:"jobno"`
		*Alias
	}{
		Status: atomic.LoadUint32((*uint32)(&p.Status)),
		Alias:  (*Alias)(p),
	})
}

func (p *TaskArray) Initialize(l *Launcher) error {

	p.Job = map[uint64]*RecordingJob{}
	p.DisplacedRecording = map[string]map[uint64]*RecordingJob{}
	p.Launcher = l

	dbdInterval.dbd = map[string]*time.Timer{}

	shutdownTimer.shutdownDevice = map[string]*time.Timer{}

	return nil
}

// タスク取得
func (p *TaskArray) Get(id uint64) (*RecordingJob, bool) {
	p.Lock()
	t, ext := p.Job[id]
	p.Unlock()
	return t, ext
}

// タスク取得
func (p *TaskArray) GetWithState(devID string, st ScheduleJobCom) *RecordingJob {
	for _, v := range p.GetArray() {
		v.RLock()
		match := v.DeviceID == devID && v.getStatus() == st
		v.RUnlock()
		if match {
			return v
		}
	}
	return nil
}

// タスクの削除
func (p *TaskArray) Delete(task *RecordingJob) {

	p.Lock()
	t, ext := p.Job[task.ID]
	if !ext || t.ID != task.ID {
		p.Unlock()
		return
	}
	delete(p.Job, task.ID)
	p.Unlock()

	if t.ChLock {
		select {
		case t.Com <- JobExit:
		default:
		}
	}

	log.Println("debug: task delete ", task.DeviceID)
}

// タスクの全削除
func (p *TaskArray) DeleteAll() {

	p.Lock()
	tasks := []*RecordingJob{}
	for _, t := range p.Job {
		tasks = append(tasks, t)
	}
	p.Job = map[uint64]*RecordingJob{}
	p.Unlock()

	for _, t := range tasks {
		if !t.ChLock {
			continue
		}
		select {
		case t.Com <- JobExit:
		default:
		}
	}

	log.Println("debug: task delete all")
}

// タスク総数
func (p *TaskArray) GetNum() int {
	p.RLock()
	n := len(p.Job)
	p.RUnlock()
	return n
}

// GetBootType
func GetBootType(ns *RecordingSchedule) uint {
	if ns.CustomComID == "FetchProgramTS" {
		return DeviceBootTypeFetchProgramTS
	}

	if ns.CustomComID == "FetchProgramBS" {
		return DeviceBootTypeFetchProgramBS
	}

	if ns.CustomComID == "FetchProgramCS" {
		return DeviceBootTypeFetchProgramCS
	}

	if ns.CustomComID == "FetchProgramOS" {
		return DeviceBootTypeFetchProgramOS
	}

	if (ns.Sid >> 16) == uint(TuningSpaceOS) {
		if ns.RecordOnly {
			return DeviceBootTypeOnesegRecord
		}
		if ns.WatchOnly {
			return DeviceBootTypeOnesegWatchOnly
		}
		return DeviceBootTypeOnesegWatch
	}

	if ns.RecordOnly {
		return DeviceBootTypeRecord
	}

	if ns.WatchOnly {
		return DeviceBootTypeWatchOnly
	}

	return DeviceBootTypeWatch
}

func (p *RecordingJob) SetOffsetTimer(ns *RecordingSchedule) *RecordingJob {
	p.BootTime = time.Unix(ns.StartTime-ns.IdleTime, 0)
	p.StreamChangeTime = time.Unix(ns.StartTime-ns.IdleTime, 0)
	p.RecordStartTime = time.Unix(ns.StartTime-ns.ReadyTime, 0)
	p.RecordStopTime = time.Unix(ns.EndTime+ns.TailTime, 0)
	p.ExitTime = time.Unix(ns.EndTime+ns.TailTime, 0)
	return p
}

func (p *RecordingJob) CompareSchedule(ns *RecordingSchedule) int {
	ret := schChangeNone
	if p.DeviceID != ns.DeviceID {
		ret |= schChangeDevice
	}
	bt := GetBootType(ns)
	if p.Priority != ns.Priority ||
		p.BootType != bt ||
		p.Complete != ns.Complete ||
		p.Validate != ns.Validate ||
		p.ExitMode != ns.ExitMode ||
		p.CustomComID != ns.CustomComID {
		ret |= schChangeParam
	}

	if p.Title != ns.Title {
		ret |= schChangeTitle
	}

	btm := time.Unix(ns.StartTime-ns.IdleTime, 0)
	stm := time.Unix(ns.StartTime-ns.IdleTime, 0)
	rtm := time.Unix(ns.StartTime-ns.ReadyTime, 0)
	ptm := time.Unix(ns.EndTime+ns.TailTime, 0)
	etm := time.Unix(ns.EndTime+ns.TailTime, 0)
	if p.BootTime != btm ||
		p.StreamChangeTime != stm ||
		p.RecordStartTime != rtm ||
		p.RecordStopTime != ptm ||
		p.ExitTime != etm {
		ret |= schChangeTimer
	}

	return ret
}

func (p *TaskArray) Add(ns *RecordingSchedule) *RecordingJob {

	p.Lock()
	rj, b := p.Job[ns.ID]
	p.Unlock()

	// 既にある場合
	if b {
		return rj
	}

	ns.NormalizeExitMode()
	rj = &RecordingJob{
		DeviceID:    ns.DeviceID,
		ID:          ns.ID,
		Sid:         ns.Sid,
		Eid:         ns.Eid,
		BootType:    GetBootType(ns),
		Title:       ns.Title,
		Status:      JobNone,
		ChLock:      true,
		Complete:    ns.Complete,
		Validate:    ns.Validate,
		ExitMode:    ns.ExitMode,
		CustomComID: ns.CustomComID,
		Com:         make(chan ScheduleJobCom, 16),
	}

	rj.SetOffsetTimer(ns)

	p.Lock()
	p.Job[ns.ID] = rj
	p.Unlock()

	if !serviceWorkers.start(func() { rj.Exec(p) }) {
		p.Lock()
		delete(p.Job, ns.ID)
		p.Unlock()
	}

	ns.State = JobBoot
	return rj
}

func (p *TaskArray) Modify(ns *RecordingSchedule) *RecordingJob {
	ns.NormalizeExitMode()

	p.Lock()
	v, b := p.Job[ns.ID]
	p.Unlock()

	if b {
		// 判定と反映を起動処理と同じロックで行う
		v.Lock()
		sta := v.CompareSchedule(ns)
		if sta&schChangeDevice != 0 {
			if v.deviceLocked || v.getStatus() != JobNone {
				log.Printf("warn: 実行開始済みタスクのデバイス変更を拒否 id:%d, device:%s, requested:%s", v.ID, v.DeviceID, ns.DeviceID)
				sta &^= schChangeDevice
			} else {
				log.Printf("info: タスクのデバイス変更 id:%d, before:%s, device:%s", v.ID, v.DeviceID, ns.DeviceID)
				v.DeviceID = ns.DeviceID
			}
		}

		if sta != schChangeNone {

			if sta&schChangeParam != 0 {
				v.BootType = GetBootType(ns)
				v.Priority = ns.Priority
				v.Complete = ns.Complete
				v.Validate = ns.Validate
				v.ExitMode = ns.ExitMode
				v.CustomComID = ns.CustomComID
				log.Printf("info: タスク内容の更新 id:%d, device:%s, titlle:%s, boot:%d, priority:%d, complete:%t, validate:%t, exitmode:%d, customcomid:%s",
					v.ID, v.DeviceID, v.Title, v.BootType, v.Priority, v.Complete, v.Validate, v.ExitMode, v.CustomComID)
			}

			if sta&schChangeTitle != 0 {
				bef := v.Title
				v.Title = ns.Title
				log.Printf("info: タスクのタイトル変更 title: id:%d, device:%s, title:%s, before:%s", v.ID, v.DeviceID, v.Title, bef)
			}

			if sta&schChangeTimer != 0 {
				v.SetOffsetTimer(ns)
				log.Printf("info: タスクの時間変更 id:%d, device:%s, title:%s, boot:%s, stream:%s, record:%s, exit:%s",
					v.ID, v.DeviceID, v.Title, v.BootTime, v.StreamChangeTime, v.RecordStartTime, v.ExitTime)
			}

		}
		v.Unlock()
		if sta != schChangeNone {
			select {
			case v.Com <- JobUpdate:
			default:
			}
		}
		return v
	}

	return nil
}

func (p *TaskArray) AddAll(rs []*RecordingSchedule) error {

	for _, v := range rs {
		p.Add(v)
	}

	return nil
}

func (p *TaskArray) GetArray() []*RecordingJob {

	p.Lock()

	ret := []*RecordingJob{}

	for _, v := range p.Job {
		ret = append(ret, v)
	}

	p.Unlock()

	return ret
}

func (p *TaskArray) Update(rs []*RecordingSchedule) error {

	refreshTaskInterval.Lock()
	defer refreshTaskInterval.Unlock()

	now := Now().Unix()
	maxtasks := configure.TaskSetting.MaxTasks
	if maxtasks < 1 {
		maxtasks = 1
	}

	candidates := []*RecordingSchedule{}
	schedulesByID := map[uint64]*RecordingSchedule{}
	for _, sch := range rs {
		schedulesByID[sch.ID] = sch
		if sch.Complete || !sch.Validate || now >= sch.EndTime+sch.TailTime {
			continue
		}
		candidates = append(candidates, sch)
	}

	selected := []*RecordingSchedule{}
	selectedMap := map[uint64]bool{}

	for _, task := range p.GetArray() {
		sch, ok := schedulesByID[task.ID]
		if !ok {
			continue
		}
		if sch.Complete || !sch.Validate || now >= sch.EndTime+sch.TailTime {
			continue
		}
		if task.getStatus() != JobNone || now >= sch.StartTime-sch.IdleTime {
			selected = append(selected, sch)
			selectedMap[sch.ID] = true
		}
	}

	for _, sch := range candidates {
		if len(selected) >= maxtasks {
			break
		}
		if selectedMap[sch.ID] {
			continue
		}
		selected = append(selected, sch)
		selectedMap[sch.ID] = true
	}

	var nextRefresh *RecordingSchedule
	for _, sch := range candidates {
		if selectedMap[sch.ID] {
			continue
		}
		nextRefresh = sch
		break
	}

	for _, task := range p.GetArray() {
		if !selectedMap[task.ID] {
			if sch, ok := schedulesByID[task.ID]; ok && (sch.Complete || !sch.Validate) {
				p.Modify(sch)
			}
			p.Delete(task)
		}
	}

	for _, sch := range selected {
		if t := p.Modify(sch); t != nil {
			continue
		}
		p.Add(sch)
	}

	if nextRefresh != nil {
		updt := nextRefresh.StartTime - nextRefresh.IdleTime
		exectime := max(60, updt-now-int64(configure.TaskSetting.ExecTime))
		if refreshTaskInterval.exec != nil {
			refreshTaskInterval.exec.Stop()
		}
		refreshTaskInterval.uxtm = updt
		refreshTaskInterval.exec = time.AfterFunc(time.Duration(exectime)*time.Second, func() {
			scheduler.UpdateScheduler()
		})
		log.Printf("debug: 他に更新が無い場合のタスクのリフレッシュ時間 %d:%d:%d時間後", exectime/3600, (exectime/60)%60, exectime%60)
	}

	return nil
}

func (p *RecordingJob) IsCurrentRecording() bool {
	v, b := scheduler.GetCurrentRecording(p.DeviceID)
	return b && v.ID == p.ID
}

func (p *TaskArray) BufferDisplacedRecording(devID string, job *RecordingJob) {
	if job == nil || job.getStatus() != JobRecordStart {
		return
	}

	p.Lock()
	defer p.Unlock()

	if p.DisplacedRecording == nil {
		p.DisplacedRecording = map[string]map[uint64]*RecordingJob{}
	}
	if p.DisplacedRecording[devID] == nil {
		p.DisplacedRecording[devID] = map[uint64]*RecordingJob{}
	}
	p.DisplacedRecording[devID][job.ID] = job
	log.Printf("debug: buffered displaced recording: %d, %s", job.ID, devID)
}

func (p *TaskArray) StopBufferedRecordings(devID string, currentID uint64) {
	p.Lock()

	pendingMap := p.DisplacedRecording[devID]
	pending := []*RecordingJob{}
	for id, job := range pendingMap {
		if id == currentID {
			continue
		}
		pending = append(pending, job)
		delete(pendingMap, id)
	}
	if len(pendingMap) == 0 {
		delete(p.DisplacedRecording, devID)
	}

	p.Unlock()

	for _, job := range pending {
		if job.getStatus() != JobRecordStart {
			continue
		}
		if err := job.ForceRecordStop(); err != nil {
			log.Println("error: ", err)
			continue
		}
		job.setStatus(JobRecordStop)
		log.Printf("debug: stopped displaced recording: %d, %s", job.ID, devID)
	}
}

// 呼び出し元はプログラムフェッチジョブを含めジョブのロックを保持していなければならない。
func (p *RecordingJob) AcquireDeviceControl(ta *TaskArray) error {
	prev, err := scheduler.acquireDevice(p)
	if err != nil {
		return err
	}
	if prev != nil && prev.ID != p.ID && ta != nil {
		if job, ext := ta.Get(prev.ID); ext {
			ta.BufferDisplacedRecording(p.DeviceID, job)
			ta.StopBufferedRecordings(p.DeviceID, p.ID)
		}
	}
	return nil
}

func (p *RecordingJob) HasDeviceControl(action string) bool {
	if p.IsCurrentRecording() {
		return true
	}
	log.Printf("debug: job skip %s without device control: %d, %s", action, p.ID, p.DeviceID)
	return false
}

// カスタムコマンド実行
func (p *RecordingJob) ExecCustomCommand(mode CustomCommandExecutionMode) {
	if len(p.CustomComID) < 1 {
		return
	}

	htk := ""
	hts := ""
	recpath := ""
	errors := uint(0)

	sta, ext := station.Get(p.Sid)
	if ext {
		hts = sta.HashTag
	}

	sch, b := scheduler.GetCurrentRecording(p.DeviceID)
	if b {
		recpath = sch.FileName
		errors = sch.Errors
		if sch.SearchID > 0 {
			kr, err := rockDB.GetKeywordReserve(CurrentSlot, uint64(sch.SearchID))
			if err != nil {
				log.Println("error:", err)
			} else {
				htk = kr.HashTag
			}
		}
	}

	for _, v := range configure.CustomCommand {
		if mode == v.Mode && v.ID == p.CustomComID {
			serviceWorkers.start(func() {
				customCommandManager.ExecCustomCommand(&v, p.DeviceID, p.Title, recpath, p.Sid, p.Eid, errors, htk, hts)
			})
		}
	}

	for _, v := range configure.CustomCommand {
		if mode == v.Mode && v.Dependencies == p.CustomComID {
			serviceWorkers.start(func() {
				customCommandManager.ExecCustomCommand(&v, p.DeviceID, p.Title, recpath, p.Sid, p.Eid, errors, htk, hts)
			})
		}
	}
}

func (p *RecordingJob) ExistDeviceControl(action string) bool {
	_, b := scheduler.GetCurrentRecording(p.DeviceID)
	return b
}

func (p *RecordingJob) Exec(ta *TaskArray) {

	p.RLock()
	log.Printf("info: タスクの起動 id:%d, device:%s, title:%s, sid:%d, eid:%d, %s, %s", p.ID, p.DeviceID, p.Title, p.Sid, p.Eid, p.BootTime.String(), p.ExitTime.String())
	complete := p.Complete
	p.RUnlock()

	defer func() {
		// 送信側がこのキューを保持し続けている可能性があるため受信側で決してクローズしてはならない
		ta.Lock()
		if current, ok := ta.Job[p.ID]; ok && current == p {
			delete(ta.Job, p.ID)
		}
		ta.Unlock()
		scheduler.releaseDevice(p)
		log.Println("debug: タスクの終了", p.ID, p.DeviceID, p.Title, p.Sid, p.Eid)
	}()

	if serviceContext().Err() != nil || complete {
		return
	}

	// 番組情報取得へ移行
	if p.BootType >= DeviceBootTypeFetchProgramTS && p.BootType <= DeviceBootTypeFetchProgramOS {
		p.ExecFetchProgram(ta)
		scheduler.DeleteCurrentRecording(p.DeviceID, p.ID)
		if p.getStatus() == JobExit {
			if err := scheduler.Complete(CurrentSlot, p.ID); err != nil {
				log.Println("error: ", err)
			}
			deviceController.Notify(ControllerUpdateSchedule, "")
		}
		return
	}

	ctx, cancel := context.WithCancel(globalContext)
	defer cancel()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	done := p.RunDueActions(Now(), ta)
L:
	for !done {
		select {
		case <-ctx.Done():
			log.Println("info: 終了通知を検出")
			break L

		case no := <-p.Com:

			switch no {
			case JobUpdate:
				log.Println("debug: job update", p.ID)
				if p.RunDueActions(Now(), ta) {
					break L
				}

			case JobExit:
				if p.getStatus() == JobRecordStart {
					if err := p.RecordStop(); err != nil {
						log.Println("error: ", err)
					}
				}
				p.RequestShutdown()
				p.setStatus(JobExit)
				log.Println("debug: job exit", p.Title)
				break L

			case JobForceExit:
				if p.getStatus() == JobRecordStart {
					if err := p.RecordStop(); err != nil {
						log.Println("error: ", err)
					}
				}
				p.RequestShutdown()
				p.setStatus(JobForceExit)
				log.Println("debug: job force exit", p.Title)
				break L
			}

		case now := <-ticker.C:
			if p.RunDueActions(now, ta) {
				break L
			}
		}
	}

	// カスタムコマンド実行(後)
	p.ExecCustomCommand(CustomCommandExecutionModeRecordStop)

	scheduler.DeleteCurrentRecording(p.DeviceID, p.ID)
	if ctx.Err() == nil {
		serviceWorkers.start(func() { queueRecordingTSAnalysis(CurrentSlot, p.ID) })
	}
	if p.getStatus() == JobExit {
		if p.shouldComplete(Now()) {
			if err := scheduler.Complete(CurrentSlot, p.ID); err != nil {
				log.Println("error: ", err)
			}
		}
		deviceController.Notify(ControllerUpdateSchedule, "")
	}

}

func (p *RecordingJob) GetDBD() (bool, bool) {
	tbds := false
	tbde := false
	var tvp *TvProgram

	dev, b := slot.Get(p.DeviceID)
	if !b || dev == nil {
		return tbds, tbde
	}

	if dev.CurrentProgram != nil && dev.CurrentProgram.Sid == p.Sid && dev.CurrentProgram.Eid == p.Eid {
		tvp = dev.CurrentProgram
	} else if dev.NextProgram != nil && dev.NextProgram.Sid == p.Sid && dev.NextProgram.Eid == p.Eid {
		tvp = dev.NextProgram
	}

	if tvp != nil && tvp.Sid == p.Sid && tvp.Eid == p.Eid {
		tbds = tvp.Start == -1
		tbde = tvp.End == -1

		if tbds || tbde {

			dbdInterval.Lock()

			if dbdInterval.dbd[p.DeviceID] == nil {
				log.Printf("info: 時間未定の検出 id:%d, sid:%d, eid:%d, %s, start: %t, end:%t",
					p.ID, p.Sid, p.Eid, tvp.Title, tbds, tbde)

				// 待機中の再割り当て後も、検出時のデバイスのタイマーを解除する。
				deviceID := p.DeviceID
				dbdInterval.dbd[deviceID] = time.AfterFunc(60*time.Second, func() {
					dbdInterval.Lock()
					if dbdInterval.dbd[deviceID] != nil {
						dbdInterval.dbd[deviceID].Stop()
						delete(dbdInterval.dbd, deviceID)
					}
					dbdInterval.Unlock()
				})
			}

			dbdInterval.Unlock()
		}
	}

	return tbds, tbde
}

func (p *RecordingJob) NearProgram() bool {
	dev, b := slot.Get(p.DeviceID)
	return b && dev != nil &&
		((dev.CurrentProgram != nil && dev.CurrentProgram.Sid == p.Sid && dev.CurrentProgram.Eid == p.Eid) ||
			(dev.NextProgram != nil && dev.NextProgram.Sid == p.Sid && dev.NextProgram.Eid == p.Eid))
}

func (p *RecordingJob) EventBoot(now time.Time, ta *TaskArray) bool {
	if !now.Before(p.BootTime) {
		if err := p.AcquireDeviceControl(ta); err != nil {
			log.Println("error: ", err)
			return true
		}

		if err := p.Boot(); err != nil {
			log.Println("error: ", err)
			return true
		}

		// カスタムコマンド実行(待機)
		p.ExecCustomCommand(CustomCommandExecutionModeIdleStart)

		p.setStatus(JobBoot)
	}
	return false
}

func (p *RecordingJob) EventDeviceStandby(now time.Time) bool {
	if !now.Before(p.StreamChangeTime) {
		if !slot.IsReady(p.DeviceID) {
			return false
		}
		p.setStatus(JobStreamChange)
	}
	return false
}

func (p *RecordingJob) EventStreamChange(now time.Time, tbds bool) bool {
	if !p.IsCurrentService() {
		if err := p.StreamChange(); err != nil {
			log.Println("error: ", err)
		}
		return false
	}

	if !tbds && !now.Before(p.RecordStartTime) {
		if err := p.RecordStart(); err != nil {
			log.Println("error: ", err)
		}
		p.setStatus(JobRecordStart)
	}
	return false
}

func (p *RecordingJob) EventRecordStart(now time.Time, tbde bool) bool {

	// 途中でRecord状態に切り替わった場合直ちに録画を開始する
	if dev, b := slot.Get(p.DeviceID); b &&
		dev.State == StateReady &&
		(tbde || now.Before(p.RecordStopTime)) &&
		p.BootType != DeviceBootTypeWatchOnly &&
		p.BootType != DeviceBootTypeOnesegWatchOnly {
		// 再接続後は初期サービスになっている場合があるため、予約局への選局を確認する。
		if !p.IsCurrentService() {
			p.setStatus(JobStreamChange)
			return false
		}
		if err := p.RecordStart(); err != nil {
			log.Println("error: ", err)
		}
	}

	if !tbde && !now.Before(p.RecordStopTime) {
		if err := p.RecordStop(); err != nil {
			log.Println("error: ", err)
		}
		p.setStatus(JobRecordStop)
	}
	return false
}

func (p *RecordingJob) EventRecordStop(now time.Time, tbde bool) bool {
	if !tbde && !now.Before(p.ExitTime) {
		p.RequestShutdown()
		p.setStatus(JobExit)
	}
	return false
}

func (p *RecordingJob) RequestShutdown() {
	// 未開始の予約の削除で、同じデバイスの手動視聴を終了させない。
	if p.ExitMode != ExitModeShutDown || p.getStatus() == JobNone {
		return
	}

	shutdownTimer.Lock()
	defer shutdownTimer.Unlock()

	req := shutdownTimer.shutdownDevice[p.DeviceID]
	if req != nil {
		req.Stop()
	}

	delay := time.Duration(configure.DeviceShutdownTime) * time.Second
	request := &deviceShutdownRequest{
		deviceID: p.DeviceID, jobID: p.ID,
		deadline: time.Now().Add(delay + 2*time.Minute),
	}
	request.timer = time.AfterFunc(delay, request.run)
	shutdownTimer.shutdownDevice[p.DeviceID] = request.timer
}

func (p *RecordingJob) ShutdownCancel() bool {
	shutdownTimer.Lock()
	defer shutdownTimer.Unlock()

	req := shutdownTimer.shutdownDevice[p.DeviceID]
	if req == nil {
		return false
	}
	req.Stop()
	delete(shutdownTimer.shutdownDevice, p.DeviceID)
	return true
}

func (p *RecordingJob) RunDueActions(now time.Time, ta *TaskArray) bool {
	p.Lock()
	defer p.Unlock()

	status := p.getStatus()

	// タスク実行中に時間が変わることがあるので柔軟に対応する

	// タスクが無効な場合
	if !p.Validate || p.Complete {
		if status != JobNone {
			if status == JobRecordStart {
				if err := p.RecordStop(); err != nil {
					log.Println("error: ", err)
				}
				p.setStatus(JobRecordStop)
				return false
			}
			p.RequestShutdown()
		}

		if p.Complete {
			p.setStatus(JobExit)
		} else {
			p.setStatus(JobForceExit)
		}
		scheduler.DeleteCurrentRecording(p.DeviceID, p.ID)
		return true
	}

	// 時間未定の検出
	tbds, tbde := p.GetDBD()

	// 開始時間が未来の場合
	delay := now.Add(3 * time.Minute)
	if delay.Before(p.BootTime) {
		switch status {
		case JobBoot, JobStreamChange, JobRecordStop:
			if !slot.IsReady(p.DeviceID) {
				break
			}
			p.RequestShutdown()

		case JobRecordStart:
			if !slot.IsReady(p.DeviceID) {
				break
			}
			if err := p.RecordStop(); err != nil {
				log.Println("error: ", err)
			}
			p.setStatus(JobRecordStop)
			return false
		}
		scheduler.DeleteCurrentRecording(p.DeviceID, p.ID)
		p.setStatus(JobNone)
		return false
	}

	// 既に終了していれば抜ける
	if !tbds && !tbde && Now().After(p.ExitTime) && !p.NearProgram() {
		p.setStatus(JobExit)
	}

	switch status {

	case JobNone:
		return p.EventBoot(now, ta)

	case JobBoot:
		return p.EventDeviceStandby(now)

	case JobStreamChange:
		return p.EventStreamChange(now, tbds)

	case JobRecordStart:
		return p.EventRecordStart(now, tbde)

	case JobRecordStop:
		return p.EventRecordStop(now, tbde)

	case JobExit, JobForceExit:
		return true
	}

	return false
}

func (p *RecordingJob) Boot() error {

	if !p.HasDeviceControl("boot") {
		log.Printf("info: デバイス操作の権限が無いので起動要求をスキップ %d,%s", p.ID, p.Title)
		return nil
	}

	if len(p.DeviceID) < 1 {
		return fmt.Errorf("不正なデバイス id=%d,deviceid=%s", p.ID, p.DeviceID)
	}

	if !slot.IsReady(p.DeviceID) {

		// デバイスの起動リクエスト
		launcher.Exec(&LaunchMessage{
			RegistryID: "",
			DeviceID:   p.DeviceID,
			BootType:   p.BootType,
			Sid:        p.Sid,
		})

		// 起動待ち
		isBoot := false
		for i := 0; i < configure.DeviceBootupTimeout; i++ {
			isBoot = slot.IsReady(p.DeviceID)
			if isBoot {
				return nil
			}
			log.Println("debug: job ", p.DeviceID, " booting wait ", i, "sec.")
			time.Sleep(1 * time.Second)
		}
		if !isBoot {
			return fmt.Errorf("デバイスの起動に失敗しました deviceid=%s", p.DeviceID)
		}

	} else {
		log.Println("info: 既にデバイスは起動中なので起動をスキップ", p.DeviceID)
	}

	log.Println("debug: job boot " + p.DeviceID + "/, " + p.Title)
	return nil
}

func (p *RecordingJob) IsCurrentService() bool {
	dev, ext := slot.Get(p.DeviceID)
	if ext {
		log.Printf("debug: current service: device:%s current:%d target:%d", p.DeviceID, dev.CurrentService, p.Sid)
		return dev.CurrentService == p.Sid
	}
	return false
}

func (p *RecordingJob) StreamChange() error {

	if !p.HasDeviceControl("stream change") || p.IsCurrentService() {
		return nil
	}

	// 親がある場合、子->親の順で渡す
	sid := p.Sid
	for _, v := range station.GetStationList() {
		if v.Sid == p.Sid {
			if v.Parent != 0 && v.Parent != v.Sid {
				slot.Send(&RequestMessage{
					ID:      p.DeviceID,
					Command: "StreamChange",
					Message: fmt.Sprintf("%d", SubChannnelCode<<16|sid&0xffff),
				})
				sid = v.Parent
			}
			break
		}
	}

	slot.Send(&RequestMessage{
		ID:      p.DeviceID,
		Command: "StreamChange",
		Message: fmt.Sprintf("%d", sid),
	})

	log.Printf("debug: job stream change id:%d, sid:%d", p.ID, p.Sid)
	return nil
}

func (p *RecordingJob) RecordStart() error {
	if p.BootType == DeviceBootTypeWatchOnly ||
		p.BootType == DeviceBootTypeOnesegWatchOnly {
		return nil
	}

	if !p.HasDeviceControl("record start") {
		log.Printf("info: デバイス操作の権限が無いので録画開始要求をスキップ %d,%s", p.ID, p.Title)
		return nil
	}

	dev, ext := slot.Get(p.DeviceID)
	if !ext {
		return fmt.Errorf("デバイスが見つかりません %s", p.DeviceID)
	}

	setting := CurrentRecorderSetting()
	fname := SanitizeRecordFilePath(setting.FilenameEx,
		dev.RecordPath,
		p.Title,
		setting.RecordFileExtension,
		p.DeviceID, p.Sid, p.Eid)

	slot.Send(&RequestMessage{
		ID:      p.DeviceID,
		Command: "RecordStart",
		Message: fname,
	})

	// ファイル名登録
	if sch, b := scheduler.GetCurrentRecording(dev.ID); b {
		sch.FileName = fname
		if err := scheduler.FileNameEx(CurrentSlot, sch.ID, sch.FileName); err != nil {
			return err
		}
	}

	log.Printf("info: 録画の開始を要求 %d,%s, device:%s", p.ID, p.Title, p.DeviceID)

	// カスタムコマンド実行(前)
	p.ExecCustomCommand(CustomCommandExecutionModeRecordStart)
	return nil
}

func (p *RecordingJob) RecordStop() error {
	if p.BootType == DeviceBootTypeWatchOnly ||
		p.BootType == DeviceBootTypeOnesegWatchOnly {
		return nil
	}

	// エラー数を書き込む
	dev, b := slot.Get(p.DeviceID)
	if b && dev != nil {
		if err := scheduler.AddErrors(CurrentSlot, p.ID, dev.DeviceInfo.Error); err != nil {
			log.Println("error:", err)
		}
	}

	if !p.HasDeviceControl("record stop") {
		log.Printf("info: デバイス操作の権限が無いので録画停止要求をスキップ %d,%s", p.ID, p.Title)
		return nil
	}

	return p.sendRecordStop()
}

func (p *RecordingJob) ForceRecordStop() error {
	if p.BootType == DeviceBootTypeWatchOnly ||
		p.BootType == DeviceBootTypeOnesegWatchOnly {
		return nil
	}
	return p.sendRecordStop()
}

func (p *RecordingJob) sendRecordStop() error {
	slot.Send(&RequestMessage{
		ID:      p.DeviceID,
		Command: "RecordStop",
		Message: "",
	})

	log.Printf("info: 録画の停止を要求 %d,%s", p.ID, p.Title)
	return nil
}

func (p *RecordingJob) Exit() error {
	if remoteWatching.activeDevice(p.DeviceID) {
		log.Printf("info: デバイス操作の権限が無いので終了要求をスキップ %d,%s", p.ID, p.Title)
		return nil
	}

	// デバイスコントローラーがあれば破棄
	if p.ExistDeviceControl("exit") {
		log.Printf("info: デバイス操作が不明なので終了要求をスキップ %d,%s", p.ID, p.Title)
		return nil
	}

	sent := slot.Send(&RequestMessage{
		ID:      p.DeviceID,
		Command: "PowerOff",
		Message: "",
	})
	if sent == 0 {
		return fmt.Errorf("デバイス終了指示を送信できませんでした id:%d device:%s", p.ID, p.DeviceID)
	}

	log.Printf("info: デバイスの終了を要求 %d,%s", p.ID, p.Title)
	return nil
}

func (p *TaskArray) GetTaskArray() []*RecordingJob {
	p.Lock()

	ret := []*RecordingJob{}
	for _, ts := range p.Job {
		ret = append(ret, ts)
	}

	p.Unlock()

	return ret
}
