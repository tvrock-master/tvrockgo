package tvrock

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

type ArchiverSetting struct {
	ExecutionInterval int64 `yaml:"executioninterval"`
	SuspendTime       int64 `yaml:"suspendtime"`
	BleveInsertMax    int64 `yaml:"bleveinsertmax"`
}

var lastUpdate int64

func InitializeArchiver() error {

	if !configure.BootArchiver {
		return nil
	}

	// cronへ登録
	cronManager.Add(TaskArchiver, func() {
		now := Now().Unix()

		sch, err := scheduler.GetNextSchedules()
		if err != nil {
			log.Println("error: ", err)
			return
		}

		if len(sch) > 0 && sch[0].StartTime < now+configure.ArchiverSetting.SuspendTime {
			return
		}

		if lastUpdate > Now().Unix()-configure.ArchiverSetting.ExecutionInterval {
			return
		}

		if err := Archive(); err != nil && serviceContext().Err() == nil {
			log.Println("error:", err)
		}

		lastUpdate = now
	})

	return nil
}

func Archive() error {
	return archiveContext(serviceContext())
}

func archiveContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	log.Println("info: 古いスケジュールをアーカイブへ移動")

	if err := archiveScheduleContext(ctx); err != nil {
		return err
	}

	log.Println("info: 古い番組情報をアーカイブへ移動")

	if err := archiveProgramContext(ctx); err != nil {
		return err
	}

	log.Println("info: 古いキーワードをアーカイブへ移動")

	if err := archiveKeywordReserveContext(ctx); err != nil {
		return err
	}

	log.Println("info: 古い字幕をアーカイブへ移動")

	if err := archiveCaptionContext(ctx); err != nil {
		return err
	}

	// Vacuum
	log.Println("info: データベースのvacuumを実行")

	if err := vacuumContext(ctx, rockDB.GetData(0)); err != nil {
		return err
	}
	if err := vacuumContext(ctx, rockDB.GetProgram(0)); err != nil {
		return err
	}
	if rockDB.GetCaption(0) != nil {
		if err := vacuumContext(ctx, rockDB.GetCaption(0)); err != nil {
			return err
		}
	}

	log.Println("info: アーカイブを終了")

	return nil
}

func ArchiveSchedule() error {
	return archiveScheduleContext(serviceContext())
}

func archiveScheduleContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := Now().Unix()
	spoint := now - configure.CurrentProgramThreshold

	// スケジュール、分析、および削除には単一のソーススナップショットを使用すること。
	// 古いコピーがアーカイブされた後に同時進行中の再生更新が削除されるような事態は避けなければならない。
	tx2, err := rockDB.GetData(0).BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx2.Rollback()
	rows, err := tx2.QueryContext(ctx, "SELECT id,deviceid,title,customcomid,filename,"+
		"validate,watchonly,recordonly,shutdown,complete,epgfollow,autoselect,"+
		"starttime,endtime,idletime,readytime,tailtime,lastupdate,"+
		"sid,eid,searchid,priority,exitmode,no,errors"+
		" FROM schedule WHERE endtime<? ORDER BY starttime", spoint)

	if err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	defer rows.Close()
	tx1, err := rockDB.GetData(1).BeginTx(ctx, nil)
	if err != nil {
		rows.Close()
		return err
	}

	defer tx1.Rollback()
	stmt, err := tx1.PrepareContext(ctx,
		`REPLACE INTO schedule (id,deviceid,title,customcomid,filename,`+
			`validate,watchonly,recordonly,shutdown,complete,epgfollow,autoselect,`+
			`starttime,endtime,idletime,readytime,tailtime,lastupdate,`+
			`sid,eid,searchid,priority,exitmode,no,errors) VALUES`+
			`(?,?,?,?,? ,?,?,?,?,?,?,? ,?,?,?,?,?,? ,?,?,?,?,?,?,?)`)
	if err != nil {
		rows.Close()
		tx1.Rollback()
		return err
	}

	var rs *RecordingSchedule
	defer stmt.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
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
			stmt.Close()
			rows.Close()
			tx1.Rollback()
			return err
		}

		_, err := stmt.ExecContext(ctx,
			rs.ID,
			rs.DeviceID,
			rs.Title,
			rs.CustomComID,
			fileName,

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
		)

		if err != nil {
			stmt.Close()
			rows.Close()
			tx1.Rollback()
			return err
		}
	}
	stmt.Close()
	if err := rows.Err(); err != nil {
		rows.Close()
		tx1.Rollback()
		return err
	}
	if err := rows.Close(); err != nil {
		tx1.Rollback()
		return err
	}

	if err := copyTSAnalysisForArchive(ctx, tx2, tx1, spoint); err != nil {
		return err
	}
	// 分析および再生の状態を同一のスケジュール・カットオフの対象とする。
	// また、SQLiteの外部キー制約や削除トリガーが無効な場合であっても
	// ソースの削除がアトミック（不可分）に行われるようにする。
	_, err = tx2.ExecContext(ctx, `DELETE FROM recording_ts_analysis WHERE schedule_id IN (SELECT id FROM schedule WHERE endtime<?)`, spoint)
	if err != nil {
		return fmt.Errorf("delete archived TS analysis: %w", err)
	}
	_, err = tx2.ExecContext(ctx, `DELETE FROM schedule WHERE endtime<?`, spoint)
	if err != nil {
		tx1.Rollback()
		tx2.Rollback()
		return err
	}

	err = tx1.Commit()
	if err != nil {
		tx1.Rollback()
		tx2.Rollback()
		return err
	}
	err = tx2.Commit()
	if err != nil {
		tx2.Rollback()
		return err
	}

	return nil
}

