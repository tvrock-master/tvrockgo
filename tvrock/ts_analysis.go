package tvrock

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

const tsAnalysisVersion = 6

// 既存のversion列に、解析方式と実際に使用した局別ルールの識別情報を保存する。
// ルールのない局や他の局の設定変更で、無関係な録画のキャッシュは無効にしない。
func tsAnalysisCacheVersion(scanner *tsPIDCMScanner) int64 {
	if scanner == nil {
		return tsAnalysisVersion
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d/%d/%d/%d/%d", tsAnalysisVersion, scanner.pid, scanner.forward, scanner.back, scanner.timeout)))
	return int64(binary.BigEndian.Uint64(digest[:])&0x7fffffffffffff00) | tsAnalysisVersion
}

var tsAnalysisWake = make(chan struct{}, 1)
var tsAnalysisGate = make(chan struct{}, 1)
var tsAnalysisRunning atomic.Int32

type tsFileIdentity struct {
	size, mtime int64
	fingerprint string
}

type tsStoredPoint struct {
	Offset int64 `json:"offset"`
	TimeNS int64 `json:"timeNs"`
}

type tsStreamInfo struct {
	PID   uint16 `json:"pid"`
	Type  uint8  `json:"streamType"`
	Codec string `json:"codec"`
}

type tsBoundaryCandidate struct {
	TimeNS          int64  `json:"timeNs"`
	Offset          int64  `json:"signalOffset"`
	Kind            string `json:"kind"`
	Evidence        string `json:"evidence"`
	EventID         uint32 `json:"eventId"`
	ConfirmedCM     bool   `json:"confirmedCM"`
	PreviousEventID uint16 `json:"previousEventId,omitempty"`
}

type tsAnalysisMetrics struct {
	ServiceID, TransportStreamID                                  uint16
	PMTPID                                                        uint16
	Streams                                                       []tsStreamInfo
	Packets, TEI, Scrambled, ContinuityErrors, PCRDiscontinuities int64
	AverageBitrate, AnalysisMS                                    int64
	CMStatus                                                      string
	Candidates                                                    []tsBoundaryCandidate
	ProgramBoundaries                                             []tsBoundaryCandidate
}

const tsAnalysisColumns = `schedule_id,
file_name,
file_size,
mtime_ns,
fingerprint,
service_id,
version,
status,
queued_at,
started_at,
finished_at,
duration_ns,
pcr_pid,
pmt_pid,
packet_count,
tei_count,
scrambled_count,
continuity_errors,
pcr_discontinuities,
average_bitrate,
analysis_ms,
streams_json,
seek_points_json,
cm_status,
cm_candidates_json,
program_boundaries_json,
error`

const tsPlaybackColumns = `resume_position_ms,file_deleted,playback_updated_at,manual_bookmarks_json`

func tsAnalysisActive() bool {
	return tsAnalysisRunning.Load() > 0
}

func createTSAnalysisTable(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS recording_ts_analysis (" +
		"schedule_id INTEGER PRIMARY KEY REFERENCES schedule(id) ON DELETE CASCADE," +
		"file_name TEXT NOT NULL DEFAULT ''," +
		"file_size INTEGER NOT NULL DEFAULT 0," +
		"mtime_ns INTEGER NOT NULL DEFAULT 0," +
		"fingerprint TEXT NOT NULL DEFAULT ''," +
		"service_id INTEGER NOT NULL DEFAULT 0," +
		"version INTEGER NOT NULL DEFAULT 0," +
		"status TEXT NOT NULL DEFAULT 'queued'," +
		"queued_at INTEGER NOT NULL DEFAULT 0," +
		"started_at INTEGER NOT NULL DEFAULT 0," +
		"finished_at INTEGER NOT NULL DEFAULT 0," +
		"duration_ns INTEGER NOT NULL DEFAULT 0," +
		"pcr_pid INTEGER NOT NULL DEFAULT 0," +
		"pmt_pid INTEGER NOT NULL DEFAULT 0," +
		"packet_count INTEGER NOT NULL DEFAULT 0," +
		"tei_count INTEGER NOT NULL DEFAULT 0," +
		"scrambled_count INTEGER NOT NULL DEFAULT 0," +
		"continuity_errors INTEGER NOT NULL DEFAULT 0," +
		"pcr_discontinuities INTEGER NOT NULL DEFAULT 0," +
		"average_bitrate INTEGER NOT NULL DEFAULT 0," +
		"analysis_ms INTEGER NOT NULL DEFAULT 0," +
		"streams_json TEXT NOT NULL DEFAULT '[]'," +
		"seek_points_json TEXT NOT NULL DEFAULT '[]'," +
		"cm_status TEXT NOT NULL DEFAULT 'not-analyzed'," +
		"cm_candidates_json TEXT NOT NULL DEFAULT '[]'," +
		"program_boundaries_json TEXT NOT NULL DEFAULT '[]'," +
		"resume_position_ms INTEGER NOT NULL DEFAULT 0," +
		"file_deleted INTEGER NOT NULL DEFAULT 0," +
		"playback_updated_at INTEGER NOT NULL DEFAULT 0," +
		"manual_bookmarks_json TEXT NOT NULL DEFAULT '[]'," +
		"error TEXT NOT NULL DEFAULT '')")
	if err != nil {
		return err
	}
	if _, err = db.Exec(`CREATE INDEX IF NOT EXISTS recording_ts_analysis_queue ON recording_ts_analysis(status,queued_at)`); err != nil {
		return err
	}
	// 既存のインストール環境において外部キー（foreign_keys）が有効になっていなかった場合、クリーンアップも行います
	_, err = db.Exec(`CREATE TRIGGER IF NOT EXISTS schedule_delete_ts_analysis AFTER DELETE ON schedule BEGIN DELETE FROM recording_ts_analysis WHERE schedule_id=OLD.id; END`)
	return err
}

func identifyTSFile(file *os.File) (tsFileIdentity, error) {
	info, err := file.Stat()
	if err != nil {
		return tsFileIdentity{}, err
	}
	if !info.Mode().IsRegular() {
		return tsFileIdentity{}, errors.New("通常のTSファイルではありません")
	}
	h := sha256.New()
	for _, offset := range []int64{0, max(0, info.Size()-65536)} {
		if _, err = io.Copy(h, io.NewSectionReader(file, offset, min(65536, info.Size()-offset))); err != nil {
			return tsFileIdentity{}, err
		}
	}
	return tsFileIdentity{info.Size(), info.ModTime().UnixNano(), hex.EncodeToString(h.Sum(nil))}, nil
}

func readTSAnalysis(ctx context.Context, db *sql.DB, schedule *RecordingSchedule, identity tsFileIdentity) (*playbackIndex, error) {
	var duration int64
	var pid, pmt uint16
	var streams, points, candidates, cmStatus, programs string
	version := tsAnalysisCacheVersion(newTSPIDCMScanner(schedule.Sid))
	index := &playbackIndex{analysisVersion: version}
	err := db.QueryRowContext(ctx, `SELECT duration_ns,pcr_pid,pmt_pid,
streams_json,seek_points_json,packet_count,tei_count,scrambled_count,continuity_errors,
pcr_discontinuities,average_bitrate,analysis_ms,cm_status,cm_candidates_json,program_boundaries_json
 FROM recording_ts_analysis WHERE schedule_id=? AND file_name=? AND file_size=? AND mtime_ns=?
 AND fingerprint=? AND service_id=? AND version=? AND status='ready' AND length(seek_points_json)<67108864`,
		schedule.ID,
		schedule.FileName,
		identity.size,
		identity.mtime,
		identity.fingerprint,
		schedule.Sid,
		version).Scan(&duration, &pid, &pmt, &streams, &points,
		&index.metrics.Packets, &index.metrics.TEI, &index.metrics.Scrambled,
		&index.metrics.ContinuityErrors, &index.metrics.PCRDiscontinuities,
		&index.metrics.AverageBitrate, &index.metrics.AnalysisMS, &cmStatus, &candidates, &programs)
	if err != nil {
		return nil, err
	}

	var entries []tsStoredPoint
	if duration <= 0 || pid >= 8191 || json.Unmarshal([]byte(points), &entries) != nil || len(entries) == 0 ||
		json.Unmarshal([]byte(streams), &index.metrics.Streams) != nil ||
		json.Unmarshal([]byte(candidates), &index.metrics.Candidates) != nil ||
		json.Unmarshal([]byte(programs), &index.metrics.ProgramBoundaries) != nil {
		return nil, errors.New("TS解析データが不正です")
	}

	for i, point := range entries {
		if point.Offset < 0 || point.Offset >= identity.size || point.Offset%188 != 0 ||
			point.TimeNS < 0 || point.TimeNS > duration ||
			i > 0 && (point.Offset <= entries[i-1].Offset || point.TimeNS < entries[i-1].TimeNS) {
			return nil, errors.New("TS解析の索引が不正です")
		}
		index.points = append(index.points, playbackPoint{point.Offset, time.Duration(point.TimeNS)})
	}

	index.duration, index.pcrPID = time.Duration(duration), pid
	index.metrics.PMTPID, index.metrics.CMStatus = pmt, cmStatus
	return index, nil
}

func tsAnalysisUpsert(conditional bool) string {
	columnNames := tsAnalysisColumns
	if !conditional {
		columnNames += "," + tsPlaybackColumns
	}

	columns := strings.Split(columnNames, ",")
	updates := make([]string, 0, len(columns)-1)
	for _, column := range columns[1:] {
		updates = append(updates, column+"=excluded."+column)
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(columns)), ",")
	values := "VALUES (" + placeholders + ")"
	if conditional {
		values = "SELECT " + placeholders + " WHERE EXISTS (SELECT 1 FROM schedule WHERE id=? AND filename=? AND sid=?)"
		updates = append(updates, "resume_position_ms=CASE WHEN recording_ts_analysis.file_name=excluded.file_name THEN"+
			" recording_ts_analysis.resume_position_ms ELSE 0 END", "manual_bookmarks_json=CASE WHEN recording_ts_analysis.file_name=excluded.file_name THEN"+
			" recording_ts_analysis.manual_bookmarks_json ELSE '[]' END", "file_deleted=0")
	}

	return `INSERT INTO recording_ts_analysis (` + columnNames + `) ` + values + ` ON CONFLICT(schedule_id) DO UPDATE SET ` + strings.Join(updates, ",")
}

