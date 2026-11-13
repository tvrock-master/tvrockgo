package tvrock

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"sync"
	"time"
)

const MaxScheduleBuffer = 8192

const (
	ExitModeNone     = 0
	ExitModeShutDown = 1
)

const (
	SchdulerComUpdate uint = iota + 1
	SchdulerComAdd
	SchdulerComModify
	SchdulerComStop
	SchdulerComRemove
)

type RecordingSchedule struct {
	ID uint64 `json:"id"` // スケジュールID

	DeviceID    string `json:"deviceid"` // デバイスID
	Title       string `json:"title"`
	CustomComID string `json:"customcomid"`
	FileName    string `json:"filename"`

	Validate   bool `json:"validate"`   // 予約有効
	WatchOnly  bool `json:"watchonly"`  // 視聴のみ
	RecordOnly bool `json:"recordonly"` // 録画のみ
	ShutDown   bool `json:"shutdown"`   // 終了後アプリシャットダウン
	Complete   bool `json:"complete"`   // 正常終了
	EpgFollow  bool `json:"epgfollow"`  // 時間追従
	AutoSelect bool `json:"autoselect"` // 自動デバイス選択

	StartTime  int64 `json:"starttime"`  // 録画開始時間
	EndTime    int64 `json:"endtime"`    // 録画終了時間
	IdleTime   int64 `json:"idletime"`   // 待機時間（秒）
	ReadyTime  int64 `json:"readytime"`  // 録画開始時間（秒）
	TailTime   int64 `json:"tailtime"`   // 録画終了時間（秒）
	LastUpdate int64 `json:"lastupdate"` // 最終更新時間

	Sid      uint `json:"sid"`      // サービスID
	Eid      uint `json:"eid"`      // イベントID
	SearchID uint `json:"searchid"` // 検索ID
	Priority uint `json:"priority"`
	ExitMode uint `json:"exitmode"`
	No       uint `json:"no"` // 録画番号
	Errors   uint `json:"errors"`

	State     ScheduleJobCom `json:"state"` // 状態
	tDeviceID string         `json:"-"`     // DeviceID自動選択用のテンポラリ
}

type RecordingStatus struct {
	Recording           []uint64                     `json:"recording"` // 録画中のスケジュールID
	Current             map[string]RecordingSchedule `json:"current"`   // 実行中タスクのデバイス別予約
	CurrentMilliseconds int64                        `json:"currentmilliseconds"`
}

type SchedulerRequest struct {
	Com      uint
	ID       uint64
	Schedule []*RecordingSchedule
}

type Scheduler struct {
	CurrentRecording map[string]*RecordingSchedule
	RecordingStatus  RecordingStatus
	Launcher         *Launcher
	db               *RockDB
	tvp              *TvProgramStore
	chUpdate         chan SchedulerRequest
	queueMu          sync.RWMutex
	sync.RWMutex
	muKeywordReserve  sync.Mutex                  // stage 2: キーワード予約の並列実行を防止する
	muEventRelay      sync.Mutex                  // デバイス間におけるリレーの検索および挿入を直列化する
	deviceMu          sync.Mutex                  // デバイスの取得に伴う割り当ての永続化を直列化する
	deviceAssignments map[uint64]deviceAssignment // Scheduler.RWMutexによって保護されている
}

var scheduler Scheduler

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (p *Scheduler) Initialize(db *RockDB, tvp *TvProgramStore, l *Launcher) error {
	p.CurrentRecording = map[string]*RecordingSchedule{}
	p.RecordingStatus = RecordingStatus{}
	p.deviceAssignments = map[uint64]deviceAssignment{}
	p.db = db
	p.tvp = tvp
	p.Launcher = l
	return nil
}

func (p *Scheduler) New() *RecordingSchedule {
	now := Now().Unix()
	rs := RecordingSchedule{
		ID:          0,
		DeviceID:    "",
		Title:       "no title",
		CustomComID: "",

		Validate:   true,
		WatchOnly:  false,
		RecordOnly: true,
		ShutDown:   true,
		Complete:   false,
		EpgFollow:  true,
		AutoSelect: true,

		StartTime:  now,
		EndTime:    now + (30 * 60),
		IdleTime:   60,
		ReadyTime:  30,
		TailTime:   0,
		LastUpdate: now,

		Sid:      0,
		Eid:      0,
		SearchID: 0,
		Priority: 0,
		ExitMode: ExitModeShutDown,
		Errors:   0,

		State:     0,
		tDeviceID: "",
	}

	return &rs
}

func (p *RecordingSchedule) NormalizeExitMode() {
	if p.ShutDown {
		p.ExitMode = ExitModeShutDown
		return
	}
	p.ExitMode = ExitModeNone
}

