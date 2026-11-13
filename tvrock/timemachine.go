package tvrock

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
	"gopkg.in/ini.v1"
)

const timeMachineBatchSize = 512
const timeMachineMaxWorkers = 8

var gaijiCode = []string{
	"🅊", "🅌", "🄿", "🅆", "🅋",
	"🈐", "🈑", "🈑", "🈒", "🈓",
	"🈔", "🈕", "🈖", "🅍", "🄱",
	"🄽", "🈗", "🈘", "🈙", "🈚",
	"🈛", "⚿", "🈜", "🈝", "🈞",
	"🈟", "🈡", "🈢", "🅂", "🈤",
	"🈠", "🅎",
	// "🈣", "🈤", "🈥",, "㊙", "🈀"
}

// ImportTimeMachineはオフラインインポートに必要なストアのみを開きます。
// 通常のサーバーやそのバックアップサービスは意図的に初期化しません。
func ImportTimeMachine(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var conf TvRockConf
	if err := conf.Load(TvRockConfigureFile); err != nil {
		return fmt.Errorf("取り込み用の運用設定を読み込めません: %w", err)
	}

	if conf.DBFile == "" || conf.ArchiveDBFile == "" {
		return fmt.Errorf("取り込みにはdbfileとarchivedbfileの設定が必要です")
	}

	absolute, err := filepath.Abs(conf.DBFile)
	if err != nil {
		return err
	}

	uriPath := filepath.ToSlash(absolute)
	if filepath.VolumeName(absolute) != "" {
		uriPath = "/" + uriPath
	}

	stationURI := (&url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro"}).String()
	dataDB, err := sql.Open("sqlite", stationURI)
	if err != nil {
		return err
	}
	defer dataDB.Close()

	db := &RockDB{Data: []*sql.DB{dataDB, nil}, Program: make([]*sql.DB, 2)}
	stations := &Station{db: db}
	stations.Cache, err = stations.GetStations(0)
	if err != nil {
		return fmt.Errorf("取り込み用の局情報を読み込めません: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(conf.ArchiveDBFile), 0755); err != nil {
		return err
	}

	programDB, err := sql.Open("sqlite", sqliteDSNWithSettings(conf.ArchiveDBFile, conf.SqliteSetting))
	if err != nil {
		return err
	}

	defer func() {
		log.Println("info: 取り込み用データベースを閉じます")
		if err := programDB.Close(); err != nil {
			log.Println("error: 取り込み用データベースの終了:", err)
		}
	}()

	if err := CreateProgramTable(programDB); err != nil {
		return err
	}

	if _, err := programDB.ExecContext(ctx, "PRAGMA optimize"); err != nil {
		return err
	}
	db.Program[1] = programDB
	importer := &timeMachineImporter{stations: stations, programs: &TvProgramStore{db: db}}
	return importer.scan(ctx, path)
}

type timeMachineImporter struct {
	stations *Station
	programs *TvProgramStore
}

func TimeMachineTableScan(ctx context.Context, path string) error {
	return (&timeMachineImporter{stations: &station, programs: &tvprogram}).scan(ctx, path)
}

func (p *timeMachineImporter) scan(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	sta, err := p.scanServiceID(filepath.Join(path, "dtv.ini"))
	if err != nil {
		log.Println("error:", err)
		return err
	}

	if err := p.scanTvProgram(ctx, filepath.Join(path, "timemachine"), sta); err != nil {
		return err
	}

	return p.scanTvProgram(ctx, path, sta)
}

func ScanServiceID(dtvini string) (map[string]*TvStation, error) {
	return (&timeMachineImporter{stations: &station}).scanServiceID(dtvini)
}

func (p *timeMachineImporter) scanServiceID(dtvini string) (map[string]*TvStation, error) {
	tmp := dtvini + ".tmp.ini"

	err := CreateIniTmp(dtvini, tmp)
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp)

	cfg, err := ini.Load(tmp)
	if err != nil {
		return nil, err
	}

	sta := map[string]*TvStation{}
	r := regexp.MustCompile(`SERVICE$`)
	for _, secs := range cfg.Sections() {
		if r.MatchString(secs.Name()) {
			for _, key := range secs.Keys() {
				v, ok := sta[key.Name()]
				if !ok {
					v = &TvStation{
						Sid:  0,
						Tsid: 0,
						Ch:   0,
						Tsp:  uint(TuningSpaceTS),
						Name: key.Name(),
					}
				}

				val, err := key.Uint()
				if err != nil {
					return nil, err
				}
				if val == 0 {
					continue
				}
				v.Sid = val
				sta[key.Name()] = v
			}
		}
	}
	/*
		r = regexp.MustCompile(`SERVICE-L$`)
		for _, secs := range cfg.Sections() {
			if r.MatchString(secs.Name()) {
				for _, key := range secs.Keys() {
					if _, ok := sta[key.Name()]; ok {
						continue
					}

					val, err := key.Uint()
					if err != nil {
						return err
					}

					sta[key.Name()] = &TvStation{
						Sid:  val,
						Freq: 0,
						Ch:   0,
						Tsp:  0,
						Name: key.Name(),
					}
				}
			}
		}
	*/
	r = regexp.MustCompile(`CHANNEL$`)
	for _, secs := range cfg.Sections() {
		if r.MatchString(secs.Name()) {
			for _, key := range secs.Keys() {
				v, ok := sta[key.Name()]
				if !ok {
					v = &TvStation{
						Sid:  0,
						Tsid: 0,
						Ch:   0,
						Tsp:  uint(TuningSpaceTS),
						Name: key.Name(),
					}
				}

				val, err := key.Uint()
				if err != nil {
					return nil, err
				}
				if val == 0 {
					continue
				}
				v.Ch = val
				sta[key.Name()] = v
			}
		}
	}

	src := p.stations.GetStationList()
	for key := range sta {
		s := sta[key]
		// service-idが無い場合は登録されている局情報から調べる
		if s.Sid == 0 {
			if s.Ch >= TERRESTRIAL_CHANNEL_FIRST {
				s.Sid = p.stations.GetServiceIDByChannelCode(s.Ch - TERRESTRIAL_CHANNEL_FIRST)
			}
		} else {
			val := s.Ch
			tsid := val & 0xffff
			if tsid == 0 {
				for _, st := range src {
					if st.Sid&0xffff == s.Sid&0xffff {
						s.Tsp = st.Tsp
						s.Tsid = st.Tsid
						break
					}
				}
				if s.Tsid == 0 {
					s.Tsp = 0
				}
			} else {
				s.Tsid = tsid
				if tsid >= 0x6000 {
					s.Tsp = uint(TuningSpaceCS)
				} else if tsid >= 0x4000 {
					s.Tsp = uint(TuningSpaceBS)
				}
			}
		}
	}

	return sta, nil
}

func addGaiji(code uint64) string {
	str := ""
	var b uint64 = 0x01
	for n := 0; n < 32; n++ {
		if (code & b) != 0 {
			str += gaijiCode[n]
		}
		b = b << 1
	}
	return str
}

// runTimeMachineStationsはキャンセル時を含めすべてのワーカーの完了を待ってから戻ります
func runTimeMachineStations(ctx context.Context, stations map[string]*TvStation, scan func(context.Context, *TvStation) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan *TvStation)
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error
	for range min(len(stations), runtime.GOMAXPROCS(0), timeMachineMaxWorkers) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sta := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := scan(ctx, sta); err != nil {
					once.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
			}
		}()
	}
