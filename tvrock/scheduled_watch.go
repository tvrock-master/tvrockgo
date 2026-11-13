package tvrock

// スケジュールは意図を告知するのみであり、実行ファイルや引数は各PC上に保持されます。
// 告知が繰り返されることで予約期間中に接続してきたRockbarも参加が可能になります
type scheduledWatch struct {
	Action    string `json:"action"`
	Request   string `json:"schedule"`
	Device    string `json:"device"`
	StopOnEnd bool   `json:"-"`
}

type scheduledWatchJob struct {
	id               uint64
	parent           string
	stopOnEnd, ready bool
	preserve         bool
}

func watchBootType(boot uint) bool {
	switch boot {
	case DeviceBootTypeWatch, DeviceBootTypeWatchOnly, DeviceBootTypeOnesegWatch, DeviceBootTypeOnesegWatchOnly:
		return true
	}
	return false
}

func scheduledWatchJobs() []scheduledWatchJob {
	// タスクが密集したコレクションはストリームリースやタスクライターを停滞させてはならない
	if !tasks.TryRLock() {
		return nil
	}
	jobs := make([]*RecordingJob, 0, len(tasks.Job))
	for _, job := range tasks.Job {
		jobs = append(jobs, job)
	}
	tasks.RUnlock()
	result := make([]scheduledWatchJob, 0, len(jobs))
	// リモート視聴ロックを取得する前にジョブのスナップショットを保存する
	// タスクのシャットダウン処理においても、ジョブのロックを保持した状態でアクティブな視聴セッションの確認が行われるため
	for _, job := range jobs {
		if !job.TryRLock() {
			// Boot/tune処理はアプリケーションの応答を待機している間ジョブのロックを保持し続けることがある
			// その待機中に当該処理を追い越して待機したり既存のビューを期限切れにしたりしてはならない
			result = append(result, scheduledWatchJob{id: job.ID, preserve: true})
			continue
		}
		status := job.getStatus()
		enabled := job.Validate && !job.Complete && watchBootType(job.BootType) && status >= JobBoot && status <= JobRecordStart
		id, device, sid, exit := job.ID, job.DeviceID, job.Sid, job.ExitMode
		job.RUnlock()
		if !enabled {
			continue
		}
		current, ok := scheduler.GetCurrentRecording(device)
		if !ok || current.ID != id {
			continue
		}
		d, ok := slot.Get(device)
		if !ok {
			continue
		}
		slot.RLock()
		parent, ready := d.RegistryCode, d.State >= StateReady && d.CurrentService == sid && status >= JobStreamChange
		slot.RUnlock()
		result = append(result, scheduledWatchJob{id: id, parent: parent, stopOnEnd: exit == ExitModeShutDown, ready: ready})
	}
	return result
}

func (m *remoteWatchManager) validSchedule(request, parent string) bool {
	for _, s := range m.scheduled {
		if s.Request == request && s.Device == parent {
			return true
		}
	}
	return false
}

func broadcastScheduled(s *scheduledWatch) {
	deviceController.RLock()
	uids := make([]string, 0, len(deviceController.Handler))
	for uid := range deviceController.Handler {
		uids = append(uids, uid)
	}
	deviceController.RUnlock()
	for _, uid := range uids {
		sendWatchNotice(uid, s)
	}
}

// 呼び出し元がウォッチロックを保持している。サーバー側のスケジュールIDはクライアントから入力されることはない
func (m *remoteWatchManager) updateScheduled(jobs []scheduledWatchJob) {
	if m.scheduled == nil {
		m.scheduled = map[uint64]*scheduledWatch{}
	}

	active := map[uint64]bool{}
	for _, job := range jobs {
		active[job.id] = true
		if job.preserve {
			continue
		}
		s := m.scheduled[job.id]
		if s == nil || s.Device != job.parent {
			s = &scheduledWatch{Action: "schedule", Request: watchToken(), Device: job.parent}
			if s.Request == "" {
				continue
			}
			m.scheduled[job.id] = s
		}
		s.StopOnEnd = job.stopOnEnd
		if job.ready {
			broadcastScheduled(s)
		}
	}

	for id, s := range m.scheduled {
		if active[id] {
			continue
		}
		for _, view := range m.sessions {
			if view.Schedule != s.Request {
				continue
			}

			// 連続する予約では既存の視聴セッションが維持され、その存続期間が次の予約へと引き継がれます。
			// これは現在の視聴（チューニング）が完了する前であっても同様です
			next := ""
			for nextID, candidate := range m.scheduled {
				if active[nextID] && candidate != s && candidate.Device == s.Device {
					next = candidate.Request
					break
				}
			}
			if next != "" {
				view.Schedule = next
			} else if s.StopOnEnd {
				view.Stopping = true
			}
		}
		delete(m.scheduled, id)
	}
}
