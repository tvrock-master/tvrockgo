package tvrock

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type SearchJobCom int

const (
	SearchJobStay SearchJobCom = iota
	SearchJobBusy
	SearchJobPause
	SearchJobContinue
	SearchJobBreak
	SearchJobFinish
)

type SearchOption struct {
	ID       uint64 `json:"id"`       // 検索ID
	Validate bool   `json:"validate"` // 検索有効
	DeviceID string `json:"deviceid"` // デバイスID

	StartTime  int64 `json:"starttime"`  // 検索開始時刻
	EndTime    int64 `json:"endtime"`    // 検索期間
	StartHours int64 `json:"starthours"` // 開始時間
	Hours      int64 `json:"endhours"`   // 検索時間

	IdleTime  int64 `json:"idletime"`  // 待機時間（秒）
	ReadyTime int64 `json:"readytime"` // 録画開始時間（秒）
	TailTime  int64 `json:"tailtime"`  // 録画終了時間（秒）

	Title   string `json:"title"`   // 検索キーワード
	Content string `json:"content"` // 検索キーワード

	Sid      []uint `json:"sid"`      // 局指定
	Category uint   `json:"category"` // カテゴリ指定
	Week     uint8  `json:"week"`     // 曜日指定（0は全曜日、bit 0は日曜〜bit 6は土曜）

	LastUpdate int64 `json:"lastupdate"` // 最終更新日時
	Hits       uint  `json:"hits"`       // 検索された数
	Priority   uint  `json:"priority"`

	AndSearch     bool `json:"andsearch"`     // タイトル&内容 or タイトル|内容
	FuzzySearch   bool `json:"fuzzysearch"`   // あいまい
	ArchiveSearch bool `json:"archivesearch"` // 過去検索
	CaptionSearch bool `json:"captionsearch"` // 字幕検索
	RecordOnly    bool `json:"recordonly"`    // 録画のみ
	WatchOnly     bool `json:"watchonly"`     // 視聴のみ
	ShutDown      bool `json:"shutdown"`      // 録画終了後アプリ終了
	EpgFollow     bool `json:"epgfollow"`     // 時間追従
	AutoSelect    bool `json:"autoselect"`    // 自動デバイス選択

	CustomComID string `json:"customcomid"`
	HashTag     string `json:"hashtag"`
}

type SearchTask struct {
	ID         uint64
	UID        string
	option     *SearchOption
	title      *regexp.Regexp
	content    *regexp.Regexp
	exclusions searchExclusions
	current    int64
	leng       int64
	sid        []uint
	state      SearchJobCom
	com        chan SearchJobCom
	ctx        context.Context
	cancel     context.CancelFunc
	result     []*TvProgram
	match      func(p []*TvProgram)
	finish     func(p *SearchTask)
	progress   func(p *SearchProgress)
	warning    string
}

var searchTask []*SearchTask
var taskMux sync.Mutex
var taskID uint64

func InitializeSearch() error {
	return bleveProgram.Initialize()
}

func (p *SearchTask) verifyProgramBatch(ctx context.Context, batch []*TvProgram, dbIndex int) ([]*TvProgram, error) {
	if len(batch) == 0 {
		return nil, nil
	}
	workerCount := configure.SearchMaxWorkerThreads
	if workerCount < 1 {
		workerCount = 1
	}
	if workerCount > len(batch) {
		workerCount = len(batch)
	}

	type checkResult struct {
		index   int
		matched bool
		err     error
	}

	jobs := make(chan int, len(batch))
	results := make(chan checkResult, len(batch))

	var wg sync.WaitGroup
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				v := batch[idx]
				if p.exclusions.matchesProgram(v, p.option.FuzzySearch) {
					results <- checkResult{index: idx, matched: false}
					continue
				}
				matched := matchProgramText(p.option.AndSearch, p.option.FuzzySearch, p.title, p.content, v)
				var captionErr error
				if p.option.CaptionSearch && (!matched || p.exclusions.active()) {
					matched, captionErr = rockDB.matchProgramCaptionInDB(ctx, p.option.AndSearch, p.option.FuzzySearch, p.title, p.content, v, p.exclusions, dbIndex)
				}
				results <- checkResult{index: idx, matched: matched, err: captionErr}
			}
		}()
	}

	for idx := range batch {
		jobs <- idx
	}
	close(jobs)

	wg.Wait()
	close(results)

	resArray := make([]checkResult, len(batch))
	var firstErr error
	for r := range results {
		resArray[r.index] = r
		if r.err != nil && firstErr == nil {
			firstErr = r.err
		}
	}

	matchedList := make([]*TvProgram, 0, len(batch))
	for idx, r := range resArray {
		if r.matched {
			matchedList = append(matchedList, batch[idx])
		}
	}
	return matchedList, firstErr
}

