package tvrock

import (
	"context"
	"database/sql"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var JunleList = []string{
	"ニュース／報道",
	"スポーツ",
	"情報／ワイドショー",
	"ドラマ",
	"音楽",
	"バラエティー",
	"映画",
	"アニメ／特撮",
	"ドキュメンタリー／教養 ",
	"劇場／公演",
	"趣味／教育",
	"福祉",
	"事業体定義／予備１",
	"事業体定義／予備２",
	"拡張（拡張用フラグ）／予備",
	"その他",
}

type TvProgram struct {
	Sid       uint   `json:"Sid"`
	Eid       uint   `json:"Eid"`
	Start     int64  `json:"Start"`
	End       int64  `json:"End"`
	Title     string `json:"Title"`
	Subtitle  string `json:"Subtitle"`
	Content   string `json:"Content"`
	CnibA     uint   `json:"Cnib_a"`
	CnibB     uint   `json:"Cnib_b"`
	CommonSid *uint  `json:"CommonSid,omitempty"`
	CommonEid *uint  `json:"CommonEid,omitempty"`
}

type TvProgramStore struct {
	db             *RockDB
	FirstStartTime []int64
	LastStartTime  []int64
	FirstEndTime   []int64
	LastEndTime    []int64
}

var tvprogram TvProgramStore

func programIndexHint(no int) string {
	// アーカイブDBのサイズが大きいためプランナが `ORDER BY start` のために主キーを選択し、
	// 結果として過剰な数の履歴行をスキャンしてしまう可能性があります。
	if no > 0 {
		return " INDEXED BY program_9"
	}
	return ""
}

func (p *TvProgram) IsMatchProgram(t *TvProgram) bool {
	if p == nil || t == nil ||
		p.Sid != t.Sid ||
		p.Eid != t.Eid ||
		p.Title != t.Title ||
		p.Subtitle != t.Subtitle ||
		p.Content != t.Content ||
		p.Start != t.Start ||
		p.End != t.End ||
		p.CnibA != t.CnibA ||
		p.CnibB != t.CnibB ||
		!sameOptionalUint(p.CommonSid, t.CommonSid) || !sameOptionalUint(p.CommonEid, t.CommonEid) {
		return false
	}
	return true
}

func (p *TvProgramStore) Initialize(db *RockDB) error {
	var err error
	p.db = db

	p.FirstStartTime = make([]int64, 2)
	p.LastStartTime = make([]int64, 2)
	p.FirstEndTime = make([]int64, 2)
	p.LastEndTime = make([]int64, 2)

	// 仮
	p.CleanOld(0, Now().Unix()-(60*60*24*365))
	p.CleanOld(1, Now().Unix()-(60*60*24*365*25))

	// 不必要な番組の削除
	for i := range 2 {
		p.GetFirstStartTime(i)
		p.GetLastStartTime(i)
		p.GetFirstEndTime(i)
		p.GetLastEndTime(i)
	}

	if configure.Debug {
		log.Println("debug: FirstStartTime", p.FirstStartTime, time.Unix(p.FirstStartTime[0], 0), time.Unix(p.FirstStartTime[1], 0))
		log.Println("debug: LastStartTime", p.LastStartTime, time.Unix(p.LastStartTime[0], 0), time.Unix(p.LastStartTime[1], 0))
		log.Println("debug: FirstEndTime", p.FirstEndTime, time.Unix(p.FirstEndTime[0], 0), time.Unix(p.FirstEndTime[1], 0))
		log.Println("debug: LastEndTime", p.LastEndTime, time.Unix(p.LastEndTime[0], 0), time.Unix(p.LastEndTime[1], 0))
	}

	return err
}

func (p *TvProgramStore) Insert(no int, prg *TvProgram) error {
	tx, err := p.db.GetProgram(no).Begin()
	if err != nil {
		return err
	}

	// eidは繰り返し使用することができプライマリが(sid,eid)とできないので同じ番組で時間変更があれば削除
	now := Now().Unix() - configure.CurrentProgramThreshold
	_, err = tx.Exec(`DELETE FROM program WHERE sid=? AND eid=? AND end>=?`, prg.Sid, prg.Eid, now)
	if err != nil {
		tx.Rollback()
		return err
	}
	_, err = tx.Exec(
		//		`INSERT INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b,common_sid,common_eid) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		`REPLACE INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b,common_sid,common_eid) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		prg.Sid,
		prg.Eid,
		prg.Start,
		prg.End,
		prg.Title,
		prg.Subtitle,
		prg.Content,
		prg.CnibA,
		prg.CnibB,
		prg.CommonSid,
		prg.CommonEid,
	)
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (p *TvProgramStore) Replace(no int, prg *TvProgram) error {
	return p.ReplaceContext(context.Background(), no, prg)
}

// 受信EPGを小さな単位で確定し、番組ごとのディスク同期を避ける。
func (p *TvProgramStore) InsertBatch(no int, programs []TvProgram) error {
	if len(programs) == 0 {
		return nil
	}

	tx, err := p.db.GetProgram(no).Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	remove, err := tx.Prepare(`DELETE FROM program WHERE sid=? AND eid=? AND end>=?`)
	if err != nil {
		return err
	}
	defer remove.Close()

	replace, err := tx.Prepare(replaceProgramSQL)
	if err != nil {
		return err
	}
	defer replace.Close()

	threshold := Now().Unix() - configure.CurrentProgramThreshold
	for _, prg := range programs {
		if _, err := remove.Exec(prg.Sid, prg.Eid, threshold); err != nil {
			return err
		}
		if _, err := replace.Exec(prg.Sid, prg.Eid, prg.Start, prg.End, prg.Title, prg.Subtitle, prg.Content, prg.CnibA, prg.CnibB, prg.CommonSid, prg.CommonEid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const replaceProgramSQL = `REPLACE INTO program (sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b,common_sid,common_eid) VALUES (?,?,?,?,?,?,?,?,?,?,?)`

func (p *TvProgramStore) ReplaceContext(ctx context.Context, no int, prg *TvProgram) error {
	tx, err := p.db.GetProgram(no).BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, replaceProgramSQL, prg.Sid, prg.Eid, prg.Start, prg.End, prg.Title, prg.Subtitle, prg.Content, prg.CnibA, prg.CnibB, prg.CommonSid, prg.CommonEid); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *TvProgramStore) ReplaceBatchContext(ctx context.Context, no int, programs []TvProgram) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if len(programs) == 0 {
		return nil
	}

	tx, err := p.db.GetProgram(no).BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, replaceProgramSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, prg := range programs {
		if _, err := stmt.ExecContext(ctx, prg.Sid, prg.Eid, prg.Start, prg.End, prg.Title, prg.Subtitle, prg.Content, prg.CnibA, prg.CnibB, prg.CommonSid, prg.CommonEid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (p *TvProgramStore) getStoredDuringWithStation(no int, sid []uint, st time.Time, ed time.Time, limit int64) (map[uint][]*TvProgram, error) {
	ret := map[uint][]*TvProgram{}

	if len(sid) == 0 {
		return ret, nil
	}

	strs := make([]string, len(sid))
	for i, v := range sid {
		strs[i] = strconv.FormatUint(uint64(v), 10)
	}

	sids := strings.Join(strs, ",")

	lm := ""
	if limit > 0 {
		lm = " LIMIT " + strconv.FormatInt(limit, 10)
	}

	order := ""
	if no == 0 || limit > 0 {
		order = " ORDER BY start"
	}

	rows, err := p.db.GetProgram(no).Query("SELECT sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b,common_sid,common_eid FROM program"+programIndexHint(no)+" WHERE sid IN("+sids+
		") AND start<? AND end>?"+order+lm, ed.Unix(), st.Unix())

	if err == sql.ErrNoRows {
		return ret, nil
	} else if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		tvp := TvProgram{}
		err = rows.Scan(&tvp.Sid, &tvp.Eid, &tvp.Start, &tvp.End, &tvp.Title, &tvp.Subtitle, &tvp.Content, &tvp.CnibA, &tvp.CnibB, &tvp.CommonSid, &tvp.CommonEid)
		if err != nil {
			return nil, err
		}
		ret[tvp.Sid] = append(ret[tvp.Sid], &tvp)
	}

	for sid := range ret {
		sort.Slice(ret[sid], func(a, b int) bool {
			return ret[sid][a].Start < ret[sid][b].Start
		})
	}

	return ret, rows.Err()
}

func (p *TvProgramStore) GetMulti(no int, sid []uint, st time.Time, ed time.Time, limit int64) (map[uint][]*TvProgram, error) {
	return p.getProgramsResolved([]int{no}, sid, st, ed, limit)
}

func (p *TvProgramStore) GetWithEID(no int, sid uint, eid uint) ([]*TvProgram, error) {
	return p.getProgramResolved(no, sid, eid, false)
}

func (p *TvProgramStore) getStoredWithEID(no int, sid uint, eid uint) ([]*TvProgram, error) {
	rows, err := p.db.GetProgram(no).Query("SELECT sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b,common_sid,common_eid FROM program WHERE sid=? AND eid=? ORDER BY start desc", sid, eid)

	list := []*TvProgram{}
	if err == sql.ErrNoRows {
		return list, nil
	} else if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		tvp := TvProgram{}
		err = rows.Scan(&tvp.Sid, &tvp.Eid, &tvp.Start, &tvp.End, &tvp.Title, &tvp.Subtitle, &tvp.Content, &tvp.CnibA, &tvp.CnibB, &tvp.CommonSid, &tvp.CommonEid)
		if err != nil {
			return nil, err
		}
		list = append(list, &tvp)
	}

	return list, rows.Err()
}

func (p *TvProgramStore) Get(no int, sid uint, eid uint) ([]*TvProgram, error) {
	return p.getProgramResolved(no, sid, eid, true)
}

// 受信したEIT/PFの更新は、補完した親番組ではなく保存済みの子番組だけと比較する。
func (p *TvProgramStore) getStoredProgram(no int, sid uint, eid uint) ([]*TvProgram, error) {
	bef := Now().Unix() - configure.CurrentProgramThreshold
	if no > 0 {
		bef = 0
	}

	rows, err := p.db.GetProgram(no).Query("SELECT sid,eid,start,end,title,subtitle,content,cnib_a,cnib_b,common_sid,common_eid FROM program WHERE sid=? AND eid=? AND start>=? ORDER BY start desc", sid, eid, bef)

	list := []*TvProgram{}
	if err == sql.ErrNoRows {
		return list, nil
	} else if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		tvp := TvProgram{}
		err = rows.Scan(&tvp.Sid, &tvp.Eid, &tvp.Start, &tvp.End, &tvp.Title, &tvp.Subtitle, &tvp.Content, &tvp.CnibA, &tvp.CnibB, &tvp.CommonSid, &tvp.CommonEid)
		if err != nil {
			return nil, err
		}
		list = append(list, &tvp)
	}

	return list, rows.Err()
}

func (p *TvProgramStore) GetFromSID(no int, sid []uint, st time.Time, ed time.Time, limit int64, fn func(*TvProgram) bool) error {
	indexHint := programIndexHint(no)
	if len(sid) < 1 && no > 0 {
		indexHint = " INDEXED BY program_11"
	}

	sq := commonProgramSelect(indexHint) + " WHERE p.start<? AND p.end>?"

	args := make([]interface{}, 2, len(sid)+2)
	args[0] = ed.Unix()
	args[1] = st.Unix()
	if len(sid) > 0 {
		sq += " AND p.sid IN(?" + strings.Repeat(",?", len(sid)-1) + `)`
		for _, id := range sid {
			args = append(args, id)
		}
	}

	if limit > 0 {
		sq += " LIMIT " + strconv.FormatInt(limit, 10)
	}

	rows, err := p.db.GetProgram(no).Query(sq, args...)

	if err == sql.ErrNoRows {
		return nil
	} else if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		tvp := TvProgram{}
		err = rows.Scan(&tvp.Sid, &tvp.Eid, &tvp.Start, &tvp.End, &tvp.Title, &tvp.Subtitle, &tvp.Content, &tvp.CnibA, &tvp.CnibB, &tvp.CommonSid, &tvp.CommonEid)
		if err != nil {
			return err
		}
		if fn != nil && !fn(&tvp) {
			break
		}
	}

	return rows.Err()
}

func (p *TvProgramStore) GetOne(no int, src string) (int64, error) {
	rows, err := p.db.GetProgram(no).Query(src)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if !rows.Next() {
		return 0, sql.ErrNoRows
	}

	var tm sql.NullInt64
	err = rows.Scan(&tm)
	if err != nil {
		return 0, err
	}

	return tm.Int64, rows.Err()
}

func (p *TvProgramStore) GetFirstStartTime(no int) (int64, error) {
	tm, err := p.GetOne(no, "SELECT MIN(start) from program")

	p.FirstStartTime[no] = tm

	return tm, err
}

func (p *TvProgramStore) GetFirstEndTime(no int) (int64, error) {
	tm, err := p.GetOne(no, "SELECT MIN(end) from program")

	p.FirstEndTime[no] = tm

	return tm, err
}

func (p *TvProgramStore) GetLastStartTime(no int) (int64, error) {
	tm, err := p.GetOne(no, "SELECT MAX(start) from program")

	p.LastStartTime[no] = tm

	return tm, err
}

func (p *TvProgramStore) GetLastEndTime(no int) (int64, error) {
	tm, err := p.GetOne(no, "SELECT MAX(end) from program")

	p.LastEndTime[no] = tm

	return tm, err
}

func (p *TvProgramStore) CleanOld(no int, threshold int64) error {
	tx, err := p.db.GetProgram(no).Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM program WHERE start<? OR end<?`, threshold, threshold)
	if err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// 検索・索引作成のカーソルを開いたまま別のQueryを行わないよう、直接の共有先はSQLで結合する。
// 子のSID/EID/時刻と共有指定を残し、同じ時刻に存在する参照先の内容だけを使う。
func commonProgramSelect(indexHint string) string {
	return `SELECT p.sid,p.eid,p.start,p.end,
		COALESCE(c.title,p.title),COALESCE(c.subtitle,p.subtitle),COALESCE(c.content,p.content),
		COALESCE(c.cnib_a,p.cnib_a),COALESCE(c.cnib_b,p.cnib_b),p.common_sid,p.common_eid
		FROM program AS p` + indexHint + ` LEFT JOIN program AS c ON c.rowid=(
		SELECT r.rowid FROM program AS r WHERE r.sid=p.common_sid AND r.eid=p.common_eid
		AND r.start<p.end AND r.end>p.start AND r.sid<>p.sid AND (r.sid>>16)=(p.sid>>16)
		ORDER BY r.start DESC LIMIT 1)`
}

func sameOptionalUint(a, b *uint) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// DLLは放送波の16ビットSIDを通知する。共有先も子と同じ放送種別に正規化する。
func (p *TvProgram) normalizeCommonEvent() {
	if p.CommonSid == nil || p.CommonEid == nil || *p.CommonSid == 0 || *p.CommonSid > 0xffff || *p.CommonEid > 0xffff {
		p.CommonSid, p.CommonEid = nil, nil
		return
	}
	sid := p.Sid&0xffff0000 | *p.CommonSid
	if sid == p.Sid {
		p.CommonSid, p.CommonEid = nil, nil
		return
	}
	p.CommonSid = &sid
}

func guideProgramSlots(st, ed time.Time) []int {
	slots := []int{}
	if len(tvprogram.FirstStartTime) < 1 || ed.Unix() >= tvprogram.FirstStartTime[0] {
		slots = append(slots, 0)
	}
	if len(tvprogram.LastEndTime) < 2 || st.Unix() < tvprogram.LastEndTime[1] {
		slots = append(slots, 1)
	}
	return slots
}

type commonProgramResolver struct {
	store *TvProgramStore
	slot  int
	cache map[[2]uint][]*TvProgram
}

func (r *commonProgramResolver) resolve(program *TvProgram, visited map[[2]uint]bool) (*TvProgram, error) {
	if program == nil || program.CommonSid == nil || program.CommonEid == nil {
		return program, nil
	}
	key := [2]uint{program.Sid, program.Eid}
	ref := [2]uint{*program.CommonSid, *program.CommonEid}
	if visited[key] || len(visited) >= 16 || ref[0] == program.Sid || ref[0]&0xffff0000 != program.Sid&0xffff0000 {
		return program, nil
	}
	station.RLock()
	child, source := station.Cache[program.Sid], station.Cache[ref[0]]
	valid := child == nil || source == nil || (child.Tsp == source.Tsp && child.Tsid != 0 && child.Tsid == source.Tsid)
	station.RUnlock()
	if !valid || visited[ref] {
		return program, nil
	}
	visited[key] = true
	defer delete(visited, key)
	list, ok := r.cache[ref]
	if !ok {
		var err error
		list, err = r.store.getStoredWithEID(r.slot, ref[0], ref[1])
		if err != nil {
			return nil, err
		}
		r.cache[ref] = list
	}
	var selected *TvProgram
	var longest int64
	for _, candidate := range list {
		overlap := min(program.End, candidate.End) - max(program.Start, candidate.Start)
		if overlap > longest {
			selected, longest = candidate, overlap
		}
	}
	if selected == nil {
		return program, nil
	}
	resolved, err := r.resolve(selected, visited)
	if err != nil {
		return nil, err
	}
	if resolved == selected && selected.CommonSid != nil {
		return program, nil
	}
	copy := *program
	// 放送側が共有を指定した番組の内容だけを参照する。予約先のSID/EID/時刻は変更しない。
	copy.Title, copy.Subtitle, copy.Content = resolved.Title, resolved.Subtitle, resolved.Content
	copy.CnibA, copy.CnibB = resolved.CnibA, resolved.CnibB
	return &copy, nil
}

// デバイスが保持する受信PFはそのまま残し、画面へ渡すスナップショットだけを解決する。
func (p *TvProgramStore) resolveDevicePrograms(dev *RockDevice) (*RockDevice, error) {
	copy := *dev
	programs, err := p.resolveCommonPrograms(CurrentSlot, []*TvProgram{dev.CurrentProgram, dev.NextProgram})
	if err != nil {
		return nil, err
	}
	copy.CurrentProgram, copy.NextProgram = programs[0], programs[1]
	return &copy, nil
}

func (p *TvProgramStore) resolveCommonPrograms(no int, programs []*TvProgram) ([]*TvProgram, error) {
	r := commonProgramResolver{store: p, slot: no, cache: make(map[[2]uint][]*TvProgram)}
	result := make([]*TvProgram, 0, len(programs))
	for _, program := range programs {
		resolved, err := r.resolve(program, make(map[[2]uint]bool))
		if err != nil {
			return nil, err
		}
		result = append(result, resolved)
	}
	return result, nil
}

func (p *TvProgramStore) GetDuringWithStation(no int, sids []uint, st, ed time.Time, limit int64) (map[uint][]*TvProgram, error) {
	programs, err := p.getStoredDuringWithStation(no, sids, st, ed, limit)
	if err != nil {
		return nil, err
	}
	for sid, list := range programs {
		programs[sid], err = p.resolveCommonPrograms(no, list)
		if err != nil {
			return nil, err
		}
	}
	return programs, nil
}

func (p *TvProgramStore) getProgramsResolved(slots []int, sids []uint, st, ed time.Time, limit int64) (map[uint][]*TvProgram, error) {
	result := make(map[uint][]*TvProgram)
	for _, sid := range sids {
		result[sid] = []*TvProgram{}
	}
	for _, no := range slots {
		programs, err := p.GetDuringWithStation(no, sids, st, ed, limit)
		if err != nil {
			return nil, err
		}
		for sid, list := range programs {
			result[sid] = appendUniquePrograms(result[sid], list...)
		}
	}
	for sid := range result {
		sort.SliceStable(result[sid], func(i, j int) bool { return result[sid][i].Start < result[sid][j].Start })
	}
	return result, nil
}

func (p *TvProgramStore) getProgramResolved(no int, sid, eid uint, recent bool) ([]*TvProgram, error) {
	get := p.getStoredWithEID
	if recent {
		get = p.getStoredProgram
	}
	programs, err := get(no, sid, eid)
	if err != nil {
		return nil, err
	}
	return p.resolveCommonPrograms(no, programs)
}

func programForInterval(programs []*TvProgram, start, end int64) *TvProgram {
	if len(programs) == 1 {
		return programs[0]
	}
	var selected *TvProgram
	var longest int64
	for _, program := range programs {
		overlap := min(end, program.End) - max(start, program.Start)
		if overlap > longest {
			selected, longest = program, overlap
		}
	}
	return selected
}
