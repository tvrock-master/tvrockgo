package tvrock

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"sort"
	"sync"
	"time"
)

var playbackBookmarkMu sync.Mutex

func readManualPlaybackBookmarks(ctx context.Context, slotNo int, schedule *RecordingSchedule) ([]int64, error) {
	var raw string
	err := rockDB.GetData(slotNo).QueryRowContext(ctx, `SELECT manual_bookmarks_json FROM recording_ts_analysis WHERE schedule_id=? AND file_name=?`,
		schedule.ID, schedule.FileName).Scan(&raw)
	if err == sql.ErrNoRows {
		return []int64{}, nil
	}
	if err != nil {
		return nil, err
	}

	var positions []int64
	if err = json.Unmarshal([]byte(raw), &positions); err != nil {
		return nil, err
	}
	return positions, nil
}

func manualPlaybackBoundaries(positions []int64, duration int64) []playbackBoundary {
	rows := []playbackBoundary{}
	for _, position := range positions {
		if position >= 0 && position <= duration {
			rows = append(rows, playbackBoundary{PositionMS: position, Kind: "manual"})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].PositionMS < rows[j].PositionMS })
	return rows
}

func loadManualPlaybackBookmarks(slotNo int, schedule *RecordingSchedule, duration int64) []playbackBoundary {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	positions, err := readManualPlaybackBookmarks(ctx, slotNo, schedule)
	if err != nil {
		log.Printf("error: 手動しおりの読み込み id:%d: %v", schedule.ID, err)
		return nil
	}
	return manualPlaybackBoundaries(positions, duration)
}

func changeManualPlaybackBookmark(slotNo int, schedule *RecordingSchedule, position, duration int64, remove bool) ([]playbackBoundary, error) {
	if duration <= 0 || position < 0 || position > duration {
		return nil, errors.New("しおりの位置が不正です")
	}
	playbackBookmarkMu.Lock()
	defer playbackBookmarkMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	positions, err := readManualPlaybackBookmarks(ctx, slotNo, schedule)
	if err != nil {
		return nil, err
	}
	if remove {
		kept := make([]int64, 0, len(positions))
		for _, existing := range positions {
			if existing != position {
				kept = append(kept, existing)
			}
		}
		positions = kept
	} else {
		for _, existing := range positions {
			if existing >= position-1000 && existing <= position+1000 {
				return manualPlaybackBoundaries(positions, duration), nil
			}
		}
		positions = append(positions, position)
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })
	raw, err := json.Marshal(positions)
	if err != nil {
		return nil, err
	}
	result, err := rockDB.GetData(slotNo).ExecContext(ctx, `INSERT INTO recording_ts_analysis(schedule_id,file_name,manual_bookmarks_json)
 SELECT id,filename,? FROM schedule WHERE id=? AND filename=?
 ON CONFLICT(schedule_id) DO UPDATE SET manual_bookmarks_json=excluded.manual_bookmarks_json WHERE recording_ts_analysis.file_name=excluded.file_name`,
		string(raw), schedule.ID, schedule.FileName)
	if err != nil {
		return nil, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil, errors.New("録画ファイルの情報が変更されています")
	}
	return manualPlaybackBoundaries(positions, duration), nil
}

func (m *playbackManager) updateManualBookmarks(slotNo int, schedule *RecordingSchedule) {
	playbackBookmarkMu.Lock()
	defer playbackBookmarkMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	positions, err := readManualPlaybackBookmarks(ctx, slotNo, schedule)
	if err != nil {
		log.Printf("error: 手動しおり更新 id:%d: %v", schedule.ID, err)
		return
	}
	m.Lock()
	sessions := make([]*playbackSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.Unlock()
	for _, s := range sessions {
		s.Lock()
		if s.state.Slot == slotNo && s.schedule.ID == schedule.ID && s.schedule.FileName == schedule.FileName && s.state.Phase != "idle" && s.state.Phase != "indexing" {
			s.state.ManualBookmarks = manualPlaybackBoundaries(positions, s.state.DurationMS)
		}
		s.Unlock()
	}
	m.publish()
}
