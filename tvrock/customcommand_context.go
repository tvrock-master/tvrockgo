package tvrock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// 表示用の値を保持する。番組名などをコマンド文字列に展開しない。
type CustomCommandClock struct {
	Year           string `json:"year"`
	YearShort      string `json:"yearShort"`
	Month          string `json:"month"`
	Day            string `json:"day"`
	Hour           string `json:"hour"`
	Minute         string `json:"minute"`
	Second         string `json:"second"`
	Weekday        string `json:"weekday"`
	WeekdayEnglish string `json:"weekdayEnglish"`
}

type CustomCommandContext struct {
	Title           string             `json:"title"`
	Subtitle        string             `json:"subtitle"`
	StationName     string             `json:"stationName"`
	DurationMinutes int64              `json:"durationMinutes"`
	KeywordHashtags string             `json:"keywordHashtags"`
	StationHashtags string             `json:"stationHashtags"`
	ErrorCount      uint               `json:"errorCount"`
	RecordDirectory string             `json:"recordDirectory"`
	RecordFile      string             `json:"recordFile"`
	RecordFileName  string             `json:"recordFileName"`
	Current         CustomCommandClock `json:"current"`
	Current28       CustomCommandClock `json:"current28"`
	Start           CustomCommandClock `json:"start"`
	Start28         CustomCommandClock `json:"start28"`
	End             CustomCommandClock `json:"end"`
	End28           CustomCommandClock `json:"end28"`
	Date            string             `json:"date"`
	StartTime       string             `json:"startTime"`
	StartSeconds    string             `json:"startSeconds"`
	EndTime         string             `json:"endTime"`
}

func customCommandClock(tm time.Time, extended bool) CustomCommandClock {
	hour := tm.Format("15")
	if extended && tm.Hour() < 4 {
		hour = strconv.Itoa(tm.Hour() + 24)
		tm = tm.AddDate(0, 0, -1)
	}
	return CustomCommandClock{
		Year: strconv.Itoa(tm.Year()), YearShort: strconv.Itoa(tm.Year() % 100),
		Month: strconv.Itoa(int(tm.Month())), Day: strconv.Itoa(tm.Day()),
		Hour: hour, Minute: tm.Format("04"), Second: tm.Format("05"),
		Weekday: weekName[int(tm.Weekday())], WeekdayEnglish: tm.Weekday().String(),
	}
}

func buildCustomCommandContext(title, recpath string, sid, eid, errCount uint, hashtagk, hashtags string) (*CustomCommandContext, error) {
	data := &CustomCommandContext{Title: title, RecordFile: recpath, ErrorCount: errCount}
	now := Now()
	data.Current = customCommandClock(now, false)
	data.Current28 = customCommandClock(now, true)
	data.Date = now.Format("2006/01/02")
	if recpath != "" {
		data.RecordDirectory, data.RecordFileName = filepath.Split(recpath)
	}
	if hashtagk != "" {
		data.KeywordHashtags = buildHashTag(hashtagk)
	}
	if hashtags != "" {
		data.StationHashtags = buildHashTag(hashtags)
	}
	if sta, ok := station.Get(sid); ok {
		data.StationName = sta.Name
	}
	if sid == 0 {
		return data, nil
	}
	programs, err := tvprogram.GetWithEID(CurrentSlot, sid, eid)
	if err != nil {
		return nil, err
	}
	if len(programs) == 0 {
		return data, nil
	}
	prg := programs[0]
	if data.Title == "" {
		data.Title = prg.Title
	}
	data.Subtitle = prg.Subtitle
	data.DurationMinutes = (prg.End - prg.Start) / 60
	start, end := time.Unix(prg.Start, 0), time.Unix(prg.End, 0)
	data.Start, data.Start28 = customCommandClock(start, false), customCommandClock(start, true)
	data.End, data.End28 = customCommandClock(end, false), customCommandClock(end, true)
	data.StartTime, data.StartSeconds, data.EndTime = start.Format("15:04"), start.Format("05"), end.Format("15:04")
	return data, nil
}

func writeCustomCommandContext(path string, data *CustomCommandContext) error {
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0600)
}
