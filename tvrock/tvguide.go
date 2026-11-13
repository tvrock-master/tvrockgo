package tvrock

import (
	"regexp"
	"sort"
	"sync"
	"time"
)

type TvGuide struct {
	Sta                 map[uint]*TvStation
	Tvp                 map[uint][]*TvProgram
	Start               int64
	End                 int64
	Week                bool
	ServiceList         []uint
	CurrentMilliseconds int64
}

type NewTvGuide struct {
	New   TvGuide
	End   TvGuide
	First TvGuide
	Free  TvGuide
}

type FeatureProgramCache struct {
	Feature []map[string][]*TvProgram
	Update  time.Time
	sync.RWMutex
}

var featureProgramCache FeatureProgramCache

func GetTvGuide(sid []uint, st time.Time, ed time.Time, limit int64) (TvGuide, error) {
	sta := station.GetStationList()
	tg := TvGuide{}
	tg.Sta = make(map[uint]*TvStation)
	tg.Tvp = make(map[uint][]*TvProgram)
	tg.Start = st.Unix()
	tg.End = ed.Unix()
	tg.Week = false
	tg.ServiceList = append(tg.ServiceList, sid...)
	tg.CurrentMilliseconds = Now().UnixMilli()

	programs, err := tvprogram.getProgramsResolved(guideProgramSlots(st, ed), sid, st, ed, limit)
	if err != nil {
		return TvGuide{}, err
	}
	for _, id := range sid {
		if sta[id] != nil {
			tg.Sta[id] = sta[id]
			tg.Tvp[id] = programs[id]
		}
	}

	return tg, nil
}

func GetTvGuideWeek(sid []uint, st time.Time, ed time.Time, limit int64) (TvGuide, error) {
	const week = 7

	tg := TvGuide{}
	tg.Sta = make(map[uint]*TvStation)
	tg.Tvp = make(map[uint][]*TvProgram)
	tg.Start = st.Unix()
	tg.End = ed.Unix()
	tg.Week = true
	tg.CurrentMilliseconds = Now().UnixMilli()

	sta := station.GetStationList()
	for _, s := range sta {
		if ContainsUint(sid, s.Sid) {
			tg.Sta[s.Sid] = sta[s.Sid]
		}
	}

	for i := 0; i < week; i++ {
		day := uint(i)
		tg.Tvp[day] = []*TvProgram{}

		programs, err := tvprogram.getProgramsResolved(guideProgramSlots(st, ed), sid, st, ed, limit)
		if err != nil {
			return TvGuide{}, err
		}
		for _, id := range sid {
			if sta[id] != nil {
				tg.Tvp[day] = appendUniquePrograms(tg.Tvp[day], programs[id]...)
			}
		}

		sort.Slice(tg.Tvp[day], func(a, b int) bool {
			return tg.Tvp[day][a].Start < tg.Tvp[day][b].Start
		})
		st = st.Add(24 * time.Hour)
		ed = ed.Add(24 * time.Hour)
	}

	return tg, nil
}

func appendUniquePrograms(dst []*TvProgram, src ...*TvProgram) []*TvProgram {
	for _, v := range src {
		match := false
		for _, t := range dst {
			if t.Sid == v.Sid && t.Eid == v.Eid && t.Start == v.Start {
				match = true
				break
			}
		}
		if !match {
			dst = append(dst, v)
		}
	}
	return dst
}

func GetTvFeature() ([]map[string][]*TvProgram, error) {
	featureProgramCache.RLock()
	update := featureProgramCache.Update
	cache := featureProgramCache.Feature
	featureProgramCache.RUnlock()

	if Now().Sub(update) < time.Hour {
		return cache, nil
	}

	features := configure.Features
	if len(features) == 0 {
		features = defaultFeatures
	}

	type compiledFeature struct {
		name string
		re   *regexp.Regexp
	}
	compiled := make([]compiledFeature, 0, len(features))
	for _, fp := range features {
		re, err := regexp.Compile(fp.Pattern)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, compiledFeature{name: fp.Name, re: re})
	}
	tslist := []TuningSpace{TuningSpaceTS, TuningSpaceBS, TuningSpaceCS, TuningSpaceOS}
	slist := station.GetStationList()
	ret := []map[string][]*TvProgram{}

	t1 := Now()
	t2 := t1.Add(24 * 8 * time.Hour)
	for _, tsp := range tslist {
		list := []uint{}
		for _, st := range slist {
			if TuningSpace(st.Tsp) == tsp {
				list = append(list, st.Sid)
			}
		}
		tg, err := GetTvGuide(list, t1, t2, 0)
		if err != nil {
			return nil, err
		}

		ft := map[string][]*TvProgram{}
		for _, cf := range compiled {
			hits := []*TvProgram{}
			if len(list) > 0 {
				for _, tvp := range tg.Tvp {
					for _, prg := range tvp {
						if prg.CommonSid == nil && cf.re.MatchString(prg.Title) {
							hits = append(hits, prg)
						}
					}
				}
			}
			ft[cf.name] = hits
		}
		ret = append(ret, ft)
	}

	featureProgramCache.Lock()
	featureProgramCache.Feature = ret
	featureProgramCache.Update = Now()
	featureProgramCache.Unlock()

	return ret, nil
}
