package tvrock

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	templateError403 = "page_403.html"
	templateError404 = "page_404.html"
	templateError500 = "page_500.html"
)

type KeywordReserveScheduleResult struct {
	Count int `json:"count"`
}

type FetchProgramScheduleSetting struct {
	Hours []int `json:"hours"`
}

type SystemLogResult struct {
	Date      string `json:"date"`
	FileName  string `json:"filename"`
	Level     string `json:"level"`
	Content   string `json:"content"`
	ModTime   int64  `json:"modtime"`
	Size      int64  `json:"size"`
	Unchanged bool   `json:"unchanged"`
}

type ConfigureResult struct {
	CustomCommand                  []CustomCommandSummary `json:"CustomCommand"`
	PowerManagerSetting            PowerManagerSetting    `yaml:"powermanagersetting" json:"PowerManagerSetting"`
	CurrentKeywordReserveThreshold int64                  `json:"CurrentKeywordReserveThreshold"`
}

type CustomCommandSummary struct {
	ID           string
	Mode         CustomCommandExecutionMode
	Dependencies string
}

type PowerManagerSettingUpdate struct {
	SleepTimerPause        *bool  `json:"sleeptimerpause"`
	PreventSleep           *bool  `json:"preventsleep"`
	PreventDisplayOff      *bool  `json:"preventdisplayoff"`
	PreventIfDeviceRunning *bool  `json:"preventifdevicerunning"`
	WakeBeforeSchedule     *int64 `json:"wakebeforeschedule"`
	WakeUpSuppressDuration *int64 `json:"wakeupsuppressduration"`
}

func addResponseHeader(c *gin.Context) {
	for k, v := range apiHeaders {
		c.Header(k, v)
	}
}

func bindJSON(c *gin.Context, data interface{}) bool {
	if err := c.ShouldBindJSON(data); err != nil {
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		log.Println("error:", err)
		return false
	}
	return true
}

func APIDebug(c *gin.Context) {
	if !configure.Debug {
		c.Status(http.StatusNotFound)
		return
	}
	str := ""
	str = str + "-------------------------------------------\n"
	// 診断情報の対象を、個人情報を含まない設定に限定する。設定、ランチャー
	// コマンド、録画タイトル、デバイスパスには、プライベートなデータが含まれる可能性があるため。
	str = str + JsonPrint(CurrentRecorderSetting()) + "\n"
	str = str + "-------------------------------------------\n"
	str = str + JsonPrint(rockDB) + "\n"
	str = str + "-------------------------------------------\n"
	launcher.Lock()
	str += fmt.Sprintf("launchers=%d\n", len(launcher.Pool))
	launcher.Unlock()
	str = str + "-------------------------------------------\n"
	tasks.Lock()
	str += fmt.Sprintf("tasks=%d\n", len(tasks.Job))
	tasks.Unlock()

	str = str + "-------------------------------------------\n"
	slot.RLock()
	str += fmt.Sprintf("devices=%d\n", len(slot.Device))
	slot.RUnlock()
	deviceController.RLock()
	queues := make([]*DevConQueue, 0, len(deviceController.Handler))
	for _, queue := range deviceController.Handler {
		queues = append(queues, queue)
	}
	deviceController.RUnlock()
	for i, queue := range queues {
		str += fmt.Sprintf("controller[%d]=%+v\n", i, queue.stats())
	}
	c.String(http.StatusOK, str)
}

// APIGetStationList サービス一覧
func APIGetStationList(c *gin.Context) {
	list := station.GetStationList()

	addResponseHeader(c)
	c.JSON(http.StatusOK, list)
}

// APIGetStationList サービス一覧
func APIUpdateStationList(c *gin.Context) {
	var data TvStation
	if !bindJSON(c, &data) {
		return
	}

	station.ModifyStation(&data)
	station.UpdateStation()

	// 通知
	deviceController.Notify(ControllerUpdateStation, "")

	list := station.GetStationList()

	addResponseHeader(c)
	c.JSON(http.StatusOK, list)
}