type tsAnalysisArchiveSource interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func copyTSAnalysisForArchive(ctx context.Context, source tsAnalysisArchiveSource, destination *sql.Tx, before int64) error {
	columns := tsAnalysisColumns + "," + tsPlaybackColumns
	rows, err := source.QueryContext(ctx, `SELECT `+columns+` FROM recording_ts_analysis WHERE schedule_id IN (SELECT id FROM schedule WHERE endtime<?)`, before)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		values := make([]any, len(strings.Split(columns, ",")))
		pointers := make([]any, len(values))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			return err
		}
		if _, err = destination.ExecContext(ctx, tsAnalysisUpsert(false), values...); err != nil {
			return fmt.Errorf("archive TS analysis: %w", err)
		}
	}
	return rows.Err()
}

func ArchiveProgram() error {
	return archiveProgramContext(serviceContext())
}

func archiveProgramContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := Now().Unix()
	spoint := now - configure.CurrentProgramThreshold

	rows, err := rockDB.GetProgram(0).QueryContext(ctx, "SELECT sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b,common_sid,common_eid FROM program WHERE end<? ORDER BY start", spoint)
	if err != nil {
		return err
	}
	defer rows.Close()
	tx1, err := rockDB.GetProgram(1).BeginTx(ctx, nil)
	if err != nil {
		rows.Close()
		return err
	}

	defer tx1.Rollback()
	stmt, err := tx1.PrepareContext(ctx,
		`REPLACE INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b,common_sid,common_eid) VALUES (?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		rows.Close()
		tx1.Rollback()
		return err
	}

	defer stmt.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		tvp := TvProgram{}
		err = rows.Scan(&tvp.Sid, &tvp.Eid, &tvp.Start, &tvp.End, &tvp.Title, &tvp.Subtitle, &tvp.Content, &tvp.CnibA, &tvp.CnibB, &tvp.CommonSid, &tvp.CommonEid)
		if err != nil {
			stmt.Close()
			rows.Close()
			tx1.Rollback()
			return err
		}

		_, err = stmt.ExecContext(ctx,
			tvp.Sid,
			tvp.Eid,
			tvp.Start,
			tvp.End,
			tvp.Title,
			tvp.Subtitle,
			tvp.Content,
			tvp.CnibA,
			tvp.CnibB,
			tvp.CommonSid,
			tvp.CommonEid,
		)
		if err != nil {
			stmt.Close()
			rows.Close()
			tx1.Rollback()
			return err
		}
	}
	stmt.Close()
	if err := rows.Err(); err != nil {
		rows.Close()
		tx1.Rollback()
		return err
	}
	if err := rows.Close(); err != nil {
		tx1.Rollback()
		return err
	}

	tx2, err2 := rockDB.GetProgram(0).BeginTx(ctx, nil)
	if err2 != nil {
		tx1.Rollback()
		return err2
	}
	defer tx2.Rollback()
	_, err = tx2.ExecContext(ctx, `DELETE FROM program WHERE end<?`, spoint)
	if err != nil {
		tx1.Rollback()
		tx2.Rollback()
		return err
	}

	err = tx1.Commit()
	if err != nil {
		tx1.Rollback()
		tx2.Rollback()
		return err
	}
	err = tx2.Commit()
	if err != nil {
		tx2.Rollback()
		return err
	}

	return nil
}

func ArchiveKeywordReserve() error {
	return archiveKeywordReserveContext(serviceContext())
}

func archiveKeywordReserveContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := Now().Unix()
	spoint := now - configure.CurrentKeywordReserveThreshold

	rows, err := rockDB.GetData(0).QueryContext(ctx, "SELECT id,validate,"+
		"deviceid,starttime,endtime,starthours,hours,"+
		"idletime,readytime,tailtime,"+
		"title,content,days,sid,category,week,andsearch,fuzzysearch,"+
		"recordonly,watchonly,shutdown,epgfollow,customcomid,hashtag,"+
		"createdat,updatedat FROM keywordreserve WHERE updatedat<? ORDER BY id DESC", spoint)
	if err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	defer rows.Close()
	tx1, err := rockDB.GetData(1).BeginTx(ctx, nil)
	if err != nil {
		rows.Close()
		return err
	}

	defer tx1.Rollback()
	stmt, err := tx1.PrepareContext(ctx,
		`REPLACE INTO keywordreserve (id,validate,`+
			`deviceid,starttime,endtime,starthours,hours,`+
			`idletime,readytime,tailtime,`+
			`title,content,days,sid,category,week,andsearch,fuzzysearch,`+
			`recordonly,watchonly,shutdown,epgfollow,customcomid,hashtag,`+
			`createdat,updatedat) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		rows.Close()
		tx1.Rollback()
		return err
	}

	defer stmt.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		kr := KeywordReserve{}
		var sid string
		err = rows.Scan(
			&kr.ID,
			&kr.Validate,
			&kr.DeviceID,
			&kr.StartTime,
			&kr.EndTime,
			&kr.StartHour,
			&kr.Hours,
			&kr.IdleTime,
			&kr.ReadyTime,
			&kr.TailTime,
			&kr.Title,
			&kr.Content,
			&kr.Days,
			&sid,
			&kr.Category,
			&kr.Week,
			&kr.AndSearch,
			&kr.FuzzySearch,
			&kr.RecordOnly,
			&kr.WatchOnly,
			&kr.ShutDown,
			&kr.EpgFollow,
			&kr.CustomComID,
			&kr.HashTag,
			&kr.CreatedAt,
			&kr.UpdatedAt,
		)
		if err != nil {
			stmt.Close()
			rows.Close()
			tx1.Rollback()
			return err
		}
		kr.Sid = splitUintString(sid)
		if kr.Category == 0 {
			kr.Category = 0xffff
		}

		_, err := stmt.ExecContext(ctx,
			kr.ID,
			boolToInt(kr.Validate),

			kr.DeviceID,
			kr.StartTime,
			kr.EndTime,
			kr.StartHour,
			kr.Hours,

			kr.IdleTime,
			kr.ReadyTime,
			kr.TailTime,

			kr.Title,
			kr.Content,
			kr.Days,
			joinUintToString(kr.Sid, ","),
			kr.Category,
			kr.Week,
			boolToInt(kr.AndSearch),
			boolToInt(kr.FuzzySearch),
			boolToInt(kr.RecordOnly),
			boolToInt(kr.WatchOnly),
			boolToInt(kr.ShutDown),
			boolToInt(kr.EpgFollow),
			kr.CustomComID,
			kr.HashTag,

			kr.CreatedAt,
			kr.UpdatedAt,
		)
		if err != nil {
			stmt.Close()
			rows.Close()
			tx1.Rollback()
			return err
		}
	}
	stmt.Close()
	if err := rows.Err(); err != nil {
		rows.Close()
		tx1.Rollback()
		return err
	}
	if err := rows.Close(); err != nil {
		tx1.Rollback()
		return err
	}

	tx2, err2 := rockDB.GetData(0).BeginTx(ctx, nil)
	if err2 != nil {
		tx1.Rollback()
		return err2
	}
	defer tx2.Rollback()
	_, err = tx2.ExecContext(ctx, `DELETE FROM keywordreserve WHERE updatedat<?`, spoint)
	if err != nil {
		tx1.Rollback()
		tx2.Rollback()
		return err
	}

	err = tx1.Commit()
	if err != nil {
		tx1.Rollback()
		tx2.Rollback()
		return err
	}
	err = tx2.Commit()
	if err != nil {
		tx2.Rollback()
		return err
	}

	return nil
}

