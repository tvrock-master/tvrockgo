package tvrock

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type playbackSelection struct {
	schedule *RecordingSchedule
	slot     int
	file     *os.File
}

func (m *playbackManager) releasePorts(session, uid string) {
	m.Lock()
	defer m.Unlock()
	for key, reservation := range m.ports {
		if reservation == session+"/"+uid || uid == "" && strings.HasPrefix(reservation, session+"/") {
			delete(m.ports, key)
		}
	}
}

func (m *playbackManager) devicesFor(uid string) []*RockDevice {
	m.Lock()
	allowed := map[string]bool{}
	for token, s := range m.sessions {
		allowed[token] = s.owner == "" || s.owner == uid
	}
	m.Unlock()
	result := []*RockDevice{}
	for _, device := range m.devices() {
		if allowed[device.Playback.Session] {
			result = append(result, device)
		}
	}
	return result
}

func (m *playbackManager) offer(s *playbackSession) {
	s.Lock()
	uids := make([]string, 0, len(s.invited))
	for uid := range s.invited {
		uids = append(uids, uid)
	}
	s.Unlock()
	for _, uid := range uids {
		playbackNotify(uid, map[string]any{"action": "offer", "session": s.state.Session, "device": s.device.RegistryCode, "deviceId": s.device.ID})
	}
}

func (m *playbackManager) selectFile(uid string, c playbackCommand) {
	fail := func(err error) { playbackNotify(uid, map[string]string{"error": err.Error()}) }
	if c.Slot < 0 || c.Slot >= len(rockDB.Data) {
		fail(errors.New("録画ログの保存先が不正です"))
		return
	}
	m.Lock()
	s := m.sessions[c.Session]
	_, connected := m.clients[uid]
	m.Unlock()
	if s == nil || !connected {
		return
	}
	s.Lock()
	allowed := s.invited[uid] && (s.owner == "" || s.owner == uid)
	s.Unlock()
	if !allowed {
		fail(errors.New("この再生デバイスは操作できません"))
		return
	}
	schedule, err := scheduler.GetRecordingSchedule(c.Slot, c.ScheduleID)
	if err != nil || schedule == nil {
		fail(errors.New("録画ログを取得できません"))
		return
	}
	if !schedule.Complete && !recordingTSStillActive(schedule.ID) {
		fail(errors.New("未完了の録画は選択できません"))
		return
	}
	file, err := openPlaybackRecording(schedule, c.Slot)
	if err != nil {
		fail(err)
		return
	}
	s.Lock()
	if s.ctx.Err() != nil {
		s.Unlock()
		file.Close()
		return
	}
	if s.nextFile != nil {
		s.nextFile.file.Close()
	}
	s.nextFile = &playbackSelection{schedule, c.Slot, file}
	s.stopRequested = false
	if s.fileCancel != nil {
		s.fileCancel()
	}
	s.Unlock()
	select {
	case s.commands <- playbackCommand{Action: "load"}:
	default:
	}
}

func (m *playbackManager) setIdle(s *playbackSession) {
	s.Lock()
	errorText := s.state.Error
	s.state.Generation++
	s.state.Phase = "idle"
	s.state.PositionMS = 0
	s.state.DurationMS = 0
	s.state.Recording, s.state.WaitingForData = false, false
	s.state.CMStatus = ""
	s.state.CMBoundaries = nil
	s.state.ManualBookmarks = nil
	s.state.ProgramBoundaries = nil
	s.fileCancel = nil
	for _, r := range s.receivers {
		action := "reset"
		if r.idleSupported {
			action = "idle"
		}
		s.legacyControl(r, action)
	}
	uids := make([]string, 0, len(s.invited))
	for uid := range s.invited {
		uids = append(uids, uid)
	}
	generation := s.state.Generation
	s.Unlock()
	m.publish()
	for _, uid := range uids {
		playbackNotify(uid, map[string]any{"action": "status", "session": s.state.Session, "deviceId": s.device.ID, "phase": "idle", "generation": generation})
		if errorText != "" {
			playbackNotify(uid, map[string]any{"action": "status", "session": s.state.Session, "deviceId": s.device.ID, "phase": "failed", "generation": generation, "error": errorText})
		}
	}
}