func APIGetTvProgram(c *gin.Context) {
	var data TvpParam
	if !bindJSON(c, &data) {
		return
	}

	ln := len(data.Services)
	if ln < 1 || ln > configure.FrontendStationLimit || data.Hours < 1 || data.Hours > 24 {
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		log.Println("error: 局情報が無いか多すぎるため取得に失敗", ln, configure.FrontendStationLimit)
		return
	}
	t1 := time.Unix(data.Start, 0)
	t2 := time.Unix(data.Start+int64(data.Hours*60*60), 0)

	dur := t2.Sub(t1)
	if dur.Minutes() > (24 * 60) {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("errpr: lot of times.")
		return
	}

	fn := GetTvGuide
	if data.Week {
		fn = GetTvGuideWeek
	}
	list, _ := fn(data.Services, t1, t2, 0)

	addResponseHeader(c)
	c.JSON(http.StatusOK, list)
}

func APIGetTvFeature(c *gin.Context) {
	list, _ := GetTvFeature()
	addResponseHeader(c)
	c.JSON(http.StatusOK, list)
}

func APIGetSchedule(c *gin.Context) {
	rsch, err := scheduler.GetScheduleAll()
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, rsch)
}

func APIAddSchedule(c *gin.Context) {
	var data ReserveParam
	if !bindJSON(c, &data) {
		return
	}

	tvp, err := tvprogram.Get(CurrentSlot, data.Sid, data.Eid)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	p := programForInterval(tvp, data.Start, data.End)
	if p == nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	rs := scheduler.New()
	rs.Title = p.Title
	rs.StartTime = p.Start
	rs.EndTime = p.End
	rs.Sid = p.Sid
	rs.Eid = p.Eid
	if data.Watch {
		rs.WatchOnly = true
		rs.RecordOnly = false
	}

	rsch := []RecordingSchedule{}

	// 同じ番組のスケジュールがあるか調べWatchOnlyであればそのスケジュールを録画状態に変更する
	b, err := scheduler.IfWatchingToRecording(rs.Sid, rs.Eid)
	if err != nil {
		log.Println("error:", err)
	} else if b {
		addResponseHeader(c)
		c.JSON(http.StatusOK, rsch)
		return
	}

	scheduler.Add(rs)

	addResponseHeader(c)
	c.JSON(http.StatusOK, rsch)
}

func APIUpdateSchedule(c *gin.Context) {
	var data RecordingSchedule
	if !bindJSON(c, &data) {
		return
	}

	if data.ID < 1 {
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		log.Println("error: 不正なスケジュールID")
		return
	}

	current, err := scheduler.GetRecordingSchedule(CurrentSlot, data.ID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}
	if current == nil {
		c.HTML(http.StatusNotFound, templateError404, gin.H{})
		log.Println("error: スケジュールが見つかりません", data.ID)
		return
	}

	if current.SearchID != 0 {
		current.Validate = data.Validate
	} else {
		current.DeviceID = data.DeviceID
		current.Title = html.UnescapeString(data.Title)
		current.CustomComID = data.CustomComID
		//	current.FileName = data.FileName
		current.Validate = data.Validate
		current.WatchOnly = data.WatchOnly
		current.RecordOnly = data.RecordOnly
		current.ShutDown = data.ShutDown
		current.EpgFollow = data.EpgFollow
		//		current.AutoSelect = data.AutoSelect
		current.AutoSelect = len(data.DeviceID) < 1
		current.IdleTime = data.IdleTime
		current.ReadyTime = data.ReadyTime
		current.TailTime = data.TailTime
		current.Priority = data.Priority
		current.ExitMode = data.ExitMode
	}

	current.LastUpdate = Now().Unix()

	scheduler.Modify(current)

	rsch := []RecordingSchedule{}

	addResponseHeader(c)
	c.JSON(http.StatusOK, rsch)
}

func APIDeleteSchedule(c *gin.Context) {
	_id := c.Param("id")
	if len(_id) < 1 {
		c.HTML(http.StatusInternalServerError, "", gin.H{})
		return
	}

	id, err := strconv.ParseUint(_id, 10, 64)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	scheduler.Remove(id)

	rsch := []RecordingSchedule{}

	addResponseHeader(c)
	c.JSON(http.StatusOK, rsch)
}