func (p *SearchTask) Exec() {

	p.state = SearchJobBusy

	defer p.Delete()

	if p.option.useBleve() {
		p.execBleve()
		return
	}

	limit := int64(configure.SearchLimit)
	if limit < 1 {
		limit = math.MaxInt64
	}
	ctx := p.ctx
	if ctx == nil {
		ctx = serviceContext()
	}

	end := p.option.StartTime + p.option.EndTime
	seen := make(map[searchProgramKey]bool)

	searchStart := p.current
	scanned := 0
	lastProgress := Now()
	p.sendProgress(searchStart, end, p.current, scanned, true)

searchLoop:
	for p.current < end && int64(len(p.result)) < limit {
		if ctx.Err() != nil {
			p.state = SearchJobBreak
			break
		}

		select {
		case v, ok := <-p.com:
			if !ok {
				p.state = SearchJobBreak
				break
			}
			p.state = v
		default:
		}

		switch p.state {
		case SearchJobPause:
			continue
		case SearchJobContinue:
			p.state = SearchJobBusy
		}
		if p.state == SearchJobBreak {
			break
		}

		t1 := time.Unix(p.current, 0)
		ln := p.current + p.leng
		var t2 time.Time
		if end < ln {
			t2 = time.Unix(end, 0)
		} else {
			t2 = time.Unix(ln, 0)
		}
		t1ux := t1.Unix()
		t2ux := t2.Unix()

		for i := range 2 {
			switch i {
			case 0:
				if t2ux < tvprogram.FirstStartTime[i] {
					continue
				}
			case 1:
				if !p.option.ArchiveSearch {
					continue
				}
				if t1ux >= tvprogram.LastEndTime[i] {
					continue
				}
			}

			var matchBatch []*TvProgram
			fetched := 0
			var captionErr error
			const candidateBatchSize = 64
			candidates := make([]*TvProgram, 0, candidateBatchSize)

			flushCandidates := func() bool {
				if len(candidates) == 0 {
					return true
				}
				matched, err := p.verifyProgramBatch(ctx, candidates, i)
				candidates = candidates[:0]
				if err != nil && captionErr == nil {
					captionErr = err
				}
				for _, v := range matched {
					key := searchProgramKey{v.Sid, v.Eid, v.Start}
					if seen[key] {
						continue
					}
					seen[key] = true
					p.result = append(p.result, v)
					matchBatch = append(matchBatch, v)
					if int64(len(p.result)) >= limit {
						return false
					}
				}
				return true
			}

			// テキストフィルタリングの前に調査されるプログラムではなく一致した結果を制限します
			err := tvprogram.GetFromSID(i, p.sid, t1, t2, 0, func(v *TvProgram) bool {
				fetched++
				scanned++
				select {
				case cmd, ok := <-p.com:
					if !ok {
						p.state = SearchJobBreak
						return false
					}
					p.state = cmd
				default:
				}
				if p.state == SearchJobBreak {
					return false
				}
				if int64(len(p.result)) >= limit {
					return false
				}
				key := searchProgramKey{v.Sid, v.Eid, v.Start}
				if seen[key] {
					return true
				}
				if !matchProgramCategory(p.option.Category, v.CnibA) {
					return true
				}

				if !matchProgramWeek(p.option.Week, v) || !matchProgramHours(p.option.StartHours, p.option.Hours, v) {
					return true
				}

				candidates = append(candidates, v)
				if len(candidates) >= candidateBatchSize {
					if !flushCandidates() {
						return false
					}
				}

				if scanned%500 == 0 && time.Since(lastProgress) > 500*time.Millisecond {
					lastProgress = Now()
					p.sendProgress(searchStart, end, t1ux, scanned, true)
				}
				return int64(len(p.result)) < limit
			})
			if len(candidates) > 0 && p.state != SearchJobBreak && int64(len(p.result)) < limit {
				flushCandidates()
			}
			if len(matchBatch) > 0 && p.match != nil {
				p.match(matchBatch)
			}
			if err != nil || captionErr != nil {
				log.Printf("error: program search: %v, caption search: %v", err, captionErr)
				p.state = SearchJobBreak
			}
			if p.state == SearchJobBreak {
				break searchLoop
			}

			log.Printf("debug: %s:searching %d ", t1.String(), fetched)
		}
		p.current = t2ux
		p.sendProgress(searchStart, end, p.current, scanned, true)
	}

	if p.state != SearchJobBreak {
		p.state = SearchJobFinish
		p.current = end
	}
	p.sendProgress(searchStart, end, p.current, scanned, false)

	if p.finish != nil {
		p.finish(p)
	}
}

