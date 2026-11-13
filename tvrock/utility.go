package tvrock

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

const maxRecordFilePathLength = 240

var recordFileNameReplacer = strings.NewReplacer(
	"<", "＜",
	">", "＞",
	":", "：",
	"\"", "”",
	"/", "／",
	"\\", "￥",
	"|", "｜",
	"?", "？",
	"*", "＊",
)

var gaijiReplace = strings.NewReplacer(
	"🅊", "[HV]",
	"🅌", "[SD]",
	"🄿", "[P]",
	"🅆", "[W]",
	"🅋", "[MV]",
	"🈐", "[手]",
	"🈑", "[字]",
	"🈒", "[双]",
	"🈓", "[デ]",
	"🈔", "[二]",
	"🈕", "[多]",
	"🈖", "[解]",
	"🅍", "[SS]",
	"🄱", "[Ｂ]",
	"🄽", "[Ｎ]",
	"🈗", "[天]",
	"🈘", "[交]",
	"🈙", "[映]",
	"🈚", "[無]",
	"🈛", "[料]",
	"⚿", "[鍵]",
	"🈜", "[前]",
	"🈝", "[後]",
	"🈞", "[再]",
	"🈟", "[新]",
	"🈡", "[終]",
	"🈢", "[生]",
	"🅂", "[Ｓ]",
	"🈤", "[声]",
	"🈠", "[初]",
	"🅎", "[PPV]",
	"🈦", "[演]",
)

var gaijiRemove = strings.NewReplacer(
	"🅊", "",
	"🅌", "",
	"🄿", "",
	"🅆", "",
	"🅋", "",
	"🈐", "",
	"🈑", "",
	"🈒", "",
	"🈓", "",
	"🈔", "",
	"🈕", "",
	"🈖", "",
	"🅍", "",
	"🄱", "",
	"🄽", "",
	"🈗", "",
	"🈘", "",
	"🈙", "",
	"🈚", "",
	"🈛", "",
	"⚿", "",
	"🈜", "",
	"🈝", "",
	"🈞", "",
	"🈟", "",
	"🈡", "",
	"🈢", "",
	"🅂", "",
	"🈤", "",
	"🈠", "",
	"🅎", "",
	"🈦", "",
)

const (
	FilenameExTT       = "@TT"  // 番組タイトル
	FilenameExSB       = "@SB"  // サブタイトル
	FilenameExSBE      = "@sb"  // サブタイトル(スペース無し)
	FilenameExCH       = "@CH"  // 局名
	FilenameExDR       = "@DR"  // 番組の放送時間(分)
	FilenameExHashTagK = "@HTK" // ハッシュタグ(キーワード)
	FilenameExHashTagS = "@HTS" // ハッシュタグ(局)
	FilenameExYY       = "@YY"  // ２４時間表記・年
	FilenameEx2YY      = "@2YY" // ２４時間表記・年（下２桁）
	FilenameExMM       = "@MM"  // ２４時間表記・月
	FilenameExDD       = "@DD"  // ２４時間表記・日
	FilenameExSH       = "@SH"  // ２４時間表記・開始時間・時
	FilenameExSM       = "@SM"  // ２４時間表記・開始時間・分
	FilenameExSS       = "@SS"  // ２４時間表記・開始時間・秒
	FilenameExWK       = "@WK"  // ２４時間表記・開始時間・曜日
	FilenameExWE       = "@WE"  // ２４時間表記・開始時間・曜日・英語表記
	FilenameExEH       = "@EH"  // ２４時間表記・終了時間・時
	FilenameExEM       = "@EM"  // ２４時間表記・終了時間・分
	FilenameExES       = "@ES"  // ２４時間表記・終了時間・秒
	FilenameExyy       = "@yy"  // ２８時間表記・年
	FilenameEx2yy      = "@2yy" // ２８時間表記・年（下２桁）
	FilenameExmm       = "@mm"  // ２８時間表記・月
	FilenameExdd       = "@dd"  // ２８時間表記・日
	FilenameExsh       = "@sh"  // ２８時間表記・開始時間・時
	FilenameExsm       = "@sm"  // ２８時間表記・開始時間・分
	FilenameExss       = "@ss"  // ２８時間表記・開始時間・秒
	FilenameExwk       = "@wk"  // ２８時間表記・開始時間・曜日
	FilenameExwe       = "@we"  // ２８時間表記・開始時間・曜日・英語表記
	FilenameExeh       = "@eh"  // ２８時間表記・終了時間・時
	FilenameExem       = "@em"  // ２８時間表記・終了時間・分
	FilenameExes       = "@es"  // ２８時間表記・終了時間・秒
	FilenameExEC       = "@EC"  // 録画エラー数
	FilenameExRD       = "@RD"  // 録画ファイルフォルダ
	FilenameExRF       = "@RF"  // 録画ファイルパス（カスタムコマンドのみ）
	FilenameExRN       = "@RN"  // 録画ファイル名（カスタムコマンドのみ）
)