func APIStopSchedule(c *gin.Context) {

	_id := c.Param("id")
	if len(_id) < 1 {
		c.HTML(http.StatusInternalServerError, "", gin.H{})
		return
	}

	id, err := strconv.ParseUint(_id, 10, 64)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	scheduler.Stop(id)

	rsch := []RecordingSchedule{}

	addResponseHeader(c)
	c.JSON(http.StatusOK, rsch)
}

func APIGetRecordingStatus(c *gin.Context) {
	rs, err := scheduler.GetRrecordingStatus()
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, rs)
}

func APIDoSearch(c *gin.Context) {
	var data SearchParam
	if !bindJSON(c, &data) {
		return
	}

	// 検索オプション
	opt := NewSearch()
	opt.Title = html.UnescapeString(data.Title)
	opt.Content = html.UnescapeString(data.Content)
	opt.StartTime = data.Start
	opt.EndTime = data.Days * 60 * 60
	if len(data.Sid) > 0 {
		opt.Sid = data.Sid
	} else {
		opt.Sid = searchServiceIDs(nil)
	}

	opt.Category = data.Category
	opt.Week = data.Week
	opt.AndSearch = data.AndSearch
	if data.FuzzySearch != nil {
		opt.FuzzySearch = *data.FuzzySearch
	}
	opt.ArchiveSearch = data.ArchiveSearch
	opt.CaptionSearch = data.CaptionSearch
	if opt.StartTime < 1 {
		opt.StartTime = Now().Unix()
	}
	if opt.EndTime >= (30 * 365 * 24 * 60 * 60) {
		opt.EndTime = (30 * 365 * 24 * 60 * 60)
	}
	if opt.ArchiveSearch {
		first := int64(0)
		for _, start := range tvprogram.FirstStartTime {
			if start > 0 && (first == 0 || start < first) {
				first = start
			}
		}
		start, end, err := archiveSearchPeriod(data.StartTime, data.EndTime, first, Now())
		if err != nil {
			addResponseHeader(c)
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		opt.StartTime = start
		opt.EndTime = end - start
	}
	opt.StartHours = data.StartHours
	opt.Hours = data.Hours

	// 既存のタスクを先にキャンセルして検索ループを早めに終了させる
	cancelTask := SearchTask{UID: data.UID}
	cancelTask.Cancel()

	notifySearch := func(command string, value interface{}) {
		if data.RequestID != "" {
			value = struct {
				RequestID string      `json:"requestId"`
				Data      interface{} `json:"data"`
			}{data.RequestID, value}
		} else if command == ControllerResetSearch {
			deviceController.NotifyTo(data.UID, command, "")
			return
		}
		js, err := json.Marshal(value)
		if err != nil {
			log.Println("error:", err)
			return
		}
		deviceController.NotifyTo(data.UID, command, string(js))
	}
	notifySearch(ControllerResetSearch, nil)

	// 検索結果と進捗状況は、WebSocket経由で配信されます。
	err := opt.Search(data.UID, func(p []*TvProgram) {
		for start := 0; start < len(p); start += 200 {
			end := min(start+200, len(p))
			notifySearch(ControllerSendSearch, p[start:end])
		}
	}, func(p *SearchTask) {
		r := SearchResult{
			Hits: len(p.result),
		}
		notifySearch(ControllerSendSearch, r)
	}, func(p *SearchProgress) {
		notifySearch(ControllerSendSearch, p)
	})
	if err != nil {
		notifySearch(ControllerSendSearch, &SearchProgress{Progress: true, Running: false})
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	ret := []*TvProgram{}
	addResponseHeader(c)
	c.JSON(http.StatusOK, ret)
}

// 日付のみの終了値は、選択された日全体を含みます。タイムスタンプの終了値は
// 排他的（その値を含まない）であり、プログラムの重複判定クエリ（start < end && end > start）と整合します。
func archiveSearchPeriod(startText, endText string, first int64, now time.Time) (int64, int64, error) {
	parse := func(value string, end bool) (time.Time, error) {
		value = strings.TrimSpace(value)
		for _, layout := range []string{"2006-01-02", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02 15:04:05", time.RFC3339} {
			if tm, err := time.ParseInLocation(layout, value, now.Location()); err == nil {
				if end && layout == "2006-01-02" {
					tm = tm.AddDate(0, 0, 1)
				}
				return tm, nil
			}
		}
		return time.Time{}, fmt.Errorf("検索日付が不正です: %s", value)
	}
	start, end := first, now.Unix()
	if strings.TrimSpace(startText) != "" {
		tm, err := parse(startText, false)
		if err != nil {
			return 0, 0, err
		}
		start = tm.Unix()
	}
	if strings.TrimSpace(endText) != "" {
		tm, err := parse(endText, true)
		if err != nil {
			return 0, 0, err
		}
		end = tm.Unix()
	}
	if end <= start {
		if strings.TrimSpace(startText) == "" {
			// 別のカーソルが、まだキャプションが付いていない古いドキュメントをインデックス化します。
			return end, end, nil
		}
		return 0, 0, fmt.Errorf("検索終了日時は検索開始日時より後にしてください")
	}
	return start, end, nil
}

func APIGetKeywordReserve(c *gin.Context) {
	_id := c.Param("id")
	if len(_id) < 1 {
		c.HTML(http.StatusInternalServerError, "", gin.H{})
		return
	}
	id, err := strconv.ParseUint(_id, 10, 64)

	if id == 0 {
		list, err := rockDB.GetKeywordReserves(0)
		if err != nil {
			c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
			log.Println("error:", err)
			return
		}

		addResponseHeader(c)
		c.JSON(http.StatusOK, list)
		return
	}

	kwr, err := rockDB.GetKeywordReserve(CurrentSlot, id)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, kwr)
}

func APIAddKeywordReserve(c *gin.Context) {
	data := NewKeywordReserve()
	if !bindJSON(c, data) {
		return
	}

	if len(data.Title) < 1 && len(data.Content) < 1 {
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		log.Println("error: キーワードの内容が存在しない")
		return
	}

	data.Title = html.UnescapeString(data.Title)
	data.Content = html.UnescapeString(data.Content)

	if err := rockDB.InsertKeywordReserve(CurrentSlot, data); err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	deviceController.Notify(ControllerUpdateKeywordReserve, "")

	addResponseHeader(c)
	c.JSON(http.StatusOK, data)
}

func APIUpdateKeywordReserve(c *gin.Context) {
	data := NewKeywordReserve()
	if !bindJSON(c, &data) {
		return
	}

	if data.ID < 1 || (len(data.Title) < 1 && len(data.Content) < 1) {
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		log.Println("error: キーワードの内容が無いため更新に失敗", data)
		return
	}

	data.Title = html.UnescapeString(data.Title)
	data.Content = html.UnescapeString(data.Content)

	if err := rockDB.UpdateKeywordReserve(CurrentSlot, data); err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	kr, err := rockDB.GetKeywordReserve(CurrentSlot, data.ID)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	_, err = scheduler.AddKeywordReserveSchedules(kr)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	deviceController.Notify(ControllerUpdateKeywordReserve, "")

	addResponseHeader(c)
	c.JSON(http.StatusOK, data)
}

func APIDeleteKeywordReserve(c *gin.Context) {
	_id := c.Param("id")
	if len(_id) < 1 {
		c.HTML(http.StatusInternalServerError, "", gin.H{})
		return
	}

	id, err := strconv.ParseUint(_id, 10, 64)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	if err := rockDB.DeleteKeywordReserve(CurrentSlot, id); err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	if err := scheduler.DeleteBySearchID(CurrentSlot, uint(id)); err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	scheduler.UpdateScheduler()

	deviceController.Notify(ControllerUpdateKeywordReserve, "")

	list, err := rockDB.GetKeywordReserves(0)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, list)
}

