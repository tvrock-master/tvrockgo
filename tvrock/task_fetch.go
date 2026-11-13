package tvrock

import (
	"log"
	"sort"
	"time"
)

type fetchProgramStationResult string

const (
	fetchProgramWaiting      fetchProgramStationResult = ""
	fetchProgramStationDone  fetchProgramStationResult = "番組情報数の変化なし"
	fetchProgramNoEvents     fetchProgramStationResult = "規定時間内に番組情報を受信できませんでした（0件）。放送休止・受信状態を確認してください"
	fetchProgramTuningFailed fetchProgramStationResult = "規定時間内に対象局の選局を確認できませんでした"
)

// 選局を確認してから番組情報数の変化が止まった時間を測る。
type fetchProgramStationWait struct {
	quietTime    time.Duration
	lastEvent    time.Time
	untunedSince time.Time
	lastCount    uint
	tuned        bool
	requested    bool
}

func newFetchProgramStationWait(timeoutSec int) fetchProgramStationWait {
	return fetchProgramStationWait{
		quietTime: time.Duration(max(1, timeoutSec)) * time.Second,
	}
}

func (w *fetchProgramStationWait) observe(now time.Time, ready bool, events uint) fetchProgramStationResult {
	if !ready {
		// 前の局の情報や、切断中の情報で取得終了を判定しない。
		w.tuned = false
		if w.untunedSince.IsZero() {
			w.untunedSince = now
		}
		if now.Sub(w.untunedSince) >= w.quietTime {
			return fetchProgramTuningFailed
		}
		return fetchProgramWaiting
	}

	w.untunedSince = time.Time{}
	if !w.tuned || events != w.lastCount {
		w.tuned = true
		w.lastCount = events
		w.lastEvent = now
		return fetchProgramWaiting
	}

	if now.Sub(w.lastEvent) >= w.quietTime {
		if events == 0 {
			return fetchProgramNoEvents
		}
		return fetchProgramStationDone
	}

	return fetchProgramWaiting
}

func fetchProgramTargets(stations map[uint]*TvStation, tsp TuningSpace) []*TvStation {
	var targets []*TvStation
	for _, st := range stations {
		if st.Tsp == uint(tsp) && st.FetchProgram {
			targets = append(targets, st)
		}
	}

	sort.Slice(targets, func(i, j int) bool {
		if targets[i].No != targets[j].No {
			return targets[i].No < targets[j].No
		}
		return targets[i].Sid < targets[j].Sid
	})
	return targets
}

func (p *RecordingJob) pollFetchProgramStation(now time.Time, wait *fetchProgramStationWait) fetchProgramStationResult {
	ready := slot.IsCurrentService(p.DeviceID, p.Sid)
	if !ready {
		if !wait.requested || wait.tuned {
			p.logFetchProgramTuning("選局要求")
			wait.requested = true
		}
		if err := p.StreamChange(); err != nil {
			log.Println("error: ", err)
		}
	} else if !wait.tuned {
		p.logFetchProgramTuning("選局確認")
	}

	events := uint(0)
	if dev, ext := slot.Get(p.DeviceID); ext && dev != nil {
		events = dev.DeviceInfo.Events
	}

	return wait.observe(now, ready, events)
}

func (p *RecordingJob) logFetchProgramTuning(action string) {
	name := "不明"
	if st, ok := station.Get(p.Sid); ok {
		name = st.Name
	}
	log.Printf("info: 番組情報取得%s id:%d device:%s sid:%d 局名:%s", action, p.ID, p.DeviceID, p.Sid, name)
}

func (s *Slot) isFetchProgramTuningReady(id string) bool {
	s.RLock()
	defer s.RUnlock()
	dev, ok := s.Device[id]
	return ok && dev != nil && dev.State >= StateReady && dev.CurrentService&0xffff != 0
}