func saveTSAnalysis(ctx context.Context, db *sql.DB, schedule *RecordingSchedule, identity tsFileIdentity, index *playbackIndex, started time.Time) error {
	points := make([]tsStoredPoint, 0, len(index.points))
	for _, p := range index.points {
		points = append(points, tsStoredPoint{p.offset, int64(p.time)})
	}

	streams, _ := json.Marshal(index.metrics.Streams)
	candidates, _ := json.Marshal(index.metrics.Candidates)
	programs, _ := json.Marshal(index.metrics.ProgramBoundaries)
	seek, _ := json.Marshal(points)
	_, err := db.ExecContext(ctx, tsAnalysisUpsert(true), schedule.ID, schedule.FileName, identity.size,
		identity.mtime, identity.fingerprint, schedule.Sid, index.analysisVersion, "ready",
		started.UnixMilli(), started.UnixMilli(), time.Now().UnixMilli(), int64(index.duration),
		index.pcrPID, index.metrics.PMTPID, index.metrics.Packets, index.metrics.TEI, index.metrics.Scrambled,
		index.metrics.ContinuityErrors, index.metrics.PCRDiscontinuities, index.metrics.AverageBitrate,
		index.metrics.AnalysisMS, string(streams), string(seek), index.metrics.CMStatus, string(candidates), string(programs),
		"", schedule.ID, schedule.FileName, schedule.Sid)
	return err
}