func APIAddKeywordReserveSchedule(c *gin.Context) {
	_id := c.Param("id")
	if len(_id) < 1 {
		c.HTML(http.StatusInternalServerError, "", gin.H{})
		return
	}

	id, err := strconv.ParseUint(_id, 10, 64)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	kr, err := rockDB.GetKeywordReserve(CurrentSlot, id)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	count, err := scheduler.AddKeywordReserveSchedules(kr)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, KeywordReserveScheduleResult{Count: count})
}

func APIGetSessionValue(c *gin.Context) {
	ss := sessions.Default(c)
	us, ok := ss.Get(UserData).(*UserSession)
	if !ok || us == nil || us.Data == nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, us.Data)
}

func APIUpdateSessionValue(c *gin.Context) {
	ss := sessions.Default(c)

	var data MySessionData
	if !bindJSON(c, &data) {
		return
	}

	us, ok := ss.Get(UserData).(*UserSession)
	if !ok || us == nil {
		c.Status(http.StatusUnauthorized)
		return
	}
	updated := *us
	updated.Data = &data
	ss.Set(UserData, &updated)
	if err := ss.Save(); err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, data)
}

func APIGetDeviceAll(c *gin.Context) {

	slot.RLock()

	list := []*RockDevice{}
	for _, val := range slot.Device {
		copy := *val
		list = append(list, &copy)
	}
	slot.RUnlock()
	for i, dev := range list {
		resolved, err := tvprogram.resolveDevicePrograms(dev)
		if err != nil {
			log.Printf("error: device common event: %v", err)
			continue
		}
		list[i] = resolved
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })

	addResponseHeader(c)
	c.JSON(http.StatusOK, list)
}

