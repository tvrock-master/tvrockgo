package tvrock

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type KeywordReserve struct {
	ID       uint64 `json:"id"`
	Validate bool   `json:"validate"`

	DeviceID  string `json:"deviceid"`   // デバイスID
	StartTime int64  `json:"starttime"`  // 検索開始時刻
	EndTime   int64  `json:"endtime"`    // 検索期間
	StartHour int64  `json:"starthours"` // 開始時間
	Hours     int64  `json:"hours"`      // 検索時間

	IdleTime  int64 `json:"idletime"`  // 待機時間（秒）
	ReadyTime int64 `json:"readytime"` // 録画開始時間（秒）
	TailTime  int64 `json:"tailtime"`  // 録画終了時間（秒）

	Title       string `json:"title"`
	Content     string `json:"content"`
	Days        int64  `json:"days"`
	Sid         []uint `json:"sid"`
	Category    uint   `json:"category"`
	Week        uint8  `json:"week"` // 0は全曜日、bit 0は日曜〜bit 6は土曜
	AndSearch   bool   `json:"andsearch"`
	FuzzySearch bool   `json:"fuzzysearch"`
	RecordOnly  bool   `json:"recordonly"` // 録画のみ
	WatchOnly   bool   `json:"watchonly"`  // 視聴のみ
	ShutDown    bool   `json:"shutdown"`   // 録画終了後アプリ終了
	EpgFollow   bool   `json:"epgfollow"`  // 時間追従

	CustomComID string `json:"customcomid"`

	HashTag string `json:"hashtag"`

	CreatedAt int64 `json:"createdat"`
	UpdatedAt int64 `json:"updatedat"`
}

