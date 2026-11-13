package tvrock

import (
	"fmt"
	"log"
)

// 確保したデバイスは所有タスクが完了するまで保持し続けること。
// これには遅延を伴う停止／シャットダウン処理や状態をJobNoneに戻すEPG変更なども含まれる。
type deviceAssignment struct {
	job      *RecordingJob
	deviceID string
}

func (p *Scheduler) fixedDevice(id uint64) (string, bool) {
	p.RLock()
	defer p.RUnlock()
	if assignment, ok := p.deviceAssignments[id]; ok {
		return assignment.deviceID, true
	}
	for deviceID, sch := range p.CurrentRecording {
		if sch != nil && sch.ID == id {
			return deviceID, true
		}
	}
	return "", false
}

// deviceMuのコンテキスト下、DBトランザクションの開始前に呼び出される。
// スケジュール更新のすべての経路（キーワード予約、手動予約、EPG予約、再配信）において取得済みのデバイスは保持される。
func (p *Scheduler) preserveDeviceAssignment(no int, sch *RecordingSchedule) error {
	if no != CurrentSlot {
		return nil
	}
	deviceID, fixed := p.fixedDevice(sch.ID)
	if !fixed {
		previous, err := p.GetRecordingSchedule(no, sch.ID)
		if err != nil {
			return err
		}
		// また、タスクが制御権を獲得する前（例えば、サーバー再起動後の再接続時など）の
		// 既存のスタートアップウィンドウに関するルールも保持しておく。
		now := Now().Unix()
		if previous != nil && previous.Validate && !previous.Complete &&
			previous.StartTime-previous.IdleTime <= now && now < previous.EndTime+previous.TailTime {
			deviceID, fixed = previous.DeviceID, previous.DeviceID != ""
		}
	}
	if fixed && sch.DeviceID != deviceID {
		log.Printf("info: 実行開始済み予約のデバイスを維持 id:%d, device:%s, requested:%s", sch.ID, deviceID, sch.DeviceID)
		sch.DeviceID = deviceID
	}
	return nil
}

// 呼び出し元はジョブのロックを保持している。
// deviceMuはこの最後のDB読み取りとデバイス制御の取得との間にスケジュール書き込みが割り込んでくるのを防ぐ。
func (p *Scheduler) acquireDevice(job *RecordingJob) (*RecordingSchedule, error) {
	p.deviceMu.Lock()
	defer p.deviceMu.Unlock()
	sch, err := p.GetRecordingSchedule(CurrentSlot, job.ID)
	if err != nil {
		return nil, err
	}
	if sch == nil || !sch.Validate || sch.Complete {
		return nil, fmt.Errorf("開始可能な予約がありません id:%d", job.ID)
	}
	if !job.deviceLocked && job.getStatus() == JobNone {
		if sch.DeviceID == "" {
			return nil, fmt.Errorf("予約のデバイスが未割り当てです id:%d", job.ID)
		}
		if job.DeviceID != sch.DeviceID {
			log.Printf("info: 起動前にタスクのデバイスを同期 id:%d, before:%s, device:%s", job.ID, job.DeviceID, sch.DeviceID)
			job.DeviceID = sch.DeviceID
		}
	}
	if job.DeviceID == "" {
		return nil, fmt.Errorf("タスクのデバイスが未割り当てです id:%d", job.ID)
	}
	sch.DeviceID = job.DeviceID
	p.Lock()
	defer p.Unlock()
	if p.deviceAssignments == nil {
		p.deviceAssignments = map[uint64]deviceAssignment{}
	}
	if assignment, ok := p.deviceAssignments[job.ID]; ok && assignment.job != job {
		return nil, fmt.Errorf("別のタスクが予約のデバイスを使用中です id:%d", job.ID)
	}
	previous := p.CurrentRecording[job.DeviceID]
	if p.CurrentRecording == nil {
		p.CurrentRecording = map[string]*RecordingSchedule{}
	}
	p.CurrentRecording[job.DeviceID] = sch
	// 前のタスクの遅延終了要求を、次のタスクへ引き継がない。
	job.ShutdownCancel()
	p.deviceAssignments[job.ID] = deviceAssignment{job: job, deviceID: job.DeviceID}
	job.deviceLocked = true
	if previous != nil && previous.ID == job.ID {
		return nil, nil
	}
	return previous, nil
}

func (p *Scheduler) releaseDevice(job *RecordingJob) {
	p.deviceMu.Lock()
	defer p.deviceMu.Unlock()
	p.Lock()
	defer p.Unlock()
	if assignment, ok := p.deviceAssignments[job.ID]; ok && assignment.job == job {
		delete(p.deviceAssignments, job.ID)
	}
}