func (p *Scheduler) Insert(no int, rs ...*RecordingSchedule) (int, error) {
	count := 0

	tx, err := p.db.GetData(no).Begin()
	if err != nil {
		return count, err
	}

	for _, v := range rs {
		v.NormalizeExitMode()

		res, err := tx.Exec(
			`INSERT INTO schedule (deviceid,title,customcomid,filename,`+
				`validate,watchonly,recordonly,shutdown,complete,epgfollow,autoselect,`+
				`starttime,endtime,idletime,readytime,tailtime,lastupdate,`+
				`sid,eid,searchid,priority,exitmode,no,errors) VALUES`+
				`(?,?,?,? ,?,?,?,?,?,?,? ,?,?,?,?,?,? ,?,?,?,?,?,?,?)`,

			v.DeviceID,
			v.Title,
			v.CustomComID,
			v.FileName,

			boolToInt(v.Validate),
			boolToInt(v.WatchOnly),
			boolToInt(v.RecordOnly),
			boolToInt(v.ShutDown),
			boolToInt(v.Complete),
			boolToInt(v.EpgFollow),
			boolToInt(v.AutoSelect),

			v.StartTime,
			v.EndTime,
			v.IdleTime,
			v.ReadyTime,
			v.TailTime,
			v.LastUpdate,

			v.Sid,
			v.Eid,
			v.SearchID,
			v.Priority,
			v.ExitMode,
			v.No,
			v.Errors,
		)

		if err != nil {
			tx.Rollback()
			return count, err
		}

		affected, err := res.RowsAffected()
		if err != nil {
			tx.Rollback()
			return count, err
		}

		count += int(affected)
	}

	return count, tx.Commit()
}

func (p *Scheduler) Update(no int, rs *RecordingSchedule) error {
	p.deviceMu.Lock()
	defer p.deviceMu.Unlock()

	if err := p.preserveDeviceAssignment(no, rs); err != nil {
		return err
	}
	rs.NormalizeExitMode()

	tx, err := p.db.GetData(no).Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`UPDATE schedule SET deviceid=?,title=?,customcomid=?,filename=?,`+
			`validate=?,watchonly=?,recordonly=?,shutdown=?,complete=?,epgfollow=?,autoselect=?,`+
			`starttime=?,endtime=?,idletime=?,readytime=?,tailtime=?,lastupdate=?,`+
			`sid=?,eid=?,searchid=?,priority=?,exitmode=?,no=?,errors=? WHERE id=?`,

		rs.DeviceID,
		rs.Title,
		rs.CustomComID,
		rs.FileName,

		boolToInt(rs.Validate),
		boolToInt(rs.WatchOnly),
		boolToInt(rs.RecordOnly),
		boolToInt(rs.ShutDown),
		boolToInt(rs.Complete),
		boolToInt(rs.EpgFollow),
		boolToInt(rs.AutoSelect),

		rs.StartTime,
		rs.EndTime,
		rs.IdleTime,
		rs.ReadyTime,
		rs.TailTime,
		rs.LastUpdate,

		rs.Sid,
		rs.Eid,
		rs.SearchID,
		rs.Priority,
		rs.ExitMode,
		rs.No,
		rs.Errors,

		rs.ID,
	)

	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (p *Scheduler) UpdateSchedules(no int, rs ...*RecordingSchedule) error {
	p.deviceMu.Lock()
	defer p.deviceMu.Unlock()
	return p.updateSchedulesLocked(no, rs...)
}