submit:
	for _, sta := range stations {
		select {
		case jobs <- sta:
		case <-ctx.Done():
			break submit
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func ScanTvProgram(ctx context.Context, tmpath string, stations map[string]*TvStation) error {
	return (&timeMachineImporter{programs: &tvprogram}).scanTvProgram(ctx, tmpath, stations)
}

func (p *timeMachineImporter) scanTvProgram(ctx context.Context, tmpath string, stations map[string]*TvStation) error {
	// SQLiteへの書き込みは直列化される一方、ステーションの読み取り処理や解析処理は並行して実行される
	writeSlot := make(chan struct{}, 1)
	var progress atomic.Int64
	return runTimeMachineStations(ctx, stations, func(ctx context.Context, sta *TvStation) error {
		n := progress.Add(1)
		fmt.Printf("info: 局情報の読み込み %d:%s (%d/%d)\n", sta.Sid, sta.Name, n, len(stations))
		return p.scanStation(ctx, tmpath, sta, writeSlot)
	})
}

func (p *timeMachineImporter) scanStation(ctx context.Context, tmpath string, sta *TvStation, writeSlot chan struct{}) error {
	if sta.Sid == 0 || sta.Tsp == 0 {
		log.Println("info: サービスが判別できないためスキップ", sta.Name)
		return nil
	}

	path := filepath.Join(tmpath, sta.Name)
	f, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if !f.IsDir() {
		return nil
	}
	files, err := os.ReadDir(path)
	if err != nil {
		log.Println("info: ファイルが見つからないためスキップ", sta.Name)
		return nil
	}

	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if file.IsDir() {
			continue
		}

		fname := filepath.Join(path, file.Name())
		fp, err := os.Open(fname)
		if err != nil {
			log.Println("error: file open error:", fname)
			continue
		}

		batch := make([]TvProgram, 0, timeMachineBatchSize)
		flush := func() error {
			if len(batch) == 0 {
				return ctx.Err()
			}
			select {
			case writeSlot <- struct{}{}:
			case <-ctx.Done():
				return ctx.Err()
			}
			defer func() { <-writeSlot }()
			if err := p.programs.ReplaceBatchContext(ctx, 1, batch); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return err
			}
			batch = batch[:0]
			return nil
		}

		no, scanErr := scanTimeMachineProgramsTo(ctx, fp, sta, func(tvp *TvProgram) error {
			batch = append(batch, *tvp)
			if len(batch) == timeMachineBatchSize {
				return flush()
			}
			return nil
		})
		closeErr := fp.Close()
		if scanErr != nil {
			return scanErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err := flush(); err != nil {
			return err
		}
		fmt.Printf("info: 番組情報の読み取り %d:%d:%s/%s - %dlines\n", sta.Tsp, sta.Sid, sta.Name, file.Name(), no)
	}

	return ctx.Err()
}

// scanTimeMachineProgramsはデータベースを更新するたびにキャンセルが行われていないかを確認します
func scanTimeMachinePrograms(ctx context.Context, reader io.Reader, sta *TvStation) (int, error) {
	return scanTimeMachineProgramsTo(ctx, reader, sta, func(tvp *TvProgram) error {
		return tvprogram.ReplaceContext(ctx, 1, tvp)
	})
}

func scanTimeMachineProgramsTo(ctx context.Context, reader io.Reader, sta *TvStation, write func(*TvProgram) error) (int, error) {
	if err := ctx.Err(); err != nil {
		return 1, err
	}
	scanner := bufio.NewScanner(transform.NewReader(reader, japanese.ShiftJIS.NewDecoder()))
	no := 1
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return no, err
		}
		words := strings.Split(scanner.Text(), ",")
		l := len(words)
		if l < 7 {
			continue
		}
		var tvp TvProgram
		tvp.Sid = (sta.Tsp << 16) | (sta.Sid & 0xffff)
		tvp.Start, _ = strconv.ParseInt(words[0], 10, 64)
		tvp.End, _ = strconv.ParseInt(words[1], 10, 64)
		gaiji, _ := strconv.ParseUint(words[2], 10, 64)
		eid, _ := strconv.ParseUint(words[3], 10, 64)
		cniba, _ := strconv.ParseUint(words[4], 10, 64)
		cnibb, _ := strconv.ParseUint(words[5], 10, 64)
		//				tvp.Gaiji = uint(gaiji)
		tvp.Eid = uint(eid)
		tvp.CnibA = uint(cniba)
		tvp.CnibB = uint(cnibb)
		tvp.Title = words[6] + addGaiji(gaiji)
		if l >= 8 {
			tvp.Subtitle = words[7]
		} else {
			tvp.Subtitle = ""
		}
		if l >= 9 {
			tvp.Content = strings.ReplaceAll(words[8], "+", "\r\n")
		} else {
			tvp.Content = ""
		}

		if err := write(&tvp); err != nil {
			if ctx.Err() != nil {
				return no, ctx.Err()
			}
			return no, err
		}

		no++
	}
	if err := scanner.Err(); err != nil {
		return no, err
	}

	return no, ctx.Err()
}
