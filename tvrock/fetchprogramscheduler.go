package tvrock

import (
	"log"
	"time"
)

var fetchProgramCustomComIDs = []string{
	"FetchProgramTS",
	"FetchProgramBS",
	"FetchProgramCS",
	"FetchProgramOS",
}

type fetchProgramScheduleRange struct {
	Start time.Time
	End   time.Time
}

// isInFetchProgramScheduleHours は現在時刻が設定された
// フェッチ実行時間帯の範囲内にあるかどうかを返します。
// 将来の実行枠を特定する際スケジュールの範囲は現在時刻に合わせて切り詰められるため、
// その開始時刻（Start）を使って設定された時間帯が有効かどうかを判定することはできません。
func isInFetchProgramScheduleHours(now time.Time, hours []int) bool {
	for _, hour := range hours {
		if hour == now.Hour() {
			return true
		}
	}
	return false
}

type fetchProgramScheduleTarget struct {
	TuningSpace TuningSpace
	Title       string
	CustomComID string
}

var fetchProgramScheduleTargets = []fetchProgramScheduleTarget{
	{TuningSpace: TuningSpaceTS, Title: "番組情報取得TS", CustomComID: "FetchProgramTS"},
	{TuningSpace: TuningSpaceBS, Title: "番組情報取得BS", CustomComID: "FetchProgramBS"},
	{TuningSpace: TuningSpaceCS, Title: "番組情報取得CS", CustomComID: "FetchProgramCS"},
	{TuningSpace: TuningSpaceOS, Title: "番組情報取得OS", CustomComID: "FetchProgramOS"},
}

func InitializeFetchProgramScheduler() error {

	// cronへ登録
	cronManager.Add(TaskFetchProgramScheduler, func() {
		if err := scheduler.UpdateFetchProgramSchedules(Now()); err != nil && serviceContext().Err() == nil {
			log.Println("error: 番組情報取得スケジュールの更新に失敗しました", err)
		}
	})

	return nil
}

func (p *Scheduler) UpdateFetchProgramSchedules(now time.Time) error {
	if err := serviceContext().Err(); err != nil {
		return err
	}

	if err := p.DeleteFetchProgramSchedules(now); err != nil {
		return err
	}

	ranges := getFetchProgramScheduleRanges(now, configure.FetchProgram.ScheduleHours)
	if len(ranges) == 0 {
		p.UpdateScheduler()
		return p.AddAllKeywordReserveSchedules()
	}

	// 設定された取得時間帯の間は、空き枠を探して新たな取得予約を作らない。
	// ranges の Start は now で切り詰められるため、境界時刻では now == Start となり、
	// その範囲だけを使った判定では時間帯内と見なせない。
	if isInFetchProgramScheduleHours(now, configure.FetchProgram.ScheduleHours) {
		p.UpdateScheduler()
		return p.AddAllKeywordReserveSchedules()
	}

	end := ranges[len(ranges)-1].End
	schedules, err := p.GetSchedules(0, now.Add(-24*time.Hour), end, MaxScheduleBuffer)
	if err != nil {
		return err
	}

	adds := []*RecordingSchedule{}
	for _, rg := range ranges {
		for _, target := range fetchProgramScheduleTargets {
			if err := serviceContext().Err(); err != nil {
				return err
			}
			if (target.TuningSpace == TuningSpaceTS && !configure.FetchProgram.ScheduleTS) ||
				(target.TuningSpace == TuningSpaceBS && !configure.FetchProgram.ScheduleBS) ||
				(target.TuningSpace == TuningSpaceCS && !configure.FetchProgram.ScheduleCS) ||
				(target.TuningSpace == TuningSpaceOS && !configure.FetchProgram.ScheduleOS) {
				continue
			}

			// 取得する局がなければスケジュールから除外
			if station.GetNumFetchStation(target.TuningSpace) < 1 {
				continue
			}

			sid, ok := station.GetFetchProgramServiceID(target.TuningSpace)
			if !ok {
				log.Printf("info: 番組情報取得でチューナースペースが見つかりません %d", target.TuningSpace)
				continue
			}

			if hasFetchProgramScheduleInRange(target.CustomComID, append(schedules, adds...), rg.Start, rg.End, now.Unix()) {
				continue
			}

			rs, ok := p.FindFetchProgramScheduleSlot(rg.Start, rg.End, sid, target, append(schedules, adds...))
			if !ok {
				log.Printf("info: 番組情報取得の対象が見つかりません %s", target.CustomComID)
				continue
			}
			adds = append(adds, rs)
		}
	}

	if len(adds) == 0 {
		p.UpdateScheduler()
	} else if _, err := p.Insert(0, adds...); err != nil {
		return err
	} else {
		p.UpdateScheduler()
	}

	return p.AddAllKeywordReserveSchedules()
}

