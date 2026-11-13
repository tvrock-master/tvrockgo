package tvrock

import (
	"log"
	"time"

	"github.com/gorilla/websocket"
)

// 終了要求は接続の切断ではなくランチャーのプロセス終了通知まで保持する。
// 各接続には一度だけ送信し、再接続時に再試行する。期限後は警告して打ち切る。
type deviceShutdownRequest struct {
	deviceID string
	jobID    uint64
	deadline time.Time
	timer    *time.Timer
	queued   chan RequestMessage // shutdownTimerのロックで保護
}

func (r *deviceShutdownRequest) cancelLocked() {
	if shutdownTimer.shutdownDevice[r.deviceID] == r.timer {
		r.timer.Stop()
		delete(shutdownTimer.shutdownDevice, r.deviceID)
	}
}

func (r *deviceShutdownRequest) run() {
	if !serviceWorkers.enter() {
		return
	}
	defer serviceWorkers.wg.Done()
	// 終了元が後から終了監視を登録する場合もあるため、送信可能な状態に
	// なる前にも引き継ぎを確認する。送信直前の確認はdispatchで別途行う。
	remoteWatching.Lock()
	defer remoteWatching.Unlock()
	currentJob, hasCurrentJob := scheduler.GetCurrentRecording(r.deviceID)
	handedOver := hasCurrentJob && currentJob.ID != r.jobID
	for _, session := range remoteWatching.sessions {
		if session.DeviceID == r.deviceID && !session.Stopping {
			handedOver = true
		}
	}
	shutdownTimer.Lock()
	defer shutdownTimer.Unlock()
	if shutdownTimer.shutdownDevice[r.deviceID] != r.timer {
		return
	}
	if serviceContext().Err() != nil {
		r.cancelLocked()
		return
	}
	dev, ok := slot.Get(r.deviceID)
	if !ok {
		r.cancelLocked()
		return
	}
	slot.RLock()
	exited, state, queue, done := dev.processExited, dev.State, dev.Com, dev.requestDone
	slot.RUnlock()
	if exited {
		r.cancelLocked()
		return
	}
	if handedOver {
		r.cancelLocked()
		log.Printf("info: 後続予約または視聴のためデバイス終了を中止 id:%d device:%s", r.jobID, r.deviceID)
		return
	}
	if !time.Now().Before(r.deadline) {
		log.Printf("warn: デバイスの終了を確認できず再試行を打ち切り id:%d device:%s", r.jobID, r.deviceID)
		r.cancelLocked()
		return
	}
	if state == StateReady && queue != nil && queue != r.queued {
		select {
		case <-done:
		default:
			select {
			case queue <- RequestMessage{ID: r.deviceID, Command: "PowerOff", shutdown: r}:
				r.queued = queue
			default:
				// キューが満杯でも後続タスクの制御権取得を妨げない。
			}
		}
	}
	r.timer.Reset(time.Second)
}

// 判定から実際のWebSocket送信までを制御権取得と直列化する。
// キューに残った古い要求も、後続予約・視聴が使用を開始した後は送らない。
func (r *deviceShutdownRequest) write(c *websocket.Conn, dev *RockDevice, queue chan RequestMessage) error {
	return r.dispatch(dev, queue, func(message RequestMessage) error {
		if err := c.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
			return err
		}
		err := c.WriteJSON(message)
		_ = c.SetWriteDeadline(time.Time{})
		return err
	})
}

func (r *deviceShutdownRequest) dispatch(dev *RockDevice, queue chan RequestMessage, send func(RequestMessage) error) error {
	scheduler.deviceMu.Lock()
	defer scheduler.deviceMu.Unlock()
	// 開始直前の予約も保護する。読み取り失敗時は終了を見送る。
	protected := false
	if scheduler.db != nil {
		schedules, err := scheduler.GetScheduleAll()
		protected = err != nil
		for _, sch := range schedules {
			if sch.ID != r.jobID && scheduleKeepsWatchDevice(sch, r.deviceID, Now(), 180) {
				protected = true
			}
		}
	}
	remoteWatching.Lock()
	defer remoteWatching.Unlock()
	for _, session := range remoteWatching.sessions {
		if session.DeviceID == r.deviceID && !session.Stopping {
			protected = true
		}
	}
	currentJob, hasCurrentJob := scheduler.GetCurrentRecording(r.deviceID)
	finishing := hasCurrentJob && currentJob.ID == r.jobID
	if hasCurrentJob && !finishing {
		protected = true
	}
	shutdownTimer.Lock()
	defer shutdownTimer.Unlock()
	if shutdownTimer.shutdownDevice[r.deviceID] != r.timer {
		return nil
	}
	slot.RLock()
	current := dev.Com == queue && dev.State == StateReady && !dev.processExited
	recording := dev.State == StateRecording
	slot.RUnlock()
	if protected || recording {
		r.cancelLocked()
		log.Printf("info: 後続予約または視聴のためデバイス終了を中止 id:%d device:%s", r.jobID, r.deviceID)
		return nil
	}
	if finishing {
		// 待ち時間が0秒でも、終了元タスクが制御権を解放するまで待つ。
		r.queued = nil
		return nil
	}
	if !current {
		// 起動途中などで送れなかった場合、同じ接続でも再試行できるようにする。
		r.queued = nil
		return nil
	}
	err := send(RequestMessage{ID: r.deviceID, Command: "PowerOff"})
	if err == nil {
		log.Printf("info: デバイス終了指示を送信 id:%d device:%s", r.jobID, r.deviceID)
	}
	return err
}