func NewKeywordReserve() *KeywordReserve {
	now := Now().Unix()
	return &KeywordReserve{
		Validate:    true,
		DeviceID:    "",
		StartTime:   0,
		EndTime:     0,
		StartHour:   0,
		Hours:       24,
		IdleTime:    60,
		ReadyTime:   30,
		TailTime:    0,
		Title:       "",
		Content:     "",
		Days:        24 * 10,
		Sid:         []uint{},
		Category:    0xffff,
		AndSearch:   false,
		FuzzySearch: true,
		RecordOnly:  true,
		WatchOnly:   false,
		ShutDown:    true,
		EpgFollow:   true,
		CustomComID: "",
		HashTag:     "",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func (p *RockDB) InsertKeywordReserve(no int, kr *KeywordReserve) error {
	now := Now().Unix()
	kr.CreatedAt = now
	kr.UpdatedAt = now
	if kr.Days < 1 {
		kr.Days = 24 * 10
	}
	if kr.Category == 0 {
		kr.Category = 0xffff
	}

	tx, err := p.GetData(no).Begin()
	if err != nil {
		return err
	}

	res, err := tx.Exec(
		`INSERT INTO keywordreserve (validate,`+
			`deviceid,starttime,endtime,starthours,hours,`+
			`idletime,readytime,tailtime,`+
			`title,content,days,sid,category,week,andsearch,fuzzysearch,`+
			`recordonly,watchonly,shutdown,epgfollow,customcomid,hashtag,`+
			`createdat,updatedat) VALUES (?, ?,?,?,?,?, ?,?,?, ?,?,?,?,?,?,?,?,?,?,?,?,?,?, ?,?)`,
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
		tx.Rollback()
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		tx.Rollback()
		return err
	}
	kr.ID = uint64(id)

	return tx.Commit()
}

func (p *RockDB) UpdateKeywordReserve(no int, kr *KeywordReserve) error {
	kr.UpdatedAt = Now().Unix()
	if kr.Days < 1 {
		kr.Days = 24 * 10
	}
	if kr.Category == 0 {
		kr.Category = 0xffff
	}

	tx, err := p.GetData(no).Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`UPDATE keywordreserve SET validate=?,`+
			`deviceid=?,starttime=?,endtime=?,starthours=?,hours=?,`+
			`idletime=?,readytime=?,tailtime=?,`+
			`title=?,content=?,days=?,sid=?,category=?,week=?,andsearch=?,fuzzysearch=?,`+
			`recordonly=?,watchonly=?,shutdown=?,epgfollow=?,customcomid=?,hashtag=?,`+
			`updatedat=? WHERE id=?`,
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
		kr.UpdatedAt,
		kr.ID,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (p *RockDB) UpdateKeywordReserveDate(no int, kr *KeywordReserve) error {
	kr.UpdatedAt = Now().Unix()

	tx, err := p.GetData(no).Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		`UPDATE keywordreserve SET updatedat=? WHERE id=?`,
		kr.UpdatedAt,
		kr.ID,
	)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

func (p *RockDB) DeleteKeywordReserve(no int, id uint64) error {
	_, err := p.GetData(no).Exec(`DELETE FROM keywordreserve WHERE id=?`, id)
	return err
}

func (p *RockDB) GetKeywordReserves(no int) ([]*KeywordReserve, error) {
	rows, err := p.GetData(no).Query(`SELECT id,validate,` +
		`deviceid,starttime,endtime,starthours,hours,` +
		`idletime,readytime,tailtime,` +
		`title,content,days,sid,category,week,andsearch,fuzzysearch,` +
		`recordonly,watchonly,shutdown,epgfollow,customcomid,hashtag,` +
		`createdat,updatedat FROM keywordreserve ORDER BY updatedat DESC, id DESC`,
	)
	if err == sql.ErrNoRows {
		return []*KeywordReserve{}, nil
	} else if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []*KeywordReserve{}
	for rows.Next() {
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
			return nil, err
		}
		kr.Sid = splitUintString(sid)
		if kr.Category == 0 {
			kr.Category = 0xffff
		}
		list = append(list, &kr)
	}

	return list, rows.Err()
}

func (p *RockDB) GetKeywordReserve(no int, id uint64) (*KeywordReserve, error) {
	rows, err := p.GetData(no).Query(`SELECT id,validate,`+
		`deviceid,starttime,endtime,starthours,hours,`+
		`idletime,readytime,tailtime,`+
		`title,content,days,sid,category,week,andsearch,fuzzysearch,`+
		`recordonly,watchonly,shutdown,epgfollow,customcomid,hashtag,`+
		`createdat,updatedat FROM keywordreserve WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, sql.ErrNoRows
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
		return nil, err
	}
	kr.Sid = splitUintString(sid)
	if kr.Category == 0 {
		kr.Category = 0xffff
	}

	return &kr, nil
}

func splitUintString(str string) []uint {
	ret := []uint{}
	if str == "" {
		return ret
	}
	for _, v := range strings.Split(str, ",") {
		n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 64)
		if err != nil {
			continue
		}
		ret = append(ret, uint(n))
	}
	return ret
}

func (p *Scheduler) AddKeywordReserveSchedules(kr *KeywordReserve) (int, error) {
	p.muKeywordReserve.Lock()
	defer p.muKeywordReserve.Unlock()

	count, err := p.addKeywordReserveSchedulesNoNotify(kr)
	if err != nil || count == 0 {
		return count, err
	}

	err = rockDB.UpdateKeywordReserve(CurrentSlot, kr)
	if err != nil {
		return count, err
	}

	p.UpdateScheduler()

	return count, nil
}

func (p *Scheduler) addKeywordReserveSchedulesNoNotify(kr *KeywordReserve) (int, error) {
	if kr == nil {
		return 0, fmt.Errorf("keyword reserve is nil")
	}
	if kr.ID < 1 {
		return 0, fmt.Errorf("keyword reserve id is empty")
	}
	if !kr.Validate {
		return 0, nil
	}
	if len(kr.Title) < 1 && len(kr.Content) < 1 {
		return 0, fmt.Errorf("keyword reserve keyword is empty")
	}

	title, excludedTitle, err := compileSearchPattern(kr.Title, kr.FuzzySearch)
	if err != nil {
		return 0, err
	}
	content, excludedContent, err := compileSearchPattern(kr.Content, kr.FuzzySearch)
	if err != nil {
		return 0, err
	}
	exclusions := searchExclusions{title: excludedTitle, content: excludedContent}

	now := Now()
	days := kr.Days
	if days < 1 {
		days = 24 * 10
	}
	end := now.Add(time.Duration(days) * time.Hour)

	current, err := p.GetSchedules(CurrentSlot, now.Add(-1*time.Hour), end, 0)
	if err != nil {
		return 0, err
	}
	registered := map[string]*RecordingSchedule{}
	for _, s := range current {
		if !s.Complete {
			registered[fmt.Sprintf("%d:%d", s.Sid, s.Eid)] = s
		}
	}

	sid := kr.Sid
	if len(sid) < 1 {
		sid = station.GetValidServiceID()
	}
	insert := []*RecordingSchedule{}
	update := []*RecordingSchedule{}
	count := 0
	for st := now; st.Before(end); st = st.Add(24 * time.Hour) {
		if err := serviceContext().Err(); err != nil {
			return 0, err
		}
		ed := st.Add(24 * time.Hour)
		if ed.After(end) {
			ed = end
		}

		err := p.tvp.GetFromSID(CurrentSlot, sid, st, ed, 0, func(prg *TvProgram) bool {
			if serviceContext().Err() != nil {
				return false
			}
			if title == nil && content == nil {
				return true
			}
			if !matchProgramCategory(kr.Category, prg.CnibA) {
				return true
			}

			if !matchProgramWeek(kr.Week, prg) || !matchProgramHours(kr.StartHour, kr.Hours, prg) {
				return true
			}

			if !exclusions.matchesProgram(prg, kr.FuzzySearch) && matchProgramText(kr.AndSearch, kr.FuzzySearch, title, content, prg) {

				// 既にスケジュールがあればスケジュールを修正
				key := fmt.Sprintf("%d:%d", prg.Sid, prg.Eid)
				rs, b := registered[key]
				if !b {
					rs = p.New()
					rs.SearchID = uint(kr.ID)
				}

				// キーワード予約でなければ編集しない
				if rs.SearchID != uint(kr.ID) {
					return true
				}

				rs.Title = prg.Title
				rs.Sid = prg.Sid
				rs.Eid = prg.Eid
				rs.StartTime = prg.Start
				rs.EndTime = prg.End

				rs.AutoSelect = len(kr.DeviceID) < 1
				// 自動選択の既存予約は割り当てを消さず、再計算側で必要な場合だけ変更する
				if !b || kr.DeviceID != "" {
					rs.DeviceID = kr.DeviceID
				}
				rs.CustomComID = kr.CustomComID
				rs.IdleTime = kr.IdleTime
				rs.ReadyTime = kr.ReadyTime
				rs.TailTime = kr.TailTime
				rs.WatchOnly = kr.WatchOnly
				rs.RecordOnly = kr.RecordOnly
				rs.ShutDown = kr.ShutDown
				rs.EpgFollow = kr.EpgFollow
				if b {
					update = append(update, rs)
				} else {
					insert = append(insert, rs)
					registered[key] = rs
				}
				count = count + 1
			}
			return true
		})
		if err != nil {
			return 0, err
		}
	}

	if err := serviceContext().Err(); err != nil {
		return 0, err
	}

	if len(insert) > 0 {
		_, err := p.Insert(CurrentSlot, insert...)
		if err != nil {
			return 0, err
		}
	}

	if len(update) > 0 {
		err := p.UpdateSchedules(CurrentSlot, update...)
		if err != nil {
			return 0, err
		}
	}

	return count, nil
}