func loadOrAnalyzePlaybackTS(ctx context.Context, file *os.File, schedule *RecordingSchedule, slotNo int) (*playbackIndex, error) {
	if recordingTSStillActive(schedule.ID) {
		return loadGrowingPlaybackTS(ctx, file, schedule)
	}

	index, err := analyzeRecordedTS(ctx, file, schedule, slotNo, CurrentRecorderSetting().AnalyzeTSAfterRecording)
	if index != nil && err != nil && ctx.Err() == nil {
		log.Printf("error: 再生用TS解析結果の保存 id:%d: %v", schedule.ID, err)
		return index, nil
	}
	return index, err
}

func analyzeRecordedTS(ctx context.Context, file *os.File, schedule *RecordingSchedule, slotNo int, useCache bool) (*playbackIndex, error) {
	tsAnalysisRunning.Add(1)
	defer tsAnalysisRunning.Add(-1)
	db := rockDB.GetData(slotNo)
	// 利用可能なインデックスが無関係なバックグラウンドのファイルスキャンによって待たされるようなことがあってはならない。
	if useCache && schedule.ID != 0 {
		if identity, err := identifyTSFile(file); err == nil {
			if cached, e := readTSAnalysis(ctx, db, schedule, identity); e == nil {
				_, err = file.Seek(0, io.SeekStart)
				return cached, err
			}
		}
	}

	select {
	case tsAnalysisGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	defer func() { <-tsAnalysisGate }()
	identity, err := identifyTSFile(file)
	if err != nil {
		return nil, err
	}

	if useCache && schedule.ID != 0 {
		if cached, e := readTSAnalysis(ctx, db, schedule, identity); e == nil {
			_, err = file.Seek(0, io.SeekStart)
			return cached, err
		}
	}

	started := time.Now()
	index, err := indexPlaybackTS(ctx, file, schedule.Sid)
	if err != nil {
		return nil, err
	}
	after, err := identifyTSFile(file)
	if err != nil {
		return nil, err
	}
	if identity != after {
		return nil, errors.New("解析中に録画ファイルが変更されました")
	}
	index.metrics.AnalysisMS = time.Since(started).Milliseconds()
	if schedule.ID != 0 && ctx.Err() == nil {
		if err = saveTSAnalysis(ctx, db, schedule, identity, index, started); err != nil {
			return index, err
		}
	}
	return index, ctx.Err()
}

func queueRecordingTSAnalysis(slotNo int, id uint64) {
	if !CurrentRecorderSetting().AnalyzeTSAfterRecording || serviceContext().Err() != nil {
		return
	}

	db := rockDB.GetData(slotNo)
	_, err := db.ExecContext(serviceContext(), `INSERT INTO recording_ts_analysis(schedule_id,file_name,service_id,status,queued_at) 
SELECT id,filename,sid,'queued',? FROM schedule WHERE id=? AND watchonly=0 AND filename<>''
ON CONFLICT(schedule_id) DO UPDATE SET resume_position_ms=CASE WHEN recording_ts_analysis.file_name=excluded.file_name THEN 
recording_ts_analysis.resume_position_ms ELSE 0 END,manual_bookmarks_json=CASE WHEN 
recording_ts_analysis.file_name=excluded.file_name THEN recording_ts_analysis.manual_bookmarks_json ELSE '[]' END,
file_deleted=CASE WHEN recording_ts_analysis.file_name=excluded.file_name THEN recording_ts_analysis.file_deleted ELSE 0 END,
file_name=excluded.file_name,service_id=excluded.service_id,status='queued',queued_at=excluded.queued_at,error=''`, time.Now().UnixMilli(), id)
	if err != nil {
		log.Printf("error: 録画後TS解析の登録 id:%d: %v", id, err)
		return
	}

	select {
	case tsAnalysisWake <- struct{}{}:
	default:
	}
}

func recordingTSStillActive(id uint64) bool {
	scheduler.RLock()
	defer scheduler.RUnlock()
	for _, schedule := range scheduler.CurrentRecording {
		if schedule.ID == id {
			return true
		}
	}
	return false
}

func waitForStableRecording(ctx context.Context, path string) error {
	var previous tsFileIdentity
	stable := 0
	for attempt := 0; attempt < 30; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err == nil {
			identity, e := identifyTSFile(file)
			file.Close()
			if e == nil && identity.size > 0 {
				if identity == previous {
					stable++
				} else {
					stable = 0
				}
				previous = identity
				if stable >= 2 {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("録画ファイルの書き込み完了を確認できませんでした")
}

func runTSAnalysisJob(ctx context.Context, slotNo int, schedule *RecordingSchedule) error {
	tsAnalysisRunning.Add(1)
	defer tsAnalysisRunning.Add(-1)
	if recordingTSStillActive(schedule.ID) {
		return nil
	}

	db := rockDB.GetData(slotNo)
	if _, err := db.ExecContext(ctx, `UPDATE recording_ts_analysis SET status='running',started_at=?,error='' WHERE schedule_id=?`,
		time.Now().UnixMilli(), schedule.ID); err != nil {
		return err
	}

	err := waitForStableRecording(ctx, schedule.FileName)
	if err == nil && !recordingTSStillActive(schedule.ID) {
		file, e := os.Open(schedule.FileName)
		if e == nil {
			_, err = analyzeRecordedTS(ctx, file, schedule, slotNo, false)
			file.Close()
		} else {
			err = e
		}
	}
	if err != nil && ctx.Err() == nil {
		_, _ = db.ExecContext(ctx, `UPDATE recording_ts_analysis SET status='error',finished_at=?,error=? WHERE schedule_id=?`,
			time.Now().UnixMilli(), err.Error(), schedule.ID)
	}
	return err
}

func runTSAnalysisWorker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return
		}

		if CurrentRecorderSetting().AnalyzeTSAfterRecording {
			for slotNo := range rockDB.Data {
				for ctx.Err() == nil && CurrentRecorderSetting().AnalyzeTSAfterRecording {
					var id uint64
					err := rockDB.GetData(slotNo).QueryRowContext(ctx, `SELECT a.schedule_id FROM recording_ts_analysis a JOIN schedule s ON s.id=a.schedule_id WHERE 
					a.status IN ('queued','running') ORDER BY a.queued_at LIMIT 1`).Scan(&id)
					if err != nil {
						break
					}
					if recordingTSStillActive(id) {
						break
					}
					schedule, err := scheduler.GetRecordingSchedule(slotNo, id)
					if err != nil {
						break
					}
					if err = runTSAnalysisJob(ctx, slotNo, schedule); err != nil {
						if ctx.Err() == nil {
							log.Printf("error: 録画後TS解析 id:%d: %v", id, err)
						}
						break
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-tsAnalysisWake:
		case <-ticker.C:
		}
	}
}