func APIGetRecorderSetting(c *gin.Context) {
	addResponseHeader(c)
	c.JSON(http.StatusOK, CurrentRecorderSetting())
}

func APIUpdateRecorderSetting(c *gin.Context) {
	var data RecorderSetting
	if !bindJSON(c, &data) {
		return
	}

	setting, err := updateRecorderSetting(data)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, setting)
}

func APIGetPowerManagerSetting(c *gin.Context) {
	addResponseHeader(c)
	c.JSON(http.StatusOK, CurrentPowerManagerSetting())
}

func APIUpdatePowerManagerSetting(c *gin.Context) {
	var data PowerManagerSettingUpdate
	if !bindJSON(c, &data) {
		return
	}

	setting, err := updatePowerManagerSetting(func(setting *PowerManagerSetting) {
		if data.SleepTimerPause != nil {
			setting.SleepTimerPause = *data.SleepTimerPause
		}
		if data.PreventSleep != nil {
			setting.PreventSleep = *data.PreventSleep
		}
		if data.PreventDisplayOff != nil {
			setting.PreventDisplayOff = *data.PreventDisplayOff
		}
		if data.PreventIfDeviceRunning != nil {
			setting.PreventIfDeviceRunning = *data.PreventIfDeviceRunning
		}
		if data.WakeBeforeSchedule != nil {
			setting.WakeBeforeSchedule = normalizeNonNegativeInt64(*data.WakeBeforeSchedule)
		}
		if data.WakeUpSuppressDuration != nil {
			setting.WakeUpSuppressDuration = normalizeNonNegativeInt64(*data.WakeUpSuppressDuration)
		}

	})
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, setting)
}

func normalizeNonNegativeInt64(v int64) int64 {
	if v < 0 {
		return 0
	}
	return v
}

func APIGetFetchProgramScheduleSetting(c *gin.Context) {
	addResponseHeader(c)
	c.JSON(http.StatusOK, FetchProgramScheduleSetting{
		Hours: normalizeFetchProgramScheduleHours(configure.FetchProgram.ScheduleHours),
	})
}

func APIGetSystemLog(c *gin.Context) {
	dateText := c.Param("date")
	level, ok := normalizeSystemLogLevel(c.DefaultQuery("level", "info"))
	if !ok {
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		return
	}

	date, err := time.ParseInLocation("20060102", dateText, time.Local)
	if err != nil || date.Format("20060102") != dateText {
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		return
	}

	now := Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	oldest := today.AddDate(0, 0, -7)
	if date.Before(oldest) || date.After(today) {
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		return
	}

	fileName := dateText + "0000.log"
	filePath := filepath.Join(".", "log", fileName)
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			addResponseHeader(c)
			c.JSON(http.StatusNotFound, SystemLogResult{
				Date:     dateText,
				FileName: fileName,
				Level:    level,
				Content:  "",
			})
			return
		}
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	modTime := info.ModTime().UnixNano()
	size := info.Size()
	if c.Query("modtime") == strconv.FormatInt(modTime, 10) && c.Query("size") == strconv.FormatInt(size, 10) {
		addResponseHeader(c)
		c.JSON(http.StatusOK, SystemLogResult{
			Date:      dateText,
			FileName:  fileName,
			Level:     level,
			ModTime:   modTime,
			Size:      size,
			Unchanged: true,
		})
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, SystemLogResult{
		Date:     dateText,
		FileName: fileName,
		Level:    level,
		Content:  filterSystemLogContent(string(content), level),
		ModTime:  modTime,
		Size:     size,
	})
}

