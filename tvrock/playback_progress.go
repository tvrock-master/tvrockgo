package tvrock

import (
	"context"
	"database/sql"
	"log"
	"time"
)

const playbackProgressInterval = 10 * time.Second

func readPlaybackPosition(slotNo int, schedule *RecordingSchedule, duration time.Duration, growing ...bool) time.Duration {
	var milliseconds int64
	err := rockDB.GetData(slotNo).QueryRow(`SELECT resume_position_ms FROM recording_ts_analysis WHERE schedule_id=? AND file_name=? AND file_deleted=0`,
		schedule.ID, schedule.FileName).Scan(&milliseconds)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("error: 視聴位置の読み込み id:%d: %v", schedule.ID, err)
	}
	live := len(growing) > 0 && growing[0]
	if err != nil || milliseconds <= 0 || milliseconds > duration.Milliseconds() || milliseconds == duration.Milliseconds() && !live {
		return 0
	}
	return time.Duration(milliseconds) * time.Millisecond
}

func savePlaybackPosition(slotNo int, schedule *RecordingSchedule, position, duration time.Duration, ended bool, growing ...bool) {
	milliseconds := position.Milliseconds()
	live := len(growing) > 0 && growing[0]
	if ended || milliseconds < 0 || !live && milliseconds >= duration.Milliseconds() {
		milliseconds = 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := rockDB.GetData(slotNo).ExecContext(ctx, `INSERT INTO recording_ts_analysis(schedule_id,file_name,resume_position_ms,playback_updated_at)
 SELECT id,filename,?,? FROM schedule WHERE id=? AND filename=?
 ON CONFLICT(schedule_id) DO UPDATE SET resume_position_ms=excluded.resume_position_ms,playback_updated_at=excluded.playback_updated_at
 WHERE recording_ts_analysis.file_name=excluded.file_name`, milliseconds, time.Now().UnixMilli(), schedule.ID, schedule.FileName)
	if err != nil {
		log.Printf("error: 視聴位置の保存 id:%d: %v", schedule.ID, err)
	}
}

func markPlaybackFileDeleted(slotNo int, schedule *RecordingSchedule) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := rockDB.GetData(slotNo).ExecContext(ctx, `INSERT INTO recording_ts_analysis(schedule_id,file_name,file_deleted,playback_updated_at,status)
 SELECT id,filename,1,?,'error' FROM schedule WHERE id=? AND filename=?
 ON CONFLICT(schedule_id) DO UPDATE SET resume_position_ms=CASE WHEN recording_ts_analysis.file_name=excluded.file_name THEN
 recording_ts_analysis.resume_position_ms ELSE 0 END,manual_bookmarks_json=CASE WHEN recording_ts_analysis.file_name=excluded.file_name THEN
 recording_ts_analysis.manual_bookmarks_json ELSE '[]' END,file_name=excluded.file_name,file_deleted=1,playback_updated_at=excluded.playback_updated_at
 WHERE recording_ts_analysis.file_deleted=0 OR recording_ts_analysis.file_name<>excluded.file_name`, time.Now().UnixMilli(), schedule.ID, schedule.FileName)
	if err != nil {
		log.Printf("error: 録画ファイル削除フラグの保存 id:%d: %v", schedule.ID, err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		deviceController.Notify(ControllerUpdateSchedule, "")
	}
}

func clearPlaybackFileDeleted(slotNo int, schedule *RecordingSchedule) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := rockDB.GetData(slotNo).ExecContext(ctx, `UPDATE recording_ts_analysis SET file_deleted=0 WHERE 
schedule_id=? AND file_name=? AND file_deleted<>0`, schedule.ID, schedule.FileName)
	if err != nil {
		log.Printf("error: 録画ファイル削除フラグの解除 id:%d: %v", schedule.ID, err)
		return
	}
	if n, _ := result.RowsAffected(); n > 0 {
		deviceController.Notify(ControllerUpdateSchedule, "")
	}
}