func getFetchProgramScheduleRanges(now time.Time, hours []int) []fetchProgramScheduleRange {
	selected := make(map[int]bool)
	for _, hour := range hours {
		if hour >= 0 && hour <= 23 {
			selected[hour] = true
		}
	}
	if len(selected) == 0 {
		return nil
	}

	horizon := time.Duration(configure.FetchProgram.ScheduleWindow) * time.Second
	if horizon < 24*time.Hour {
		horizon = 24 * time.Hour
	}
	windowEnd := now.Add(horizon)
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Add(-24 * time.Hour)
	raw := []fetchProgramScheduleRange{}

	for day := base; !day.After(windowEnd); day = day.Add(24 * time.Hour) {
		for hour := 0; hour < 24; hour++ {
			if !selected[hour] {
				continue
			}
			start := day.Add(time.Duration(hour) * time.Hour)
			end := start.Add(time.Hour)
			if end.After(now) && start.Before(windowEnd) {
				raw = append(raw, fetchProgramScheduleRange{Start: start, End: end})
			}
		}
	}
	if len(raw) == 0 {
		return nil
	}

	ranges := []fetchProgramScheduleRange{}
	cur := raw[0]
	for i := 1; i < len(raw); i++ {
		if !raw[i].Start.After(cur.End) {
			if raw[i].End.After(cur.End) {
				cur.End = raw[i].End
			}
			continue
		}
		ranges = appendClippedFetchProgramScheduleRange(ranges, cur, now, windowEnd)
		cur = raw[i]
	}
	ranges = appendClippedFetchProgramScheduleRange(ranges, cur, now, windowEnd)

	return ranges
}

func appendClippedFetchProgramScheduleRange(ranges []fetchProgramScheduleRange, rg fetchProgramScheduleRange, start, end time.Time) []fetchProgramScheduleRange {
	if rg.Start.Before(start) {
		rg.Start = start
	}
	if rg.End.After(end) {
		rg.End = end
	}
	if rg.Start.Before(rg.End) {
		ranges = append(ranges, rg)
	}
	return ranges
}