func (p *SearchTask) execBleve() {
	p.state = SearchJobBusy
	parent := p.ctx
	if parent == nil {
		parent = serviceContext()
	}
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case _, ok := <-p.com:
			if !ok {
				cancel()
			}
		case <-done:
		}
	}()

	total := 0
	scanned := 0
	lastProgress := Now()
	matchBatch := []*TvProgram{}
	p.sendBleveProgress(total, scanned, true)

	err := bleveProgram.Search(ctx, p.option, p.sid, func(v *TvProgram, current, hits int) bool {
		select {
		case <-ctx.Done():
			p.state = SearchJobBreak
			return false
		default:
		}

		total = hits
		scanned = current
		p.result = append(p.result, v)
		matchBatch = append(matchBatch, v)

		if len(matchBatch) >= 100 && p.match != nil {
			p.match(matchBatch)
			matchBatch = nil
		}
		if scanned%500 == 0 && time.Since(lastProgress) > 500*time.Millisecond {
			lastProgress = Now()
			p.sendBleveProgress(total, scanned, true)
		}

		return configure.SearchLimit < 1 || len(p.result) < configure.SearchLimit
	})
	if errors.Is(err, errBleveSearchPanic) {
		p.warning = fmt.Sprintf("Bleveの検索データの読み取りで異常が発生したため、検索を中止しました。録画などの処理が終わってからサーバーを停止し、Bleveデータフォルダー（%s）をバックアップ後に削除して再起動してください。検索データは順次再構築されます。番組・字幕のDBは削除しないでください。", configure.BleveFile)
		log.Println("warn:", p.warning)
		p.state = SearchJobBreak
	} else if err != nil && !errors.Is(err, context.Canceled) {
		log.Println("error: bleve search:", err)
		p.state = SearchJobBreak
	} else if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		p.state = SearchJobBreak
	} else {
		if len(matchBatch) > 0 && p.match != nil {
			p.match(matchBatch)
		}
		p.state = SearchJobFinish
	}

	p.sendBleveProgress(total, scanned, false)
	if p.finish != nil {
		p.finish(p)
	}
}

func calcSearchSliceLength(endTime int64) int64 {
	const oneDay = 24 * 60 * 60
	if endTime <= 7*oneDay {
		return oneDay
	} else if endTime <= 30*oneDay {
		return 2 * oneDay
	}
	return 7 * oneDay
}

func (p *SearchTask) Add() uint64 {
	defer taskMux.Unlock()
	taskMux.Lock()
	taskID++
	p.ID = taskID
	p.com = make(chan SearchJobCom)
	p.ctx, p.cancel = context.WithCancel(serviceContext())
	p.current = p.option.StartTime
	p.leng = calcSearchSliceLength(p.option.EndTime)
	p.state = SearchJobStay
	searchTask = append(searchTask, p)
	if !serviceWorkers.start(p.Exec) {
		p.cancel()
		searchTask = searchTask[:len(searchTask)-1]
		p.state = SearchJobBreak
	}

	log.Printf("debug: add search task %d:%s ", p.ID, p.option.DeviceID)
	return taskID
}

func unsetTask(s []*SearchTask, i int) []*SearchTask {
	if i >= len(s) {
		return s
	}
	return append(s[:i], s[i+1:]...)
}

func (p *SearchTask) Delete() {
	if p.cancel != nil {
		p.cancel()
	}
	defer taskMux.Unlock()
	taskMux.Lock()
	for i, v := range searchTask {
		if p.ID == v.ID {
			close(p.com)
			searchTask = unsetTask(searchTask, i)
			break
		}
	}

	log.Printf("debug: delete task%d leng %d", p.ID, len(searchTask))
}

func matchProgramCategory(category uint, cnibA uint) bool {
	if category == 0 || category == 0xffff {
		return true
	}
	if cnibA > 15 {
		return false
	}
	return category&(1<<cnibA) != 0
}

// matchProgramWeekはサーバーのローカル時刻で番組開始日の曜日を判定する。
func matchProgramWeek(week uint8, p *TvProgram) bool {
	return week == 0 || week&(1<<uint(time.Unix(p.Start, 0).Weekday())) != 0
}

func matchProgramHours(startHours int64, hours int64, p *TvProgram) bool {
	if hours >= 24 {
		return true
	}
	st := int64(time.Unix(p.Start, 0).Hour())
	stp := st + 24
	sed := startHours + hours
	if (startHours <= st && st < sed) || (startHours <= stp && stp < sed) {
		return true
	}
	return false
}