func (p *RecordingJob) ExecFetchProgram(ta *TaskArray) {

	ctx := serviceContext()
	if ctx.Err() != nil {
		return
	}

	// 1. BootTime まで待って起動します
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// 起動完了を待機
L_BOOT:
	for {
		now := Now()
		if !now.Before(p.BootTime) {
			break L_BOOT
		}
		select {
		case <-ctx.Done():
			return
		case no := <-p.Com:
			if no == JobExit || no == JobForceExit {
				p.setStatus(no)
				return
			}
		case <-ticker.C:
		}
	}

	if ctx.Err() != nil {
		return
	}

	// パラメータを決定する
	var tsp TuningSpace
	var timeoutSec int

	switch p.BootType {
	case DeviceBootTypeFetchProgramTS:
		tsp = TuningSpaceTS
		timeoutSec = configure.FetchProgram.TimeoutTS
	case DeviceBootTypeFetchProgramBS:
		tsp = TuningSpaceBS
		timeoutSec = configure.FetchProgram.TimeoutBS
	case DeviceBootTypeFetchProgramCS:
		tsp = TuningSpaceCS
		timeoutSec = configure.FetchProgram.TimeoutCS
	case DeviceBootTypeFetchProgramOS:
		tsp = TuningSpaceOS
		timeoutSec = configure.FetchProgram.TimeoutOS
	}

	stationsMap, err := station.GetStations(0)
	if err != nil {
		log.Println("error: ", err)
		return
	}

	targets := fetchProgramTargets(stationsMap, tsp)
	if len(targets) == 0 {
		log.Printf("info: 番組情報取得の対象局なし id:%d", p.ID)
		p.setStatus(JobExit)
		return
	}

	// 予約作成時の局はmapの走査順に左右されるため、実際の取得順の
	// 先頭局を起動引数にも使用する。起動後も選局通知を必ず確認する。
	p.Lock()
	p.Sid = targets[0].Sid
	err = p.AcquireDeviceControl(ta)
	p.Unlock()
	if err != nil {
		log.Println("error: ", err)
		return
	}
	if err := p.Boot(); err != nil {
		log.Println("error: ", err)
		return
	}
	p.setStatus(JobBoot)

	// DLLの接続完了はチューナーの初期化完了とは限らない。
	// /sidによる初回選局通知より前の切り替え要求は、アプリ側で
	// 未準備のチャンネル一覧をキャッシュさせる場合がある。
	tuningReadyLimit := Now().Add(time.Duration(max(1, configure.DeviceBootupTimeout)) * time.Second)
	// Wait for RecordStartTime and initial tuning notification.
L_WAIT_START:
	for {
		now := Now()
		if !now.Before(p.RecordStopTime) {
			log.Printf("warn: 番組情報取得の起動待ち中に終了時刻へ到達 id:%d device:%s", p.ID, p.DeviceID)
			p.RequestShutdown()
			p.setStatus(JobExit)
			return
		}

		if slot.isFetchProgramTuningReady(p.DeviceID) && !now.Before(p.RecordStartTime) {
			log.Printf("info: 番組情報取得の初回選局を確認 id:%d device:%s", p.ID, p.DeviceID)
			break L_WAIT_START
		}

		if slot.IsReady(p.DeviceID) && !now.Before(p.RecordStartTime) && !now.Before(tuningReadyLimit) {
			// 放送休止等で初回通知が来ない場合も、起動待ち上限後には
			// 各局の選局を試し、失敗した局を警告付きで次へ進める。
			log.Printf("warn: 番組情報取得の初回選局通知を待機時間内に確認できませんでした id:%d device:%s", p.ID, p.DeviceID)
			break L_WAIT_START
		}

		select {
		case <-ctx.Done():
			return
		case no := <-p.Com:
			if no == JobExit || no == JobForceExit {
				p.setStatus(no)
				return
			}
		case <-ticker.C:
		}
	}

	p.setStatus(JobRecordStart)

	log.Printf("info: 番組情報取得を開始 id:%d device:%s 対象局数:%d 終了時刻:%s", p.ID, p.DeviceID, len(targets), p.RecordStopTime)
	visited := 0
	failed := 0
	for i, st := range targets {
		now := Now()
		if !now.Before(p.RecordStopTime) || p.Complete {
			break
		}

		wait := newFetchProgramStationWait(timeoutSec)
		p.Lock()
		p.Sid = st.Sid
		p.Unlock()
		log.Printf("info: 番組情報取得 id:%d sid:%d 局名:%s 順番:%d/%d", p.ID, st.Sid, st.Name, i+1, len(targets))

	L_STATION:
		for {
			if !Now().Before(p.RecordStopTime) || p.Complete {
				break L_STATION
			}

			select {
			case <-ctx.Done():
				return
			case no := <-p.Com:
				if no == JobExit || no == JobForceExit {
					p.setStatus(no)
					return
				}
			case <-ticker.C:
				if !p.HasDeviceControl("fetch program") {
					p.setStatus(JobForceExit)
					return
				}
				if reason := p.pollFetchProgramStation(Now(), &wait); reason != "" {
					if reason == fetchProgramStationDone {
						log.Printf("info: 番組情報取得を終了 id:%d sid:%d 局名:%s 理由:%s", p.ID, st.Sid, st.Name, reason)
					} else {
						failed++
						log.Printf("warn: 番組情報取得の失敗 id:%d device:%s sid:%d 局名:%s 理由:%s", p.ID, p.DeviceID, st.Sid, st.Name, reason)
					}
					visited++
					break L_STATION
				}
			}
		}
	}

	if visited < len(targets) {
		log.Printf("warn: 番組情報取得の打ち切り id:%d 処理済み局数:%d/%d 未処理局数:%d", p.ID, visited, len(targets), len(targets)-visited)
	}
	log.Printf("info: 番組情報取得の終了 id:%d 処理済み局数:%d/%d 取得失敗局数:%d", p.ID, visited, len(targets), failed)

	if ctx.Err() != nil {
		return
	}

	// キーワード予約スケジュールの更新
	if err := scheduler.AddAllKeywordReserveSchedules(); err != nil {
		log.Println("error: ", err)
	}

	p.RequestShutdown()
	p.setStatus(JobExit)
	p.Complete = true
}