func (p *Scheduler) DeleteFetchProgramSchedules(now time.Time) error {
	uxt := now.Unix()
	for _, id := range fetchProgramCustomComIDs {
		rows, err := p.db.GetData(0).Query(
			`SELECT id,deviceid,customcomid,validate,complete,starttime,endtime,idletime,tailtime FROM schedule WHERE customcomid=?`,
			id,
		)
		if err != nil {
			return err
		}

		deleteIDs := []uint64{}
		for rows.Next() {
			sch := &RecordingSchedule{}
			if err := rows.Scan(
				&sch.ID,
				&sch.DeviceID,
				&sch.CustomComID,
				&sch.Validate,
				&sch.Complete,
				&sch.StartTime,
				&sch.EndTime,
				&sch.IdleTime,
				&sch.TailTime,
			); err != nil {
				rows.Close()
				return err
			}
			if !p.shouldKeepFetchProgramSchedule(sch, uxt) {
				deleteIDs = append(deleteIDs, sch.ID)
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}

		for _, deleteID := range deleteIDs {
			if _, err := p.db.GetData(0).Exec(`DELETE FROM schedule WHERE id=?`, deleteID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Scheduler) shouldKeepFetchProgramSchedule(sch *RecordingSchedule, now int64) bool {
	if !sch.Validate || sch.EndTime+sch.TailTime <= now {
		return false
	}
	if sch.Complete {
		return true
	}
	if job, ok := tasks.Get(sch.ID); ok && job.DeviceID == sch.DeviceID {
		return true
	}
	if sch.StartTime-sch.IdleTime > now {
		return false
	}
	if current, ok := p.GetCurrentRecording(sch.DeviceID); ok && current.ID == sch.ID {
		return true
	}
	return false
}

func hasFetchProgramScheduleInRange(customComID string, schedules []*RecordingSchedule, start, end time.Time, now int64) bool {
	for _, sch := range schedules {
		if sch.CustomComID != customComID || !sch.Validate || now > sch.EndTime+sch.TailTime {
			continue
		}
		if rangesOverlap(start.Unix(), end.Unix(), sch.StartTime, sch.EndTime+sch.TailTime) {
			return true
		}
	}
	return false
}

func rangesOverlap(aStart, aEnd, bStart, bEnd int64) bool {
	return aStart < bEnd && bStart < aEnd
}

func (p *Scheduler) FindFetchProgramScheduleSlot(now, end time.Time, sid uint, target fetchProgramScheduleTarget, schedules []*RecordingSchedule) (*RecordingSchedule, bool) {
	order, err := station.GetFetchProgramDeviceIDOrderList(sid)
	if err != nil {
		log.Println("error: 優先デバイスリストの取得に失敗しました", err)
		return nil, false
	}

	for start := now; !start.Add(time.Duration(configure.FetchProgram.ScheduleDuration) * time.Second).After(end); start = start.Add(time.Duration(configure.FetchProgram.SearchStep) * time.Second) {
		for i := range len(order) {
			rs := p.New()
			rs.Title = target.Title
			rs.DeviceID = order[i]
			rs.CustomComID = target.CustomComID
			rs.AutoSelect = false
			rs.EpgFollow = false
			rs.IdleTime = 30
			rs.ReadyTime = 15
			rs.StartTime = start.Unix()
			rs.EndTime = start.Add(time.Duration(configure.FetchProgram.ScheduleDuration) * time.Second).Unix()
			rs.Sid = sid

			dupe := false
			for _, sch := range schedules {
				if isFetchProgramScheduleDuplicate(rs, sch, now.Unix()) {
					dupe = true
					break
				}
			}
			if !dupe {
				return rs, true
			}
		}
	}

	return nil, false
}

func isFetchProgramScheduleDuplicate(sa *RecordingSchedule, sb *RecordingSchedule, now int64) bool {
	saEnd := sa.EndTime + sa.TailTime
	sbEnd := sb.EndTime + sb.TailTime

	if (sa.ID != 0 && sa.ID == sb.ID) || !sa.Validate || !sb.Validate || sa.Complete || sb.Complete || now > saEnd || now > sbEnd || len(sa.DeviceID) < 1 || len(sb.DeviceID) < 1 || sa.DeviceID != sb.DeviceID {
		return false
	}

	if (sb.StartTime <= sa.StartTime && sa.StartTime < sbEnd) ||
		(sb.StartTime < saEnd && saEnd < sbEnd) ||
		(sa.StartTime <= sb.StartTime && sb.StartTime < saEnd) ||
		(sa.StartTime < sbEnd && sbEnd < saEnd) {
		return true
	}

	return false
}

func (p *Scheduler) AddAllKeywordReserveSchedules() error {
	if err := serviceContext().Err(); err != nil {
		return err
	}
	p.muKeywordReserve.Lock()
	defer p.muKeywordReserve.Unlock()
	if err := serviceContext().Err(); err != nil {
		return err
	}

	list, err := p.db.GetKeywordReserves(0)
	if err != nil {
		return err
	}

	total := 0
	for _, kr := range list {
		if err := serviceContext().Err(); err != nil {
			return err
		}
		count, err := p.addKeywordReserveSchedulesNoNotify(kr)
		if err != nil {
			return err
		}
		// 予約があれば日付を更新する
		if count > 0 {
			total += count
			rockDB.UpdateKeywordReserveDate(0, kr)
		}
	}

	if total > 0 {
		log.Printf("info: キーワード予約の追加 total:%d", total)
		p.UpdateScheduler()
	}

	return nil
}

func (p *Station) GetFetchProgramServiceID(tsp TuningSpace) (uint, bool) {
	p.Lock()
	defer p.Unlock()

	var fallback uint
	for sid, val := range p.Cache {
		if TuningSpace(val.Tsp) != tsp {
			continue
		}
		if fallback == 0 {
			fallback = sid
		}
		if val.FetchProgram {
			return sid, true
		}
	}

	return fallback, fallback != 0
}

func (p *Station) GetNumFetchStation(tsp TuningSpace) int {
	p.Lock()
	defer p.Unlock()

	n := 0
	for _, val := range p.Cache {
		if TuningSpace(val.Tsp) != tsp || !val.FetchProgram {
			continue
		}
		n = n + 1
	}

	return n
}

func (p *Scheduler) AddFetchProgramSchedulesImmediately(now time.Time) error {
	horizon := 24 * time.Hour
	schedules, err := p.GetSchedules(0, now.Add(-24*time.Hour), now.Add(horizon), MaxScheduleBuffer)
	if err != nil {
		return err
	}

	adds := []*RecordingSchedule{}
	for _, target := range fetchProgramScheduleTargets {
		if err := serviceContext().Err(); err != nil {
			return err
		}
		if (target.TuningSpace == TuningSpaceTS && !configure.FetchProgram.ScheduleTS) ||
			(target.TuningSpace == TuningSpaceBS && !configure.FetchProgram.ScheduleBS) ||
			(target.TuningSpace == TuningSpaceCS && !configure.FetchProgram.ScheduleCS) ||
			(target.TuningSpace == TuningSpaceOS && !configure.FetchProgram.ScheduleOS) {
			continue
		}

		// 取得する局がなければスケジュールから除外
		if station.GetNumFetchStation(target.TuningSpace) < 1 {
			continue
		}

		sid, ok := station.GetFetchProgramServiceID(target.TuningSpace)
		if !ok {
			log.Printf("info: 即時番組情報取得でチューナースペースのサービスが見つかりません  tuner space=%d", target.TuningSpace)
			continue
		}

		// 今すぐから開始し、最大24時間先までの間で空きスロットを探す
		start := now
		end := now.Add(horizon)

		rs, ok := p.FindFetchProgramScheduleSlot(start, end, sid, target, append(schedules, adds...))
		if !ok {
			log.Printf("info: 即時番組情報取得の対象が見つかりません %s", target.CustomComID)
			continue
		}
		adds = append(adds, rs)
	}

	if len(adds) > 0 {
		if _, err := p.Insert(0, adds...); err != nil {
			return err
		}
	}

	return p.ApplyScheduleUpdate()
}

func (p *Scheduler) ApplyScheduleUpdate() error {
	rs, err := p.ScheduleUpdate()
	if err != nil {
		return err
	}
	if err := tasks.Update(rs); err != nil {
		return err
	}
	deviceController.Notify(ControllerUpdateSchedule, "")
	return nil
}

func (p *Scheduler) GetNextSchedules() ([]*RecordingSchedule, error) {
	now := Now()
	end := now.Add(24 * time.Hour)
	schedules, err := p.GetSchedules(0, now.Add(-24*time.Hour), end, MaxScheduleBuffer)
	if err != nil {
		return nil, err
	}

	ret := []*RecordingSchedule{}
	uxt := now.Unix()
	ext := end.Unix()
	for _, sch := range schedules {
		if !sch.Validate || sch.Complete || uxt > sch.EndTime+sch.TailTime {
			continue
		}
		if sch.StartTime < ext {
			ext = sch.StartTime
		}
	}

	for _, sch := range schedules {
		if !sch.Validate || sch.Complete || uxt > sch.EndTime+sch.TailTime {
			continue
		}
		if sch.StartTime == ext {
			ret = append(ret, sch)
		}
	}

	return ret, nil
}