func fuzzySearchText(s string) string {
	s = norm.NFKC.String(s)
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		if r >= 'ぁ' && r <= 'ゖ' {
			return r + ('ァ' - 'ぁ')
		}
		return unicode.ToLower(r)
	}, s)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func fuzzyApproximateTextMatch(keyword string, text string) bool {
	keywordRunes := []rune(keyword)
	textRunes := []rune(text)
	keywordLen := len(keywordRunes)
	textLen := len(textRunes)

	if keywordLen < 1 {
		return false
	}
	if keywordLen < 3 {
		return strings.Contains(text, keyword)
	}
	if strings.Contains(text, keyword) {
		return true
	}

	maxDistance := keywordLen / 4
	minWindowLen := maxInt(1, keywordLen-maxDistance)
	maxWindowLen := minInt(textLen, keywordLen+maxDistance)
	if textLen < minWindowLen {
		return fuzzyTextSimilarityOK(keywordRunes, textRunes)
	}

	// 枝刈り: テキスト全体にキーワードの構成文字が不足していれば即座に不一致
	requiredMatches := keywordLen - maxDistance
	keywordCounts := make(map[rune]int, keywordLen)
	for _, r := range keywordRunes {
		keywordCounts[r]++
	}
	matchedRunes := 0
	for _, r := range textRunes {
		if c, ok := keywordCounts[r]; ok && c > 0 {
			matchedRunes++
			if matchedRunes >= requiredMatches {
				break
			}
		}
	}
	if matchedRunes < requiredMatches {
		return false
	}

	for windowLen := minWindowLen; windowLen <= maxWindowLen; windowLen++ {
		for i := 0; i+windowLen <= textLen; i++ {
			if fuzzyTextSimilarityOK(keywordRunes, textRunes[i:i+windowLen]) {
				return true
			}
		}
	}

	return false
}

func fuzzyTextSimilarityOK(a []rune, b []rune) bool {
	baseLen := maxInt(len(a), len(b))
	if baseLen < 1 {
		return false
	}
	distance := levenshteinDistance(a, b)
	return distance*4 <= baseLen
}

func levenshteinDistance(a []rune, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)

	for j := range prev {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			cur[j] = minInt(
				minInt(cur[j-1]+1, prev[j]+1),
				prev[j-1]+cost,
			)
		}
		prev, cur = cur, prev
	}

	return prev[len(b)]
}

func containsRegexMeta(s string) bool {
	return strings.ContainsAny(s, `\.+*?()|[]{}^$`)
}

func fuzzySearchPattern(s string) string {
	// 正規化で全角記号が正規表現の演算子になるのを防ぐ。
	var pattern strings.Builder
	escaped := false
	for _, r := range s {
		normalized := norm.NFKC.String(string(r))
		if r > unicode.MaxASCII && containsRegexMeta(normalized) {
			if escaped {
				pattern.WriteString(normalized)
			} else {
				pattern.WriteString(regexp.QuoteMeta(normalized))
			}
		} else {
			pattern.WriteRune(r)
		}
		escaped = r == '\\' && !escaped
	}
	return fuzzySearchText(pattern.String())
}

func matchRegexpText(re *regexp.Regexp, s string, fuzzySearch bool) bool {
	if re == nil {
		return false
	}
	if fuzzySearch {
		s = fuzzySearchText(s)
		if literal, complete := re.LiteralPrefix(); complete && fuzzyApproximateTextMatch(literal, s) {
			return true
		}
	}
	return re.MatchString(s)
}

func matchProgramText(andSearch bool, fuzzySearch bool, title *regexp.Regexp, content *regexp.Regexp, prg *TvProgram) bool {
	titleMatch := matchRegexpText(title, prg.Title, fuzzySearch)
	contentMatch := content != nil && (matchRegexpText(content, prg.Subtitle, fuzzySearch) || matchRegexpText(content, prg.Content, fuzzySearch))
	return matchSearchTerms(andSearch, title, content, titleMatch, contentMatch)
}

func matchSearchTerms(andSearch bool, title, content *regexp.Regexp, titleMatch, contentMatch bool) bool {
	if andSearch {
		if title != nil && !titleMatch {
			return false
		}
		if content != nil && !contentMatch {
			return false
		}
		return title != nil || content != nil
	}

	return titleMatch || contentMatch
}