// トランザクションがコミットされるまでdeviceMuを保持しておく必要がある
func (p *Scheduler) updateSchedulesLocked(no int, rs ...*RecordingSchedule) error {
	for _, sch := range rs {
		if err := p.preserveDeviceAssignment(no, sch); err != nil {
			return err
		}
	}

	tx, err := p.db.GetData(no).Begin()
	if err != nil {
		return err
	}

	for _, v := range rs {
		v.NormalizeExitMode()

		_, err := tx.Exec(
			`UPDATE schedule SET deviceid=?,title=?,customcomid=?,filename=?,`+
				`validate=?,watchonly=?,recordonly=?,shutdown=?,complete=?,epgfollow=?,autoselect=?,`+
				`starttime=?,endtime=?,idletime=?,readytime=?,tailtime=?,lastupdate=?,`+
				`sid=?,eid=?,searchid=?,priority=?,exitmode=?,no=?,errors=? WHERE id=?`,

			v.DeviceID,
			v.Title,
			v.CustomComID,
			v.FileName,

			boolToInt(v.Validate),
			boolToInt(v.WatchOnly),
			boolToInt(v.RecordOnly),
			boolToInt(v.ShutDown),
			boolToInt(v.Complete),
			boolToInt(v.EpgFollow),
			boolToInt(v.AutoSelect),

			v.StartTime,
			v.EndTime,
			v.IdleTime,
			v.ReadyTime,
			v.TailTime,
			v.LastUpdate,

			v.Sid,
			v.Eid,
			v.SearchID,
			v.Priority,
			v.ExitMode,
			v.No,
			v.Errors,

			v.ID,
		)

		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

func (p *Scheduler) Delete(no int, id uint64) error {
	_, err := p.db.GetData(no).Exec(`DELETE FROM schedule WHERE id=?`, id)
	if err != nil {
		return err
	}
	return nil
}

func (p *Scheduler) DeleteBySearchID(no int, searchID uint) error {
	_, err := p.db.GetData(no).Exec(`DELETE FROM schedule WHERE searchid=?`, searchID)
	if err != nil {
		return err
	}
	return nil
}

func (p *Scheduler) Complete(no int, id uint64) error {
	_, err := p.db.GetData(no).Exec(`UPDATE schedule SET complete=1 WHERE id=?`, id)
	return err
}

func (p *Scheduler) FileNameEx(no int, id uint64, filename string) error {
	_, err := p.db.GetData(no).Exec(`UPDATE schedule SET filename=? WHERE id=?`, filename, id)
	if err == nil {
		deviceController.Notify(ControllerUpdateSchedule, "")
	}
	return err
}

func (p *Scheduler) AddErrors(no int, id uint64, ect uint) error {
	_, err := p.db.GetData(no).Exec(`UPDATE schedule SET errors=errors+? WHERE id=?`, ect, id)
	return err
}

func (p *Scheduler) GetRecordingSchedule(no int, id uint64) (*RecordingSchedule, error) {
	rows, err := p.db.GetData(no).Query("SELECT id,deviceid,title,customcomid,filename,"+
		"validate,watchonly,recordonly,shutdown,complete,epgfollow,autoselect,"+
		"starttime,endtime,idletime,readytime,tailtime,lastupdate,"+
		"sid,eid,searchid,priority,exitmode,no,errors"+
		" FROM schedule WHERE id=?", id)

	if err == sql.ErrNoRows {
	} else if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rs *RecordingSchedule
	if rows.Next() {
		rs = &RecordingSchedule{}
		var fileName sql.NullString
		err = rows.Scan(
			&rs.ID,
			&rs.DeviceID,
			&rs.Title,
			&rs.CustomComID,
			&fileName,

			&rs.Validate,
			&rs.WatchOnly,
			&rs.RecordOnly,
			&rs.ShutDown,
			&rs.Complete,
			&rs.EpgFollow,
			&rs.AutoSelect,

			&rs.StartTime,
			&rs.EndTime,
			&rs.IdleTime,
			&rs.ReadyTime,
			&rs.TailTime,
			&rs.LastUpdate,

			&rs.Sid,
			&rs.Eid,
			&rs.SearchID,
			&rs.Priority,
			&rs.ExitMode,
			&rs.No,
			&rs.Errors,
		)
		if err != nil {
			return nil, err
		}
		if fileName.Valid {
			rs.FileName = fileName.String
		}
	}

	return rs, nil
}

func (p *Scheduler) GetSchedules(no int, st time.Time, ed time.Time, limit int64) ([]*RecordingSchedule, error) {
	lm := ""
	if limit > 0 {
		lm = " LIMIT " + strconv.FormatInt(limit, 10)
	}

	rows, err := p.db.GetData(no).Query("SELECT id,deviceid,title,customcomid,filename,"+
		"validate,watchonly,recordonly,shutdown,complete,epgfollow,autoselect,"+
		"starttime,endtime,idletime,readytime,tailtime,lastupdate,"+
		"sid,eid,searchid,priority,exitmode,no,errors"+
		" FROM schedule WHERE starttime<? AND endtime>? ORDER BY starttime"+lm, ed.Unix(), st.Unix())

	list := []*RecordingSchedule{}
	if err == sql.ErrNoRows {
		return list, nil
	} else if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		rs := RecordingSchedule{}
		var fileName sql.NullString
		err = rows.Scan(
			&rs.ID,
			&rs.DeviceID,
			&rs.Title,
			&rs.CustomComID,
			&fileName,

			&rs.Validate,
			&rs.WatchOnly,
			&rs.RecordOnly,
			&rs.ShutDown,
			&rs.Complete,
			&rs.EpgFollow,
			&rs.AutoSelect,

			&rs.StartTime,
			&rs.EndTime,
			&rs.IdleTime,
			&rs.ReadyTime,
			&rs.TailTime,
			&rs.LastUpdate,

			&rs.Sid,
			&rs.Eid,
			&rs.SearchID,
			&rs.Priority,
			&rs.ExitMode,
			&rs.No,
			&rs.Errors,
		)
		if err != nil {
			return nil, err
		}
		if fileName.Valid {
			rs.FileName = fileName.String
		}

		list = append(list, &rs)
	}

	return list, nil
}

func (p *Scheduler) GetRecordingScheduleFromEID(no int, sid, eid uint) (*RecordingSchedule, error) {
	rows, err := p.db.GetData(no).Query("SELECT id,deviceid,title,customcomid,filename,"+
		"validate,watchonly,recordonly,shutdown,complete,epgfollow,autoselect,"+
		"starttime,endtime,idletime,readytime,tailtime,lastupdate,"+
		"sid,eid,searchid,priority,exitmode,no,errors"+
		" FROM schedule WHERE sid=? AND eid=?", sid, eid)

	if err == sql.ErrNoRows {
	} else if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rs *RecordingSchedule
	if rows.Next() {
		rs = &RecordingSchedule{}
		var fileName sql.NullString
		err = rows.Scan(
			&rs.ID,
			&rs.DeviceID,
			&rs.Title,
			&rs.CustomComID,
			&fileName,

			&rs.Validate,
			&rs.WatchOnly,
			&rs.RecordOnly,
			&rs.ShutDown,
			&rs.Complete,
			&rs.EpgFollow,
			&rs.AutoSelect,

			&rs.StartTime,
			&rs.EndTime,
			&rs.IdleTime,
			&rs.ReadyTime,
			&rs.TailTime,
			&rs.LastUpdate,

			&rs.Sid,
			&rs.Eid,
			&rs.SearchID,
			&rs.Priority,
			&rs.ExitMode,
			&rs.No,
			&rs.Errors,
		)
		if err != nil {
			return nil, err
		}
		if fileName.Valid {
			rs.FileName = fileName.String
		}
	}

	return rs, nil
}

func (p *Scheduler) GetScheduleAll() ([]*RecordingSchedule, error) {
	t1 := Now()
	t1 = t1.Add(-24 * time.Hour)
	t2 := t1.Add(14 * 24 * time.Hour)
	sch, err := p.GetSchedules(CurrentSlot, t1, t2, MaxScheduleBuffer)
	if err != nil {
		return nil, err
	}
	return sch, nil
}

func (p *Scheduler) RecordingNow(id string, sid, eid uint) error {
	b, err := p.IfWatchingToRecording(sid, eid) // エラー無視
	if b {
		return nil
	}

	tvp, err := p.tvp.Get(CurrentSlot, sid, eid)
	if err != nil {
		return err
	}

	now := Now().Unix()
	if len(tvp) > 0 {
		prg := tvp[0]
		if prg.Start <= now && now < prg.End {
			rs := p.New()
			rs.DeviceID = id
			rs.Title = prg.Title
			rs.StartTime = prg.Start
			rs.EndTime = prg.End
			rs.Sid = prg.Sid
			rs.Eid = prg.Eid
			rs.ShutDown = false
			rs.AutoSelect = false
			if _, err := p.Insert(CurrentSlot, rs); err != nil {
				return err
			}
		}
	} else {
		rs := p.New()
		rs.DeviceID = id
		rs.Title = "タイトル未定"
		rs.StartTime = now
		rs.EndTime = now + (30 * 60)
		rs.Sid = sid
		rs.Eid = eid
		rs.ShutDown = false
		rs.AutoSelect = false
		if _, err := p.Insert(CurrentSlot, rs); err != nil {
			return err
		}
	}

	p.UpdateScheduler()

	return nil
}

func (p *Scheduler) GetCurrentRecording(id string) (*RecordingSchedule, bool) {
	p.Lock()
	v, ext := p.CurrentRecording[id]
	p.Unlock()
	return v, ext
}

func (p *Scheduler) GetCurrentRecordings() int {
	p.Lock()
	n := len(p.CurrentRecording)
	p.Unlock()
	return n
}

func (p *Scheduler) SetCurrentRecording(id string, sch *RecordingSchedule) error {
	_, err := p.ReplaceCurrentRecording(id, sch)
	return err
}

func (p *Scheduler) ReplaceCurrentRecording(id string, sch *RecordingSchedule) (*RecordingSchedule, error) {
	p.Lock()

	v, ext := p.CurrentRecording[id]
	if ext && v.ID == sch.ID {
		p.Unlock()
		return nil, nil
	}

	p.CurrentRecording[id] = sch
	p.Unlock()
	deviceController.Notify(ControllerUpdateSchedule, "")

	if ext {
		return v, nil
	}
	return nil, nil
}

func (p *Scheduler) DeleteCurrentRecording(id string, sid uint64) error {
	p.Lock()
	changed := false
	if v, ext := p.CurrentRecording[id]; ext && v.ID == sid {
		delete(p.CurrentRecording, id)
		changed = true
	}
	p.Unlock()
	if changed {
		deviceController.Notify(ControllerUpdateSchedule, "")
	}

	return nil
}

func (p *Scheduler) RecordingStop(id string) error {
	v, ext := p.GetCurrentRecording(id)

	if ext {
		if err := scheduler.Complete(CurrentSlot, v.ID); err != nil {
			return err
		}
		p.Lock()
		v.Complete = true
		p.Unlock()
		p.UpdateScheduler()
	}

	return nil
}

func (p *Scheduler) RecordingStopByScheduleID(id uint64) error {
	p.Lock()
	deviceID := ""
	for did, sch := range p.CurrentRecording {
		if sch.ID == id {
			deviceID = did
			break
		}
	}
	p.Unlock()

	if deviceID == "" {
		return nil
	}

	return p.RecordingStop(deviceID)
}

func (p *Scheduler) IfWatchingToRecording(sid, eid uint) (bool, error) {
	sch, err := scheduler.GetRecordingScheduleFromEID(CurrentSlot, sid, eid)
	if err != nil {
		return false, err
	}
	if sch != nil && sch.WatchOnly {
		sch.WatchOnly = false
		sch.LastUpdate = Now().Unix()
		scheduler.Modify(sch)

		v, ext := p.GetCurrentRecording(sch.DeviceID)
		if ext {
			if v.ID == sch.ID {
				p.Lock()
				v.WatchOnly = false
				p.Unlock()
			}
		}
		return true, nil
	}
	return false, nil
}

func (p *Scheduler) enqueue(req SchedulerRequest) {
	ctx := serviceContext()
	if ctx.Err() != nil {
		return
	}
	p.queueMu.RLock()
	queue := p.chUpdate
	p.queueMu.RUnlock()
	if queue == nil {
		return
	}
	select {
	case queue <- req:
	case <-ctx.Done():
	}
}

func (p *Scheduler) UpdateScheduler() {
	p.enqueue(SchedulerRequest{Com: SchdulerComUpdate})
}

func (p *Scheduler) DistributeOrder(rs []*RecordingSchedule, now int64) []*RecordingSchedule {
	p.ClearDeviceID(rs)

	upd := p.FixedDeviceAssist(rs)

	for _, v := range rs {

		// 実行中の割り当てはClearDeviceIDで復元した値をそのまま使用する。
		if len(v.DeviceID) > 0 {
			continue
		}
		if !v.Validate || v.Complete || !v.AutoSelect || v.EndTime+v.TailTime < now {
			v.DeviceID = v.tDeviceID
			continue
		}

		order, err := station.GetDeviceIDOrderList(v.Sid)
		if err != nil {
			log.Println("error:", err)
			continue
		}

		detect := true
		for _, did := range order {
			v.DeviceID = did
			// 衝突していなければこれを割り当てる
			detect = p.CollisionDetection(v, rs, now)
			if !detect {
				break
			}
			// 被りが同じサービスであれば補助デバイスを使用する
			if configure.RecordingDevice.Assist {
				aid := p.SelectAssistDevice([]string{did}, v, rs, now)
				if len(aid) > 0 {
					v.DeviceID = aid
					detect = false
					break
				}
			}
		}

		if detect {
			// 全て塞がっていれば被りの少ないデバイスを選択
			err = p.SelectMinimalOverlap(order, v, rs, now)
		}
	}

	// デバイスIDが変わっていればスケジュールを更新
	for _, v := range rs {
		if v.AutoSelect {
			if v.DeviceID != v.tDeviceID {
				upd = append(upd, v)
			}
		}
	}

	return upd
}

func (p *Scheduler) ClearDeviceID(rs []*RecordingSchedule) {
	now := Now().Unix()
	for _, v := range rs {
		v.tDeviceID = v.DeviceID
		// EPG更新で開始時刻が未来へ移っても、取得済みのデバイスを維持する。
		if deviceID, ok := p.fixedDevice(v.ID); ok {
			v.DeviceID = deviceID
			continue
		}
		// 既に開始している場合は固定
		if !v.AutoSelect || (v.Validate && v.StartTime-v.IdleTime <= now && now < v.EndTime+v.TailTime) {
			continue
		}
		v.DeviceID = ""
	}
}

func (p *Scheduler) FixedDeviceAssist(rs []*RecordingSchedule) []*RecordingSchedule {
	upd := []*RecordingSchedule{}

	if !configure.RecordingDevice.Assist {
		return upd
	}

	now := Now().Unix()
	for _, v := range rs {
		if !v.Validate || v.AutoSelect {
			continue
		}
		for _, sch := range rs {
			if _, fixed := p.fixedDevice(sch.ID); fixed {
				continue
			}
			if v.ID == sch.ID || !sch.Validate || sch.AutoSelect || sch.StartTime-sch.IdleTime <= now && now < sch.EndTime+sch.TailTime {
				continue
			}
			if p.CollisionLength(sch, v, now) > 0 {
				if sch.DeviceID == v.DeviceID {
					did := sch.DeviceID
					sub := configure.RecordingDevice.DeviceAssistance[did]
					if sub == "" || sub == did {
						continue
					}
					sch.DeviceID = sub
					sch.tDeviceID = sub
					upd = append(upd, sch)
					break
				}
			}
		}
	}

	return upd
}

func (p *Scheduler) CollisionLength(sa *RecordingSchedule, sb *RecordingSchedule, now int64) int64 {
	saStart := sa.StartTime
	sbStart := sb.StartTime
	saEnd := sa.EndTime + sa.TailTime
	sbEnd := sb.EndTime + sb.TailTime
	if configure.IsAfterWithMargin {
		saStart = saStart - sa.IdleTime
		sbStart = sbStart - sb.IdleTime
	}
	assistName := configure.RecordingDevice.DeviceAssistance[sa.DeviceID]
	if len(assistName) < 1 {
		assistName = sa.DeviceID
	}
	paired := assistName == sb.DeviceID
	if configure.RecordingDevice.Assist {
		// 補助側を候補にした場合も通常側との衝突を検出する。
		paired = paired || configure.RecordingDevice.DeviceAssistance[sb.DeviceID] == sa.DeviceID
	}

	if sa.ID == sb.ID || !sa.Validate || !sb.Validate ||
		sa.Complete || sb.Complete ||
		now > saEnd || now > sbEnd ||
		len(sa.DeviceID) < 1 || len(sb.DeviceID) < 1 || (sa.DeviceID != sb.DeviceID && !paired) {
		return 0
	}

	if (sbStart <= saStart && saStart < sbEnd) ||
		(sbStart < saEnd && saEnd < sbEnd) ||
		(saStart <= sbStart && sbStart < saEnd) ||
		(saStart < sbEnd && sbEnd < saEnd) {
		return min(saEnd, sbEnd) - max(saStart, sbStart)
	}

	return 0
}

func (p *Scheduler) CollisionDetection(sch *RecordingSchedule, rs []*RecordingSchedule, now int64) bool {
	for _, v := range rs {
		if p.CollisionLength(sch, v, now) > 0 {
			return true
		}
	}
	return false
}

func (p *Scheduler) SelectMinimalOverlap(order []string, sch *RecordingSchedule, rs []*RecordingSchedule, now int64) error {
	dev := ""
	min := int64(10000)
	if configure.RecordingDevice.Assist {
		min = int64(1<<63 - 1)
	}
	for _, did := range order {
		sch.DeviceID = did
		mlen := int64(0)
		for _, v := range rs {
			len := p.CollisionLength(sch, v, now)
			if len > mlen {
				mlen = len
			}
		}
		if mlen < min {
			dev = did
			if configure.RecordingDevice.Assist {
				min = mlen
			}
		}
	}
	sch.DeviceID = dev
	return nil
}

func (p *Scheduler) SelectAssistDevice(order []string, sch *RecordingSchedule, rs []*RecordingSchedule, now int64) string {
	if !configure.RecordingDevice.Assist {
		return ""
	}

	for _, did := range order {
		sub := configure.RecordingDevice.DeviceAssistance[did]
		if sub == "" || sub == did {
			continue
		}
		if _, exists := slot.Get(did); !exists {
			continue
		}
		if _, exists := slot.Get(sub); !exists {
			continue
		}
		// 通常側が空いていれば通常側、通常側だけが使用中なら補助側を試す。
		for _, target := range []string{did, sub} {
			candidate := *sch
			candidate.DeviceID = target
			shared, blocked := false, false
			for _, v := range rs {
				if v.DeviceID != did && v.DeviceID != sub {
					continue
				}
				// 時間の重複は同一論理デバイスとして判定し、ペアの向きに依存させない。
				other := *v
				other.DeviceID = target
				if p.CollisionLength(&candidate, &other, now) <= 0 {
					continue
				}
				if v.DeviceID == target || v.Sid != sch.Sid {
					blocked = true
					break
				}
				shared = true
			}
			if shared && !blocked {
				return target
			}
		}
	}
	return ""
}

func ScheduleContains(rs []*RecordingSchedule, id uint64) bool {
	for _, v := range rs {
		if v.ID == id {
			return true
		}
	}
	return false
}

func (p *Scheduler) IsCombo(rs []*RecordingSchedule, s *RecordingSchedule, now int64) *RecordingSchedule {
	for _, v := range rs {
		if !v.Validate || s.ID == v.ID || s.Sid != v.Sid || now > (v.EndTime+v.TailTime) {
			continue
		}
		if (v.EndTime-1) <= s.StartTime && s.StartTime <= (v.EndTime-1) {
			return v
		}
	}
	return nil
}

func (p *Scheduler) ScheduleUpdateAndOrder(rs []*RecordingSchedule) ([]*RecordingSchedule, error) {
	p.deviceMu.Lock()
	defer p.deviceMu.Unlock()

	upd := p.DistributeOrder(rs, Now().Unix())

	if err := p.updateSchedulesLocked(CurrentSlot, upd...); err != nil {
		log.Println("error: スケジュールの更新に失敗しました", err, upd)
		return nil, err
	}

	return rs, nil
}

func (p *RecordingSchedule) TvProgramFollow(tvp *TvProgram) bool {
	if tvp.Start != p.StartTime || tvp.End != p.EndTime || tvp.Title != p.Title {
		log.Printf("info: 番組情報の更新を確認 %d:%d [%s,%s,%s] → [%s %s,%s]",
			tvp.Sid, tvp.Eid, tvp.Title, time.Unix(p.StartTime, 0), time.Unix(p.EndTime, 0),
			tvp.Title, time.Unix(tvp.Start, 0), time.Unix(tvp.End, 0))

		p.StartTime = tvp.Start
		p.EndTime = tvp.End
		p.Title = tvp.Title
		return true
	}
	return false
}

func (p *Scheduler) EpgFollow(rs []*RecordingSchedule) ([]*RecordingSchedule, error) {
	day := Now().Unix() + 24*60*60
	upd := []*RecordingSchedule{}
	for _, v := range rs {
		if v.StartTime-v.IdleTime > day {
			break
		}

		if !v.EpgFollow {
			continue
		}

		prg, err := tvprogram.Get(CurrentSlot, v.Sid, v.Eid)
		if err != nil || len(prg) < 1 {
			log.Printf("error: 番組情報が見つかりません %d:%d", v.Sid, v.Eid)
			continue
		}

		e := programForInterval(prg, v.StartTime, v.EndTime)
		if e == nil {
			continue
		}
		if v.TvProgramFollow(e) {
			upd = append(upd, v)
		}
	}

	if err := p.UpdateSchedules(CurrentSlot, upd...); err != nil {
		log.Println("error: 番組情報の更新に失敗しました", err, upd)
		return rs, err
	}

	return rs, nil
}

func (p *Scheduler) ScheduleUpdate() ([]*RecordingSchedule, error) {
	rs, err := p.GetScheduleAll()
	if err != nil {
		return rs, err
	}

	rs, err = p.EpgFollow(rs)
	if err != nil {
		return rs, err
	}

	return p.ScheduleUpdateAndOrder(rs)
}

func (p *Scheduler) GetRrecordingStatus() (*RecordingStatus, error) {
	p.Lock()
	defer p.Unlock()

	status := &RecordingStatus{
		Recording:           []uint64{},
		Current:             make(map[string]RecordingSchedule, len(p.CurrentRecording)),
		CurrentMilliseconds: Now().UnixMilli(),
	}
	for deviceID, rs := range p.CurrentRecording {
		if rs == nil {
			continue
		}
		status.Recording = append(status.Recording, rs.ID)
		// ファイル名など不要な可変フィールドを読み取らず、実際のタスク所有先を返す。
		status.Current[deviceID] = RecordingSchedule{
			ID: rs.ID, DeviceID: deviceID, Title: rs.Title, Validate: rs.Validate,
			Complete: rs.Complete, WatchOnly: rs.WatchOnly,
			StartTime: rs.StartTime, EndTime: rs.EndTime,
			IdleTime: rs.IdleTime, ReadyTime: rs.ReadyTime, TailTime: rs.TailTime, Sid: rs.Sid, Eid: rs.Eid,
		}
	}

	return status, nil
}

// startupDeviceIDsには、起動時刻が再接続猶予期間内にある予約が含まれます。
// 未割り当てのデバイスIDについては、その準備が整っているかどうかがまだ判断できないため、
// 期限まで待機するよう意図的に処理されます。
func startupDeviceIDs(schedules []*RecordingSchedule, now int64, deadline int64) []string {
	ids := []string{}
	seen := make(map[string]bool)
	for _, sch := range schedules {
		if !sch.Validate || sch.Complete || sch.EndTime+sch.TailTime <= now || sch.StartTime-sch.IdleTime > deadline {
			continue
		}
		if !seen[sch.DeviceID] {
			seen[sch.DeviceID] = true
			ids = append(ids, sch.DeviceID)
		}
	}
	return ids
}

func (p *Scheduler) waitForStartupDevices(ctx context.Context, timeout time.Duration) {
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	now := Now()
	schedules, err := p.GetScheduleAll()
	if err != nil {
		log.Println("error: 再接続待機対象の予約を取得できません", err)
		// 読み取りの失敗によって、再接続の猶予期間が解除されてはならない。
		<-waitCtx.Done()
		return
	}
	ids := startupDeviceIDs(schedules, now.Unix(), now.Add(timeout).Unix())
	if len(ids) == 0 {
		return
	}
	log.Printf("info: 予約再開前にデバイスの再接続を待機: %v (最大 %s)", ids, timeout)
	if err := slot.waitForDevices(waitCtx, ids); err == context.DeadlineExceeded && ctx.Err() == nil {
		log.Printf("warn: デバイス再接続待機がタイムアウトしました。スケジューラーを開始します: %v", ids)
	}
}

func (p *Scheduler) ExecScheduler() {
	ctx, cancel := context.WithCancel(globalContext)
	defer cancel()

	//	done := make(chan struct{})
	p.queueMu.Lock()
	queue := make(chan SchedulerRequest, 256)
	p.chUpdate = queue
	p.queueMu.Unlock()
	defer func() {
		log.Printf("info: スケジューラーを終了")
		p.queueMu.Lock()
		p.chUpdate = nil
		p.queueMu.Unlock()
	}()

	// 初期読み込み
	p.UpdateScheduler()

	// ランチャー接続ではなく予約で使用するデバイスの状態通知を待つ。
	p.waitForStartupDevices(ctx, 3*time.Second)
	if ctx.Err() != nil {
		return
	}

	log.Printf("info: スケジューラーの起動")
	for {
		select {
		//		case <-done:
		//			return
		case que := <-queue:

			switch que.Com {

			case SchdulerComUpdate:
				rs, err := scheduler.ScheduleUpdate()
				if err != nil {
					log.Println("error:", err)
					break
				}

				if err = tasks.Update(rs); err != nil {
					log.Println("error:", err)
					break
				}

				deviceController.Notify(ControllerUpdateSchedule, "")

				log.Printf("debug: update schedule and task total %d", len(rs))

			case SchdulerComAdd:
				if _, err := scheduler.Insert(CurrentSlot, que.Schedule...); err != nil {
					log.Println("error:", err)
					break
				}

				rs, err := scheduler.ScheduleUpdate()
				if err != nil {
					log.Println("error:", err)
					break
				}

				if err = tasks.Update(rs); err != nil {
					log.Println("error:", err)
					break
				}

				deviceController.Notify(ControllerUpdateSchedule, "")

				log.Printf("debug: add schedule and task total %d", len(rs))

			case SchdulerComModify:
				for _, v := range que.Schedule {
					if err := scheduler.Update(CurrentSlot, v); err != nil {
						log.Println("error:", err)
						break
					}
				}

				rs, err := scheduler.ScheduleUpdate()
				if err != nil {
					log.Println("error:", err)
					break
				}

				if err = tasks.Update(rs); err != nil {
					log.Println("error:", err)
					break
				}

				deviceController.Notify(ControllerUpdateSchedule, "")

				log.Printf("debug: modify schedule and task total %d", len(rs))

			case SchdulerComStop:
				if err := scheduler.Complete(CurrentSlot, que.ID); err != nil {
					log.Println("error:", err)
					break
				}

				rs, err := scheduler.ScheduleUpdate()
				if err != nil {
					log.Println("error:", err)
					break
				}

				if err = tasks.Update(rs); err != nil {
					log.Println("error:", err)
					break
				}

				deviceController.Notify(ControllerUpdateSchedule, "")

				log.Printf("debug: stop schedule and task id:%d, total %d", que.ID, len(rs))

			case SchdulerComRemove:
				if err := scheduler.Delete(CurrentSlot, que.ID); err != nil {
					log.Println("error:", err)
					break
				}

				rs, err := scheduler.ScheduleUpdate()
				if err != nil {
					log.Println("error:", err)
					break
				}

				if err = tasks.Update(rs); err != nil {
					log.Println("error:", err)
					break
				}

				deviceController.Notify(ControllerUpdateSchedule, "")

				log.Printf("debug: remove schedule and task id:%d, total %d", que.ID, len(rs))
			}

		case <-ctx.Done():
			log.Println("info: 終了通知を検出")
			return
		}
	}
}

func (p *Scheduler) Add(rs ...*RecordingSchedule) {
	p.enqueue(SchedulerRequest{Com: SchdulerComAdd, Schedule: rs})
}

func (p *Scheduler) Modify(rs ...*RecordingSchedule) {
	p.enqueue(SchedulerRequest{Com: SchdulerComModify, Schedule: rs})
}

func (p *Scheduler) Stop(id uint64) {
	p.enqueue(SchedulerRequest{Com: SchdulerComStop, ID: id})
}

func (p *Scheduler) Remove(id uint64) {
	p.enqueue(SchedulerRequest{Com: SchdulerComRemove, ID: id})
}