// 各デバイスが視聴アプリのプロセスを管理し、ファイルごとに再生を中断できる。
func (m *playbackManager) run(s *playbackSession, file *os.File) {
	defer func() {
		s.cancel()
		s.Lock()
		if s.nextFile != nil {
			s.nextFile.file.Close()
			s.nextFile = nil
		}
		for _, r := range s.receivers {
			if r.cancel != nil {
				r.cancel()
			}
		}
		uids := make([]string, 0, len(s.invited))
		for uid := range s.invited {
			uids = append(uids, uid)
		}
		s.Unlock()
		for _, uid := range uids {
			playbackNotify(uid, map[string]any{"action": "close", "session": s.state.Session, "deviceId": s.device.ID})
		}
		m.releasePorts(s.state.Session, "")
		m.Lock()
		delete(m.sessions, s.state.Session)
		m.Unlock()
		m.publish()
	}()
	initial := file != nil
	selection := (*playbackSelection)(nil)
	if file != nil {
		snapshot := s.schedule
		selection = &playbackSelection{&snapshot, 0, file}
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastReady := time.Now()
	for {
		s.Lock()
		stopped := s.stopRequested
		s.stopRequested = false
		phase := s.state.Phase
		s.Unlock()
		if stopped && selection != nil {
			selection.file.Close()
			selection = nil
			if phase != "idle" {
				m.setIdle(s)
			}
		}
		if s.ctx.Err() != nil {
			if selection != nil {
				selection.file.Close()
			}
			return
		}
		if selection != nil {
			s.Lock()
			ctx, cancel := context.WithCancel(s.ctx)
			s.fileContext, s.fileCancel = ctx, cancel
			s.schedule = *selection.schedule
			if !initial {
				s.state.Generation++
			}
			initial = false
			s.state.Phase = "indexing"
			s.state.Error = ""
			s.state.Title = s.schedule.Title
			s.state.ScheduleID = s.schedule.ID
			s.state.Slot = selection.slot
			s.state.PositionMS, s.state.DurationMS = 0, 0
			s.state.Recording, s.state.WaitingForData = false, false
			s.state.CMStatus = ""
			s.state.CMBoundaries = nil
			s.state.ManualBookmarks = nil
			s.state.ProgramBoundaries = nil
			for _, r := range s.receivers {
				s.legacyControl(r, "reset")
			}
			s.Unlock()
			m.publish()
			m.playFile(s, selection.file)
			cancel()
			selection = nil
			if s.ctx.Err() != nil {
				return
			}
			m.setIdle(s)
		}
		s.Lock()
		selection = s.nextFile
		s.nextFile = nil
		count := len(s.invited)
		s.Unlock()
		if count == 0 {
			return
		}
		if selection != nil {
			continue
		}
		select {
		case <-s.ctx.Done():
			return
		case <-s.commands:
		case <-ticker.C:
			s.Lock()
			ready := false
			for _, r := range s.receivers {
				if r.connected && time.Since(r.lastSeen) < 15*time.Second {
					ready = true
				}
			}
			s.Unlock()
			if ready {
				lastReady = time.Now()
			} else if time.Since(lastReady) > 30*time.Second {
				return
			}
		}
	}
}

type playbackRecording struct {
	*RecordingSchedule
	Slot      int  `json:"slot"`
	Recording bool `json:"recording"`
}

func playbackRecordings() ([]playbackRecording, error) {
	active := map[uint64]bool{}
	scheduler.RLock()
	for _, s := range scheduler.CurrentRecording {
		active[s.ID] = true
	}
	scheduler.RUnlock()
	list := []playbackRecording{}
	deleted := map[uint64]string{}
	rows, err := rockDB.GetData(CurrentSlot).Query(`SELECT schedule_id,file_name FROM recording_ts_analysis WHERE file_deleted=1`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id uint64
		var name string
		if err = rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		deleted[id] = name
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	schedules, err := scheduler.GetSchedules(CurrentSlot, time.Unix(0, 0), time.Now().Add(24*time.Hour), 0)
	if err != nil {
		return nil, err
	}
	for _, schedule := range schedules {
		if name, ok := deleted[schedule.ID]; ok && name == schedule.FileName {
			continue
		}
		recording := active[schedule.ID]
		if !schedule.WatchOnly && schedule.FileName != "" && (schedule.Complete || recording) {
			list = append(list, playbackRecording{schedule, CurrentSlot, recording})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].StartTime == list[j].StartTime {
			return list[i].ID > list[j].ID
		}
		return list[i].StartTime > list[j].StartTime
	})
	return list, nil
}

func APIPlaybackRecordings(c *gin.Context) {
	list, err := playbackRecordings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "録画一覧を取得できません"})
		return
	}
	addResponseHeader(c)
	c.JSON(http.StatusOK, list)
}

func (m *playbackManager) startOnAllRockbars(schedule *RecordingSchedule) ([]string, error) {
	m.Lock()
	uids := []string{}
	for uid, client := range m.clients {
		if client.native {
			uids = append(uids, uid)
		}
	}
	m.Unlock()
	sort.Strings(uids)
	sessions := []string{}
	for _, uid := range uids {
		token, err := m.start(schedule, uid)
		if err != nil {
			playbackNotify(uid, map[string]string{"error": err.Error()})
			continue
		}
		sessions = append(sessions, token)
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("再生可能なRockbarまたは空きPlaybackデバイスがありません")
	}
	return sessions, nil
}