func NewSearch() *SearchOption {

	rs := SearchOption{}

	rs.ID = 0
	rs.Validate = true
	rs.DeviceID = ""

	rs.StartTime = Now().Unix()
	rs.EndTime = 10 * 24 * 60 * 60 // 1week+
	rs.StartHours = 0
	rs.Hours = 24 // 24hours

	rs.IdleTime = 60
	rs.ReadyTime = 30
	rs.TailTime = 0

	rs.Title = ""
	rs.Content = ""

	rs.Sid = []uint{}
	rs.Category = 0xffff

	rs.LastUpdate = rs.StartTime
	rs.Hits = 0
	rs.Priority = 0

	rs.AndSearch = false
	rs.FuzzySearch = true
	rs.ArchiveSearch = false
	rs.RecordOnly = true
	rs.WatchOnly = false
	rs.ShutDown = true
	rs.EpgFollow = true
	rs.AutoSelect = true

	rs.CustomComID = ""

	return &rs
}

func (p *SearchOption) useBleve() bool {
	return configure.Bleve && p.ArchiveSearch && !p.FuzzySearch &&
		!searchNotSeparator.MatchString(p.Title) && !searchNotSeparator.MatchString(p.Content)
}

// 局未指定時は親サービスだけを検索する。明示的な局指定はそのまま尊重する。
func searchServiceIDs(requested []uint) []uint {
	ids := []uint{}
	for _, s := range station.GetStationList() {
		if len(requested) == 0 && (s.View == 0 || (s.Parent != 0 && s.Parent != s.Sid)) {
			continue
		}
		if len(requested) == 0 || ContainsUint(requested, s.Sid) {
			ids = append(ids, s.Sid)
		}
	}
	return ids
}

func (p *SearchOption) Search(uid string, match func(p []*TvProgram), finishfunc func(p *SearchTask), progressfunc ...func(p *SearchProgress)) error {

	var progress func(*SearchProgress)
	if len(progressfunc) > 0 {
		progress = progressfunc[0]
	}

	ss := searchServiceIDs(p.Sid)

	var regt *regexp.Regexp
	var regc *regexp.Regexp
	var exclusions searchExclusions
	var err error

	if !p.useBleve() {
		regt, exclusions.title, err = compileSearchPattern(p.Title, p.FuzzySearch)
		if err != nil {
			return err
		}
		regc, exclusions.content, err = compileSearchPattern(p.Content, p.FuzzySearch)
		if err != nil {
			return err
		}
	}

	// タイトルと内容の両方が空の場合は検索しない
	if len(ss) == 0 || (regt == nil && regc == nil && (!p.useBleve() || (strings.TrimSpace(p.Title) == "" && strings.TrimSpace(p.Content) == ""))) {
		task := SearchTask{
			UID:      uid,
			option:   p,
			progress: progress,
		}
		task.Cancel()
		end := p.StartTime + p.EndTime
		task.sendProgress(p.StartTime, end, end, 0, false)
		if finishfunc != nil {
			finishfunc(&task)
		}
		return nil
	}

	task := SearchTask{
		UID:        uid,
		option:     p,
		title:      regt,
		content:    regc,
		exclusions: exclusions,
		sid:        ss,
		match:      match,
		finish:     finishfunc,
		progress:   progress,
	}

	task.Cancel()

	task.Add()

	return nil
}

func (p *SearchTask) sendProgress(start, end, current int64, scanned int, running bool) {
	if p.progress == nil {
		return
	}
	if current < start {
		current = start
	}
	if current > end {
		current = end
	}
	percent := 0
	if end > start {
		percent = int(((current - start) * 100) / (end - start))
	}
	p.progress(&SearchProgress{
		Progress: true,
		Running:  running,
		Start:    start,
		End:      end,
		Current:  current,
		Percent:  percent,
		Scanned:  scanned,
		Hits:     len(p.result),
	})
}

func (p *SearchTask) sendBleveProgress(total, scanned int, running bool) {
	if p.progress == nil {
		return
	}
	percent := 0
	if total > 0 {
		percent = int((int64(scanned) * 100) / int64(total))
	}
	if !running {
		percent = 100
	}
	p.progress(&SearchProgress{
		Warning:  p.warning,
		Progress: true,
		Running:  running,
		Start:    0,
		End:      int64(total),
		Current:  int64(scanned),
		Percent:  percent,
		Scanned:  scanned,
		Hits:     len(p.result),
	})
}

func (p *SearchTask) Cancel() {
	defer taskMux.Unlock()
	taskMux.Lock()

	for i := 0; i < len(searchTask); {
		v := searchTask[i]
		if v.UID == p.UID {
			if v.cancel != nil {
				v.cancel()
			}
			close(v.com)
			searchTask = unsetTask(searchTask, i)
			log.Printf("debug: cancel search %d", v.ID)
		} else {
			i++
		}
	}
}