var FilenameExCommands = []string{
	FilenameExTT,
	FilenameExSB,
	FilenameExSBE,
	FilenameExCH,
	FilenameExDR,
	FilenameExHashTagK,
	FilenameExHashTagS,
	FilenameExYY,
	FilenameEx2YY,
	FilenameExMM,
	FilenameExDD,
	FilenameExSH,
	FilenameExSM,
	FilenameExSS,
	FilenameExWK,
	FilenameExWE,
	FilenameExEH,
	FilenameExEM,
	FilenameExES,
	FilenameExyy,
	FilenameEx2yy,
	FilenameExmm,
	FilenameExdd,
	FilenameExsh,
	FilenameExsm,
	FilenameExss,
	FilenameExwk,
	FilenameExwe,
	FilenameExeh,
	FilenameExem,
	FilenameExes,
	FilenameExEC,
	FilenameExRD,
	FilenameExRF,
	FilenameExRN,
}

var weekName = []string{
	"日",
	"月",
	"火",
	"水",
	"木",
	"金",
	"土",
}

var CurrentSlot int = 0
var broadcastDelay time.Duration = 0

func Now() time.Time {
	return time.Now().Add(broadcastDelay)
}

func JsonPrint(p interface{}) string {
	str := fmt.Sprintf("%T ", p)
	if jsonBytes, jsonErr := json.MarshalIndent(p, "\t", "  "); jsonErr != nil {
		str = str + jsonErr.Error()
	} else {
		str = str + string(jsonBytes)
	}
	return str
}

func now28() time.Time {
	tm := Now()
	if tm.Hour() < 4 {
		tm = tm.AddDate(0, 0, -1)
	}
	return tm
}

func time28(uxtm int64) time.Time {
	tm := time.Unix(uxtm, 0)
	if tm.Hour() < 4 {
		tm = tm.AddDate(0, 0, -1)
	}
	return tm
}

func time28hour(uxtm int64) int {
	tm := time.Unix(uxtm, 0)
	hour := tm.Hour()
	if hour < 4 {
		hour = hour + 24
	}
	return hour
}

func pathRemoveFileSpec(path string) string {
	path = strings.TrimSuffix(path, "\\")
	path = strings.TrimSuffix(path, "/")
	return path
}

func CreateIniTmp(src string, dst string) error {
	//読み込みファイル準備
	file1, err := os.Open(src)
	if err != nil {
		return err
	}
	defer file1.Close()

	//書き込みファイル準備
	file2, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer file2.Close()

	reader := transform.NewReader(file1, japanese.ShiftJIS.NewDecoder())
	if _, err := io.Copy(file2, reader); err != nil {
		return err
	}
	return file2.Close()
}

func ContainsStr(arr []string, str string) bool {
	for _, v := range arr {
		if v == str {
			return true
		}
	}
	return false
}

func ContainsInt(arr []int, val int) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}

