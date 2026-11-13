package tvrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"
	"github.com/ikawaha/bleveplugin/analysis/lang/ja"
)

type BleveProgramStore struct {
	index     bleve.Index
	mu        sync.RWMutex
	archiveMu sync.Mutex
}

type bleveProgramDocument struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Sid      uint   `json:"sid"`
	Eid      uint   `json:"eid"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Content  string `json:"content"`
	Caption  string `json:"caption"`
	CnibA    uint   `json:"cnib_a"`
	CnibB    uint   `json:"cnib_b"`
}

// Bleveの型フィールド検索ではJSONタグではなくGoのフィールド名が使用されます。
// そのためJSONマップだけでなく構造体ドキュメントについても明示的に日本語用マッピングを選択してください。
func (p bleveProgramDocument) BleveType() string {
	return "tvprogram"
}

var bleveProgram BleveProgramStore

func (p *BleveProgramStore) Initialize() error {
	if !configure.Bleve {
		return nil
	}

	im, err := newTvProgramBleveMapping()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(configure.BleveFile), 0755); err != nil {
		return err
	}

	var index bleve.Index
	if _, statErr := os.Stat(configure.BleveFile); statErr == nil {
		index, err = bleve.Open(configure.BleveFile)
	} else if os.IsNotExist(statErr) {
		index, err = bleve.New(configure.BleveFile, im)
	} else {
		return statErr
	}
	if err != nil {
		return err
	}
	// 既存のインデックスは、ディスク上の元のマッピングを保持します。
	// インデックス作成や検索が実行される前に、オープンするたびにアクティブなマッピングを拡張します。
	if err := addCaptionBleveMapping(index); err != nil {
		index.Close()
		return err
	}
	p.mu.Lock()
	p.index = index
	p.mu.Unlock()

	// cronへ登録
	cronManager.Add(TaskIndexingArchive, func() {
		now := Now().Unix()

		sch, err := scheduler.GetNextSchedules()
		if err != nil {
			log.Println("error: ", err)
			return
		}

		if len(sch) > 0 && sch[0].StartTime < now+configure.ArchiverSetting.SuspendTime {
			return
		}

		// 過去番組検索登録
		if err := IndexingArchiveProgram(); err != nil && serviceContext().Err() == nil {
			log.Println("error:", err)
		}

	})

	return nil
}

func newTvProgramBleveMapping() (*mapping.IndexMappingImpl, error) {
	keywordFieldMapping := bleve.NewTextFieldMapping()
	keywordFieldMapping.Analyzer = keyword.Name

	jaTextFieldMapping := bleve.NewTextFieldMapping()
	jaTextFieldMapping.Analyzer = "ja"

	numberFieldMapping := bleve.NewNumericFieldMapping()

	dm := bleve.NewDocumentMapping()
	dm.AddFieldMappingsAt("type", keywordFieldMapping)
	dm.AddFieldMappingsAt("id", keywordFieldMapping)
	dm.AddFieldMappingsAt("sid", numberFieldMapping)
	dm.AddFieldMappingsAt("eid", numberFieldMapping)
	dm.AddFieldMappingsAt("start", numberFieldMapping)
	dm.AddFieldMappingsAt("end", numberFieldMapping)
	dm.AddFieldMappingsAt("title", jaTextFieldMapping)
	dm.AddFieldMappingsAt("subtitle", jaTextFieldMapping)
	dm.AddFieldMappingsAt("content", jaTextFieldMapping)
	dm.AddFieldMappingsAt("caption", jaTextFieldMapping)
	dm.AddFieldMappingsAt("cnib_a", numberFieldMapping)
	dm.AddFieldMappingsAt("cnib_b", numberFieldMapping)

	im := bleve.NewIndexMapping()
	im.TypeField = "type"
	im.AddDocumentMapping("tvprogram", dm)

	if err := im.AddCustomTokenizer("ja_tokenizer", map[string]interface{}{
		"type":      ja.Name,
		"dict":      ja.DictIPA,
		"base_form": true,
		"stop_tags": true,
	}); err != nil {
		return nil, fmt.Errorf("failed to create ja tokenizer: %w", err)
	}

	if err := im.AddCustomAnalyzer("ja", map[string]interface{}{
		"type":      custom.Name,
		"tokenizer": "ja_tokenizer",
		"token_filters": []string{
			ja.StopWordsName,
			lowercase.Name,
		},
	}); err != nil {
		return nil, fmt.Errorf("failed to create ja analyzer: %w", err)
	}

	return im, nil
}

func addCaptionBleveMapping(index bleve.Index) error {
	im, ok := index.Mapping().(*mapping.IndexMappingImpl)
	if !ok || im.TypeMapping["tvprogram"] == nil {
		return fmt.Errorf("missing tvprogram index mapping")
	}
	dm := im.TypeMapping["tvprogram"]
	if dm.Properties["caption"] == nil {
		field := bleve.NewTextFieldMapping()
		field.Analyzer = "ja"
		dm.AddFieldMappingsAt("caption", field)
	}
	return im.Validate()
}

func (p *BleveProgramStore) Close() error {
	if !configure.Bleve {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index == nil {
		return nil
	}
	err := p.index.Close()
	p.index = nil
	return err
}

func newBleveProgramDocument(prg *TvProgram, caption string) *bleveProgramDocument {
	return &bleveProgramDocument{
		Type:     "tvprogram",
		ID:       tvProgramBleveID(prg),
		Sid:      prg.Sid,
		Eid:      prg.Eid,
		Start:    prg.Start,
		End:      prg.End,
		Title:    prg.Title,
		Subtitle: prg.Subtitle,
		Content:  prg.Content,
		Caption:  caption,
		CnibA:    prg.CnibA,
		CnibB:    prg.CnibB,
	}
}

func (p *BleveProgramStore) IndexProgram(prg *TvProgram) error {
	doc := newBleveProgramDocument(prg, "")
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index == nil {
		return nil
	}
	if err := p.deleteSameProgramLocked(prg); err != nil {
		return err
	}
	if err := p.index.Delete(tvProgramLegacyBleveID(prg)); err != nil {
		return err
	}
	return p.index.Index(doc.ID, doc)
}

func (p *BleveProgramStore) BatchIndexProgram(prg ...*bleveProgramDocument) error {

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index == nil || len(prg) == 0 {
		return nil
	}

	batch := p.index.NewBatch()

	for _, doc := range prg {
		batch.Delete(fmt.Sprintf("%d:%d", doc.Sid, doc.Eid))
		if err := batch.Index(doc.ID, doc); err != nil {
			return err
		}
	}
	// アーカイブバッチは (start, sid, eid) の順序で処理されます。
	// 中断されたバックフィルを再試行できるよう、新しいカーソルをドキュメントとアトミックにコミットします。
	last := prg[len(prg)-1]
	cursor, err := json.Marshal(archiveCaptionIndexKey{last.Start, last.Sid, last.Eid})
	if err != nil {
		return err
	}
	batch.SetInternal([]byte(archiveCaptionCursorKey), cursor)

	return p.index.Batch(batch)
}

const archiveCaptionCursorKey = "tvrock.archive-caption.v1.cursor"

type archiveCaptionIndexKey struct {
	Start int64
	Sid   uint
	Eid   uint
}

func (p *BleveProgramStore) lastCaptionProgramKey() (int64, uint, uint, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.index == nil {
		return 0, 0, 0, nil
	}
	data, err := p.index.GetInternal([]byte(archiveCaptionCursorKey))
	if err != nil {
		return 0, 0, 0, err
	}
	var key archiveCaptionIndexKey
	if len(data) > 0 {
		if err := json.Unmarshal(data, &key); err != nil {
			return 0, 0, 0, err
		}
	}
	return key.Start, key.Sid, key.Eid, nil
}

func (p *BleveProgramStore) LastProgramStart() (int64, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.index == nil {
		return 0, nil
	}

	req := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), 1, 0, false)
	req.Fields = []string{"start"}
	req.SortBy([]string{"-start"})

	res, err := p.index.Search(req)
	if err != nil {
		return 0, err
	}
	if len(res.Hits) < 1 {
		return 0, nil
	}

	return int64FromBleveField(res.Hits[0].Fields["start"]), nil
}

func (p *BleveProgramStore) LastProgramKey() (int64, uint, uint, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.index == nil {
		return 0, 0, 0, nil
	}

	req := bleve.NewSearchRequestOptions(bleve.NewMatchAllQuery(), 1, 0, false)
	req.Fields = []string{"start", "sid", "eid"}
	req.SortBy([]string{"-start", "-sid", "-eid"})

	res, err := p.index.Search(req)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(res.Hits) < 1 {
		return 0, 0, 0, nil
	}

	fields := res.Hits[0].Fields
	return int64FromBleveField(fields["start"]), uintFromBleveField(fields["sid"]), uintFromBleveField(fields["eid"]), nil
}

func (p *BleveProgramStore) deleteSameProgramLocked(prg *TvProgram) error {
	threshold := float64(Now().Unix() - configure.CurrentProgramThreshold)
	minInclusive := true

	sid := float64(prg.Sid)
	eid := float64(prg.Eid)

	sidQuery := bleve.NewNumericRangeInclusiveQuery(&sid, &sid, &minInclusive, &minInclusive)
	sidQuery.SetField("sid")
	eidQuery := bleve.NewNumericRangeInclusiveQuery(&eid, &eid, &minInclusive, &minInclusive)
	eidQuery.SetField("eid")
	endQuery := bleve.NewNumericRangeInclusiveQuery(&threshold, nil, &minInclusive, nil)
	endQuery.SetField("end")

	req := bleve.NewSearchRequestOptions(bleve.NewConjunctionQuery(sidQuery, eidQuery, endQuery), 1000, 0, false)
	res, err := p.index.Search(req)
	if err != nil {
		return err
	}

	for _, hit := range res.Hits {
		if err := p.index.Delete(hit.ID); err != nil {
			return err
		}
	}
	return nil
}

var errBleveSearchPanic = errors.New("bleve search panic")

// アプリケーションのコールバックで発生したパニックが隠蔽されないよう、リカバリ処理はライブラリの境界に留める。
func searchBleveSafely(ctx context.Context, index bleve.Index, req *bleve.SearchRequest) (result *bleve.SearchResult, err error) {
	defer func() {
		if value := recover(); value != nil {
			log.Printf("warn: bleve search panic: %v\n%s", value, debug.Stack())
			result = nil
			err = fmt.Errorf("%w: %v", errBleveSearchPanic, value)
		}
	}()
	return index.SearchInContext(ctx, req)
}

func (p *BleveProgramStore) Search(ctx context.Context, opt *SearchOption, sid []uint, fn func(*TvProgram, int, int) bool) error {
	if opt == nil {
		return nil
	}

	query := buildTvProgramBleveQuery(opt)
	if query == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	size := int(configure.SearchLimit)
	if size < 1 {
		size = 1000
	}
	size = minInt(maxInt(size*20, 1000), 100000)

	req := bleve.NewSearchRequestOptions(query, size, 0, false)
	req.Fields = []string{"sid", "eid", "start", "end", "title", "subtitle", "content", "cnib_a", "cnib_b"}
	req.SortBy([]string{"start"})

	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.index == nil {
		return nil
	}

	res, err := searchBleveSafely(ctx, p.index, req)
	if err != nil {
		return err
	}

	limit := int(configure.SearchLimit)
	if limit < 1 {
		limit = math.MaxInt
	}

	scanned := 0
	total := int(res.Total)
	if total < len(res.Hits) {
		total = len(res.Hits)
	}
	for _, hit := range res.Hits {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		scanned++
		prg := tvProgramFromBleveFields(hit.Fields)
		if prg == nil {
			continue
		}
		if len(sid) > 0 && !ContainsUint(sid, prg.Sid) {
			continue
		}
		if prg.Start >= opt.StartTime+opt.EndTime || prg.End <= opt.StartTime {
			continue
		}
		if !matchProgramCategory(opt.Category, prg.CnibA) {
			continue
		}
		if !matchProgramWeek(opt.Week, prg) || !matchProgramHours(opt.StartHours, opt.Hours, prg) {
			continue
		}

		if fn != nil && !fn(prg, scanned, total) {
			break
		}
		limit--
		if limit <= 0 {
			break
		}
	}
	return nil
}

func buildTvProgramBleveQuery(opt *SearchOption) query.Query {
	queries := []query.Query{}
	if opt.Title != "" {
		q := bleve.NewMatchQuery(opt.Title)
		q.SetField("title")
		if opt.CaptionSearch {
			caption := bleve.NewMatchQuery(opt.Title)
			caption.SetField("caption")
			queries = append(queries, bleve.NewDisjunctionQuery(q, caption))
		} else {
			queries = append(queries, q)
		}
	}
	if opt.Content != "" {
		subtitle := bleve.NewMatchQuery(opt.Content)
		subtitle.SetField("subtitle")
		content := bleve.NewMatchQuery(opt.Content)
		content.SetField("content")
		fields := []query.Query{subtitle, content}
		if opt.CaptionSearch {
			caption := bleve.NewMatchQuery(opt.Content)
			caption.SetField("caption")
			fields = append(fields, caption)
		}
		queries = append(queries, bleve.NewDisjunctionQuery(fields...))
	}
	if len(queries) < 1 {
		return nil
	}
	var textQuery query.Query = bleve.NewDisjunctionQuery(queries...)
	if opt.AndSearch {
		textQuery = bleve.NewConjunctionQuery(queries...)
	}
	// インデックスクエリにおいて、結果件数の制限を適用する前に期間を適用します。
	start, end := float64(opt.StartTime), float64(opt.StartTime+opt.EndTime)
	exclusive := false
	startsBefore := bleve.NewNumericRangeInclusiveQuery(nil, &end, nil, &exclusive)
	startsBefore.SetField("start")
	endsAfter := bleve.NewNumericRangeInclusiveQuery(&start, nil, &exclusive, nil)
	endsAfter.SetField("end")
	return bleve.NewConjunctionQuery(textQuery, startsBefore, endsAfter)
}

func tvProgramBleveID(prg *TvProgram) string {
	return fmt.Sprintf("%d:%d:%d", prg.Sid, prg.Eid, prg.Start)
}

func tvProgramLegacyBleveID(prg *TvProgram) string {
	return fmt.Sprintf("%d:%d", prg.Sid, prg.Eid)
}

func tvProgramFromBleveFields(fields map[string]interface{}) *TvProgram {
	if fields == nil {
		return nil
	}
	return &TvProgram{
		Sid:      uintFromBleveField(fields["sid"]),
		Eid:      uintFromBleveField(fields["eid"]),
		Start:    int64FromBleveField(fields["start"]),
		End:      int64FromBleveField(fields["end"]),
		Title:    stringFromBleveField(fields["title"]),
		Subtitle: stringFromBleveField(fields["subtitle"]),
		Content:  stringFromBleveField(fields["content"]),
		CnibA:    uintFromBleveField(fields["cnib_a"]),
		CnibB:    uintFromBleveField(fields["cnib_b"]),
	}
}

func stringFromBleveField(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func int64FromBleveField(v interface{}) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case uint:
		return int64(n)
	case float64:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}

func uintFromBleveField(v interface{}) uint {
	n := int64FromBleveField(v)
	if n < 0 {
		return 0
	}
	return uint(n)
}