// Bleveへ番組情報登録
func IndexingArchiveProgram() error {
	return indexingArchiveProgramContext(serviceContext())
}

func indexingArchiveProgramContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !configure.Bleve {
		return nil
	}

	bleveProgram.archiveMu.Lock()
	defer bleveProgram.archiveMu.Unlock()
	// 別のカーソルが、まだキャプションが付いていない古いドキュメントを再インデックスします。
	startdate, startsid, starteid, err := bleveProgram.lastCaptionProgramKey()
	if err != nil {
		return err
	}

	limit := configure.ArchiverSetting.BleveInsertMax
	if limit < 1 {
		limit = 200
	}

	rows, err := rockDB.GetProgram(1).QueryContext(ctx,
		commonProgramSelect(" INDEXED BY program_12")+`
		WHERE (p.start>? OR (p.start=? AND (p.sid>? OR (p.sid=? AND p.eid>?)))) AND p.sid>0 AND p.eid>0
		ORDER BY p.start,p.sid,p.eid
		LIMIT ?`,
		startdate, startdate, startsid, startsid, starteid, limit)
	if err != nil {
		return err
	}

	defer rows.Close()

	total := 0
	add := []*bleveProgramDocument{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		tvp := TvProgram{}
		err = rows.Scan(&tvp.Sid, &tvp.Eid, &tvp.Start, &tvp.End, &tvp.Title, &tvp.Subtitle, &tvp.Content, &tvp.CnibA, &tvp.CnibB, &tvp.CommonSid, &tvp.CommonEid)
		if err != nil {
			return err
		}
		if tvp.Sid == 0 || tvp.Eid == 0 {
			continue
		}

		caption, err := rockDB.CaptionText(ctx, tvp.Sid, tvp.Eid, tvp.Start)
		if err != nil {
			return err
		}
		add = append(add, newBleveProgramDocument(&tvp, caption))
		total = total + 1
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(add) < 1 {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := bleveProgram.BatchIndexProgram(add...); err != nil {
		log.Println("error: bleve index:", err)
		return err
	}

	log.Println("debug: 過去番組を全文検索へ登録 ", total)

	return nil
}

func ArchiveCaption() error {
	return archiveCaptionContext(serviceContext())
}

func archiveCaptionContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if rockDB.GetCaption(0) == nil || rockDB.GetCaption(1) == nil {
		return nil
	}

	threshold := configure.CurrentCaptionThreshold
	if threshold <= 0 {
		threshold = 604800 // デフォルト: 7日 (604800秒)
	}

	now := Now().Unix()
	spoint := now - threshold

	rows, err := rockDB.GetCaption(0).QueryContext(ctx,
		"SELECT id,sid,eid,start,pts,lang,clear,x,y,color,colorname,bgcolor,size,cap FROM caption WHERE start<? ORDER BY id DESC", spoint)
	if err != nil {
		return err
	}

	defer rows.Close()
	tx1, err := rockDB.GetCaption(1).BeginTx(ctx, nil)
	if err != nil {
		rows.Close()
		return err
	}

	defer tx1.Rollback()
	stmt, err := tx1.PrepareContext(ctx,
		`REPLACE INTO caption (id,sid,eid,start,pts,lang,clear,x,y,color,colorname,bgcolor,size,cap) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		rows.Close()
		tx1.Rollback()
		return err
	}

	defer stmt.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		cap := CaptionInfo{}
		err = rows.Scan(
			&cap.HistoryID,
			&cap.Sid,
			&cap.Eid,
			&cap.Start,
			&cap.Pts,
			&cap.Lang,
			&cap.Clear,
			&cap.X,
			&cap.Y,
			&cap.Color,
			&cap.ColorName,
			&cap.BgColor,
			&cap.Size,
			&cap.Text,
		)
		if err != nil {
			stmt.Close()
			rows.Close()
			tx1.Rollback()
			return err
		}
		_, err := stmt.ExecContext(ctx,
			cap.HistoryID,
			cap.Sid,
			cap.Eid,
			cap.Start,
			cap.Pts,
			cap.Lang,
			boolToInt(cap.Clear),
			cap.X,
			cap.Y,
			cap.Color,
			cap.ColorName,
			cap.BgColor,
			cap.Size,
			cap.Text,
		)
		if err != nil {
			stmt.Close()
			rows.Close()
			tx1.Rollback()
			return err
		}
	}
	stmt.Close()

	if err := rows.Err(); err != nil {
		rows.Close()
		tx1.Rollback()
		return err
	}
	if err := rows.Close(); err != nil {
		tx1.Rollback()
		return err
	}

	tx2, err2 := rockDB.GetCaption(0).BeginTx(ctx, nil)
	if err2 != nil {
		tx1.Rollback()
		return err2
	}
	defer tx2.Rollback()
	_, err = tx2.ExecContext(ctx, `DELETE FROM caption WHERE start<?`, spoint)
	if err != nil {
		tx1.Rollback()
		tx2.Rollback()
		return err
	}

	err = tx1.Commit()
	if err != nil {
		tx1.Rollback()
		tx2.Rollback()
		return err
	}
	err = tx2.Commit()
	if err != nil {
		tx2.Rollback()
		return err
	}

	return nil
}