func ContainsUint(arr []uint, val uint) bool {
	for _, v := range arr {
		if v == val {
			return true
		}
	}
	return false
}

func joinUintToString(ar []uint, sp string) string {
	str := []string{}
	for _, v := range ar {
		str = append(str, fmt.Sprintf("%d", v))
	}
	return strings.Join(str, sp)
}

func buildHashTag(hashtag string) string {
	tags := strings.Split(hashtag, ",")
	var sb strings.Builder
	for i, v := range tags {
		if i > 0 {
			sb.WriteString(" ")
		}
		sb.WriteString("#")
		sb.WriteString(v)
	}
	return sb.String()
}

// ファイル名置換
func FilenameEx(src, title, did, recpath string, sid, eid, errct uint, htk, hts string) (string, error) {
	ex := src
	if ex == "" {
		return ex, nil
	}

	var prg []*TvProgram
	var err error
	now := Now()
	programTokens := []string{FilenameExSB, FilenameExSBE, FilenameExDR,
		FilenameExSH, FilenameExSM, FilenameExSS, FilenameExWK, FilenameExWE,
		FilenameExEH, FilenameExEM, FilenameExES, FilenameExsh, FilenameExsm,
		FilenameExss, FilenameExwk, FilenameExwe, FilenameExeh, FilenameExem, FilenameExes}
	needsProgram := title == "" && strings.Contains(src, FilenameExTT)
	for _, token := range programTokens {
		needsProgram = needsProgram || strings.Contains(src, token)
	}
	if needsProgram {
		prg, err = tvprogram.GetWithEID(CurrentSlot, sid, eid)
		if err != nil {
			return src, err
		}
	}
	for _, v := range FilenameExCommands {
		if !strings.Contains(ex, v) {
			continue
		}
		switch v {
		case FilenameExTT:
			if title == "" && len(prg) > 0 {
				ex = strings.Replace(ex, v, prg[0].Title, -1)
			} else {
				ex = strings.Replace(ex, v, title, -1)
			}
		case FilenameExSB:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, " "+prg[0].Subtitle, -1)
			}
		case FilenameExSBE:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, prg[0].Subtitle, -1)
			}
		case FilenameExCH:
			staname := ""
			sta, ext := station.Get(sid)
			if ext {
				staname = sta.Name
			}
			ex = strings.Replace(ex, v, staname, -1)
		case FilenameExDR:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, strconv.Itoa(int((prg[0].End-prg[0].Start)/60)), -1)
			}
		case FilenameExHashTagK:
			ex = strings.Replace(ex, v, buildHashTag(htk), -1)
		case FilenameExHashTagS:
			ex = strings.Replace(ex, v, buildHashTag(hts), -1)
		case FilenameExYY:
			ex = strings.Replace(ex, v, strconv.Itoa(now.Year()), -1)
		case FilenameEx2YY:
			ex = strings.Replace(ex, v, strconv.Itoa(now.Year()%100), -1)
		case FilenameExMM:
			ex = strings.Replace(ex, v, strconv.Itoa(int(now.Month())), -1)
		case FilenameExDD:
			ex = strings.Replace(ex, v, strconv.Itoa(now.Day()), -1)
		case FilenameExSH:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time.Unix(prg[0].Start, 0).Hour()), -1)
			}
		case FilenameExSM:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time.Unix(prg[0].Start, 0).Minute()), -1)
			}
		case FilenameExSS:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time.Unix(prg[0].Start, 0).Second()), -1)
			}
		case FilenameExWK:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, weekName[int(time.Unix(prg[0].Start, 0).Weekday())], -1)
			}
		case FilenameExWE:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, time.Unix(prg[0].Start, 0).Weekday().String(), -1)
			}
		case FilenameExEH:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time.Unix(prg[0].End, 0).Hour()), -1)
			}
		case FilenameExEM:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time.Unix(prg[0].End, 0).Minute()), -1)
			}
		case FilenameExES:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time.Unix(prg[0].End, 0).Second()), -1)
			}
		case FilenameExyy:
			ex = strings.Replace(ex, v, strconv.Itoa(now28().Year()), -1)
		case FilenameEx2yy:
			ex = strings.Replace(ex, v, strconv.Itoa(now28().Year()%100), -1)
		case FilenameExmm:
			ex = strings.Replace(ex, v, strconv.Itoa(int(now28().Month())), -1)
		case FilenameExdd:
			ex = strings.Replace(ex, v, strconv.Itoa(now28().Day()), -1)
		case FilenameExsh:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time28hour(prg[0].Start)), -1)
			}
		case FilenameExsm:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time28(prg[0].Start).Minute()), -1)
			}
		case FilenameExss:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time28(prg[0].Start).Second()), -1)
			}
		case FilenameExwk:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, weekName[int(time28(prg[0].Start).Weekday())], -1)
			}
		case FilenameExwe:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, time28(prg[0].Start).Weekday().String(), -1)
			}
		case FilenameExeh:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time28hour(prg[0].End)), -1)
			}
		case FilenameExem:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time28(prg[0].End).Minute()), -1)
			}
		case FilenameExes:
			if len(prg) > 0 {
				ex = strings.Replace(ex, v, fmt.Sprintf("%02d", time28(prg[0].End).Second()), -1)
			}
		case FilenameExRD:
			dir, _ := filepath.Split(recpath)
			ex = strings.Replace(ex, v, dir, -1)
		case FilenameExRF:
			ex = strings.Replace(ex, v, recpath, -1)
		case FilenameExRN:
			_, fname := filepath.Split(recpath)
			ex = strings.Replace(ex, v, fname, -1)
		case FilenameExEC:
			ex = strings.Replace(ex, v, fmt.Sprintf("%02d", errct), -1)
		}
	}

	return ex, err
}