func normalizeSystemLogLevel(level string) (string, bool) {
	switch strings.ToLower(level) {
	case "", "info":
		return "info", true
	case "all":
		return "all", true
	case "debug":
		return "debug", true
	case "error":
		return "error", true
	case "warn":
		return "warn", true
	default:
		return "", false
	}
}

func filterSystemLogContent(content string, level string) string {
	if level == "all" {
		return content
	}

	logLevel := level

	lines := strings.SplitAfter(content, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if matchSystemLogLevel(line, logLevel) {
			filtered = append(filtered, line)
		}
	}
	return strings.Join(filtered, "")
}

func matchSystemLogLevel(line string, level string) bool {
	if !strings.HasPrefix(line, "[") {
		return false
	}
	end := strings.Index(line, "]")
	if end < 0 {
		return false
	}
	return strings.TrimSpace(line[1:end]) == level
}

func APIUpdateFetchProgramScheduleSetting(c *gin.Context) {
	var data FetchProgramScheduleSetting
	if !bindJSON(c, &data) {
		return
	}

	configure.FetchProgram.ScheduleHours = normalizeFetchProgramScheduleHours(data.Hours)
	if err := configure.Save(TvRockConfigureFile); err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	if err := scheduler.UpdateFetchProgramSchedules(Now()); err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, FetchProgramScheduleSetting{
		Hours: configure.FetchProgram.ScheduleHours,
	})
}

func APIUpdateFetchProgramScheduleImmediately(c *gin.Context) {
	if err := scheduler.AddFetchProgramSchedulesImmediately(Now()); err != nil {
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		log.Println("error:", err)
		return
	}

	addResponseHeader(c)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func APIGetConfigure(c *gin.Context) {
	commands := make([]CustomCommandSummary, 0, len(configure.CustomCommand))
	for _, command := range configure.CustomCommand {
		commands = append(commands, CustomCommandSummary{
			ID: command.ID, Mode: command.Mode, Dependencies: command.Dependencies,
		})
	}
	ret := ConfigureResult{
		CustomCommand:                  commands,
		PowerManagerSetting:            CurrentPowerManagerSetting(),
		CurrentKeywordReserveThreshold: configure.CurrentKeywordReserveThreshold,
	}
	addResponseHeader(c)
	c.JSON(http.StatusOK, ret)
}

func APIGetCaptionHistory(c *gin.Context) {
	addResponseHeader(c)
	values := make(map[string]int64)
	for _, key := range []string{"sid", "eid", "start", "after"} {
		raw := c.Query(key)
		if key == "after" && raw == "" {
			raw = "0"
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value < 0 || (key == "start" && value == 0) || ((key == "sid" || key == "eid") && value > 4294967295) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + key})
			return
		}
		values[key] = value
	}
	const pageSize = 300
	snapshot, err := rockDB.CaptionSnapshot(c.Request.Context(), uint(values["sid"]), uint(values["eid"]), values["start"])
	if raw, present := c.GetQuery("snapshot"); present {
		var parseErr error
		snapshot, parseErr = strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || snapshot < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid snapshot"})
			return
		}
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "字幕の取得に失敗しました"})
		return
	}
	items, err := rockDB.CaptionHistory(c.Request.Context(), uint(values["sid"]), uint(values["eid"]), values["start"], values["after"], pageSize+1, snapshot)
	if err != nil {
		log.Printf("error: caption history: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "字幕の取得に失敗しました"})
		return
	}
	hasMore := len(items) > pageSize
	if hasMore {
		items = items[:pageSize]
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "hasMore": hasMore, "snapshot": snapshot})
}