func SanitizeRecordFilePath(src, recordPath, title, extension, did string, sid, eid uint) string {
	hts := ""
	sta, ext := station.Get(sid)
	if ext {
		hts = sta.HashTag
	}

	htk := ""
	sch, b := scheduler.GetCurrentRecording(did)
	if b && sch.SearchID > 0 {
		kr, err := rockDB.GetKeywordReserve(CurrentSlot, uint64(sch.SearchID))
		if err != nil {
			log.Println("error:", err)
			return ""
		}
		htk = kr.HashTag
	}

	name, _ := FilenameEx(src, title, did, recordPath, sid, eid, 0, htk, hts)

	name = recordFileNameReplacer.Replace(name)
	switch CurrentRecorderSetting().GaijiExclusion {
	case GaijiExclusionReplace:
		name = gaijiReplace.Replace(name)
	case GaijiExclusionDelete:
		name = gaijiRemove.Replace(name)
	}

	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '　'
		}
		return r
	}, name)

	name = strings.TrimRight(name, " .")
	if len(name) == 0 {
		name = "無題"
	}

	prefix := recordPath + "/"
	maxNameLength := maxRecordFilePathLength - len([]rune(prefix+extension))
	if maxNameLength < 1 {
		maxNameLength = 1
	}
	if runes := []rune(name); len(runes) > maxNameLength {
		name = strings.TrimRight(string(runes[:maxNameLength]), " .")
		if len(name) == 0 {
			name = "無題"
		}
	}

	return prefix + name + extension
}

func ReceiverError(ctx context.Context, err error) {
	if websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
	) {
		log.Printf("info: クライアントの正常切断: %v\n", err)
		return
	}

	if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		log.Printf("warn: クライアントとの接続が異常切断: %v\n", err)
		return
	}

	if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
		log.Printf("warn: ネットワークエラーまたは強制リセットによる切断: %v\n", err)
		return
	}

	select {
	case <-ctx.Done():
		log.Println("info: 終了通知を検出")
	default:
		log.Println("warn:", err)
	}
}
