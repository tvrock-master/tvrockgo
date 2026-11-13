package tvrock

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	yaml "gopkg.in/yaml.v2"
)

type GaijiExclusionType uint8

const (
	GaijiExclusionNone GaijiExclusionType = iota
	GaijiExclusionReplace
	GaijiExclusionDelete
)

const (
	TvRockConfigureOriginalFile = "conf/tvrock.org.yml"
	TvRockConfigureFile         = "conf/tvrock.yml"
)

type RecorderSetting struct {
	FilenameEx              string             `yaml:"filenameex" json:"filenameex"`
	RecordFileExtension     string             `yaml:"recordfileextension" json:"recordfileextension"`
	GaijiExclusion          GaijiExclusionType `yaml:"gaijiexclusion" json:"gaijiexclusion"`
	AnalyzeTSAfterRecording bool               `yaml:"analyzetsafterrecording" json:"analyzetsafterrecording"`
}

type TSCMBoundaryRule struct {
	SID                 uint   `yaml:"sid"`
	PID                 uint16 `yaml:"pid"`
	NoPacketSeconds     int    `yaml:"nopacketseconds"`
	StartForwardPackets int    `yaml:"startforwardpackets"`
	EndBackPackets      int    `yaml:"endbackpackets"`
}

type TaskSetting struct {
	MaxTasks int   `yaml:"maxtasks" json:"maxtasks"`
	ExecTime int64 `yaml:"exectime" json:"exectime"`
}

type RemoteWatchSetting struct {
	MaxSessions               int `yaml:"maxsessions"`
	KeepBeforeScheduleSeconds int `yaml:"keepbeforescheduleseconds"`
	DisconnectGraceSeconds    int `yaml:"disconnectgraceseconds"`
	StartupTimeoutSeconds     int `yaml:"startuptimeoutseconds"`
}

type FetchProgram struct {
	ScheduleTS bool `yaml:"schedulets"`
	ScheduleBS bool `yaml:"schedulebs"`
	ScheduleCS bool `yaml:"schedulecs"`
	ScheduleOS bool `yaml:"scheduleos"`

	TimeoutTS int `yaml:"timeoutts"`
	TimeoutBS int `yaml:"timeoutbs"`
	TimeoutCS int `yaml:"timeoutcs"`
	TimeoutOS int `yaml:"timeoutos"`

	ScheduleDuration int   `yaml:"scheduleduration"`
	ScheduleWindow   int   `yaml:"schedulewindow"`
	ScheduleHours    []int `yaml:"schedulehours"`
	SchedulerPeriod  int   `yaml:"schedulerperiod"`
	SearchStep       int   `yaml:"searchstep"`

	DeviceOrderTS []string `yaml:"deviceorderts"`
	DeviceOrderBS []string `yaml:"deviceorderbs"`
	DeviceOrderCS []string `yaml:"deviceordercs"`
	DeviceOrderOS []string `yaml:"deviceorderos"`
}

// 電源管理
type PowerManagerSetting struct {
	PreventSleep           bool   `yaml:"preventsleep" json:"preventsleep"`
	PowerManager           bool   `yaml:"powermanager" json:"powermanager"`
	SleepTimer             int64  `yaml:"sleeptimer" json:"sleeptimer"`
	SleepTimerPause        bool   `yaml:"sleeptimerpause" json:"sleeptimerpause"`
	PreventIfDeviceRunning bool   `yaml:"preventifdevicerunning" json:"preventifdevicerunning"`
	PreventSleepTime       int64  `yaml:"preventsleeptime" json:"preventsleeptime"`
	PreventDisplayOff      bool   `yaml:"preventdisplayoff" json:"preventdisplayoff"`
	WakeTaskName           string `yaml:"waketaskname" json:"waketaskname"`
	WakeBeforeSchedule     int64  `yaml:"wakebeforeschedule" json:"wakebeforeschedule"`
	WakeUpSuppressDuration int64  `yaml:"wakeupsuppressduration" json:"wakeupsuppressduration"`
}

// 設定ファイル
type TvRockConf struct {
	Host                 string              `yaml:"host"`
	DBFile               string              `yaml:"dbfile"`
	ProgramDBFile        string              `yaml:"programdbfile"`
	PastDBFile           string              `yaml:"pastdbfile"`
	CaptionDBFile        string              `yaml:"captiondbfile"`
	ArchiveDBFile        string              `yaml:"archivedbfile"`
	CaptionArchiveDBFile string              `yaml:"captionarchivedbfile"`
	BleveFile            string              `yaml:"blevefile"`
	RecorderSetting      RecorderSetting     `yaml:"recordersetting"`
	TSCMBoundaryRules    []TSCMBoundaryRule  `yaml:"tscmboundaryrules"`
	TaskSetting          TaskSetting         `yaml:"tasksetting"`
	PowerManagerSetting  PowerManagerSetting `yaml:"powermanagersetting"`
	RemoteWatch          RemoteWatchSetting  `yaml:"remotewatch"`
	ArchiverSetting      ArchiverSetting     `yaml:"archiversetting"`

	LauncherHostID         string          `yaml:"launcherhostid"`
	LauncherToken          string          `yaml:"launchertoken" json:"-"`
	BootWebServer          bool            `yaml:"bootwebserver"`
	BootLauncher           bool            `yaml:"bootlauncher"`
	BootScheduler          bool            `yaml:"bootscheduler"`
	BootArchiver           bool            `yaml:"bootarchiver"`
	Bleve                  bool            `yaml:"bleve"`
	CaptionRecord          bool            `yaml:"captionrecord"`
	AdjustBCTime           bool            `yaml:"adjustbctime"`
	IsAfterWithMargin      bool            `yaml:"isafterwithmargin"`
	AdjustTimeThreshold    int64           `yaml:"adjusttimethreshold"`
	AllowOrigins           []string        `yaml:"allowOrigins"`
	AllowOriginPrefix      []string        `yaml:"allowOriginPrefix"`
	Devices                []RockDevice    `yaml:"devices"`
	PlaybackDevices        []RockDevice    `yaml:"playbackdevices"`
	TsidList               []FrequencyTSID `yaml:"tsidlist"`
	DeviceRequestQueueSize int             `yaml:"devicerequestqueuesize"`
	DeviceBootupTimeout    int             `yaml:"devicebootuptimeout"`
	DeviceShutdownTime     int             `yaml:"deviceshutdowntime"`
	SearchMaxWorkerThreads int             `yaml:"searchmaxworkerthreads"`
	Debug                  bool            `yaml:"debug"`

	Authentication       bool              `yaml:"authentication"`
	UserAccount          map[string]string `yaml:"useraccount"`
	CookieDomain         string            `yaml:"cookiedomain"`
	CookieName           string            `yaml:"cookiename"`
	CookiePath           string            `yaml:"cookiepath"`
	SessionName          string            `yaml:"sessionname"`
	SessionSecretKey     string            `yaml:"sessionsecretkey"`
	SessionMaxAge        int               `yaml:"sessionmaxage"`
	SessionMaxLength     int               `yaml:"sessionmaxlength"`
	SessionSecure        bool              `yaml:"sessionsecure"`
	SessionHTTPOnly      bool              `yaml:"sessionhttponly"`
	SearchLimit          int               `yaml:"searchlimit"`
	FrontendStationLimit int               `yaml:"frontendstationlimit"`
	SqliteSetting        []string          `yaml:"sqlitesetting"`
	CronTask             map[string]string `yaml:"crontask"`

	CurrentProgramThreshold        int64           `yaml:"currentprogramthreshold"`
	CurrentKeywordReserveThreshold int64           `yaml:"currentkeywordreservethreshold"`
	CurrentCaptionThreshold        int64           `yaml:"currentcaptionthreshold"`
	RecordingDevice                RecordingDevice `yaml:"recordingdevice"`

	MaxConnection     int           `yaml:"maxconnection"`
	WriteTimeout      time.Duration `yaml:"writetimeout"`
	ReadTimeout       time.Duration `yaml:"readtimeout"`
	ReadHeaderTimeout time.Duration `yaml:"readHeadertimeout"`
	IdleTimeout       time.Duration `yaml:"idletimeout"`

	FetchProgram FetchProgram `yaml:"fetchprogram"`

	StorageCommand CustomCommand   `yaml:"storagecommand"`
	CustomCommand  []CustomCommand `yaml:"customcommand"`

	Features      []FeaturePattern `yaml:"features"`
	ServiceGroups []ServiceGroup   `yaml:"servicegroups"`

	mux sync.RWMutex `yaml:"-"`
}

type FeaturePattern struct {
	Name    string `yaml:"name" json:"name"`
	Pattern string `yaml:"pattern" json:"pattern"`
}

var defaultFeatures = []FeaturePattern{
	{Name: "new", Pattern: `.*(🈟|\[新\])|（新）|\(新\)|＜新＞.*`},
	{Name: "end", Pattern: `.*(🈡|\[終\])|（終）|\(終\)|＜終＞.*`},
	{Name: "first", Pattern: `.*第(1|一)回|第(1|一)話|第(1|一)幕|#0*1($|^[0-9])|（初）|🈠|No\.0*1($|^[0-9])`},
	{Name: "free", Pattern: `.*(🈚|\[無\]\[無料\]).*`},
}

func normalizeFeatures(conf *TvRockConf) {
	if len(conf.Features) == 0 {
		conf.Features = make([]FeaturePattern, len(defaultFeatures))
		copy(conf.Features, defaultFeatures)
	}
}

var configure TvRockConf

func normalizeFetchProgramScheduleHours(hours []int) []int {
	seen := make([]bool, 24)
	for _, hour := range hours {
		if hour >= 0 && hour <= 23 {
			seen[hour] = true
		}
	}

	ret := []int{}
	for hour, ok := range seen {
		if ok {
			ret = append(ret, hour)
		}
	}
	return ret
}

func normalizeRecorderSetting(setting *RecorderSetting) {
	if setting.RecordFileExtension == "" {
		setting.RecordFileExtension = ".ts"
	}
	if setting.GaijiExclusion > GaijiExclusionDelete {
		setting.GaijiExclusion = GaijiExclusionNone
	}
}

func (p *TvRockConf) BuildHash() bool {
	p.mux.Lock()
	defer p.mux.Unlock()

	changed := false
	for i := 0; i < len(p.Devices); i++ {
		if len(p.Devices[i].RegistryCode) > 0 {
			continue
		}
		md5 := md5.Sum([]byte(p.Devices[i].ID + ":" + strconv.FormatInt(Now().UnixNano(), 10)))
		p.Devices[i].RegistryCode = hex.EncodeToString(md5[:])
		changed = true
	}

	if p.SessionSecretKey == "" || p.SessionSecretKey == "secret-key" {
		p.SessionSecretKey = generateSessionSecretKey()
		changed = true
	}

	for uid, password := range p.UserAccount {
		if password == "" || isPasswordHash(password) {
			continue
		}
		p.UserAccount[uid] = generatePasswordHash(password)
		changed = true
	}

	return changed
}

func generateSessionSecretKey() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(buf)
}

// 内蔵ランチャーの資格情報をサービス起動前に永続化する。
// 保存失敗時は生成値を破棄し呼び出し元へエラーを返す。
func (p *TvRockConf) ensureLauncherToken(path string) error {
	p.mux.Lock()
	defer p.mux.Unlock()
	if p.LauncherToken != "" {
		if !validLauncherToken(p.LauncherToken) {
			return fmt.Errorf("launchertoken must be 64 hexadecimal characters")
		}
		return nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("generate launcher token: %w", err)
	}
	p.LauncherToken = hex.EncodeToString(buf)
	if err := p.saveLocked(path); err != nil {
		p.LauncherToken = ""
		return fmt.Errorf("save launcher token: %w", err)
	}
	return nil
}

func generatePasswordHash(password string) string {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	return string(hash)
}

func isPasswordHash(password string) bool {
	if !strings.HasPrefix(password, "$2a$") &&
		!strings.HasPrefix(password, "$2b$") &&
		!strings.HasPrefix(password, "$2y$") {
		return false
	}
	_, err := bcrypt.Cost([]byte(password))
	return err == nil
}

func (p *TvRockConf) Load(fpath string) error {
	log.Println("info: 設定ファイルの読み込み", fpath)
	p.mux.Lock()
	defer p.mux.Unlock()

	buf, err := os.ReadFile(fpath)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(buf, p); err != nil {
		return err
	}
	normalizeFeatures(p)
	normalizeRemoteWatchSetting(&p.RemoteWatch)
	if err := validateTSCMBoundaryRules(p.TSCMBoundaryRules); err != nil {
		return err
	}
	return validateServiceGroups(p.ServiceGroups)
}

func validateTSCMBoundaryRules(rules []TSCMBoundaryRule) error {
	seen := make(map[uint]bool)
	for i, rule := range rules {
		if rule.SID == 0 || uint64(rule.SID) > 0xffffffff || rule.PID == 0 || rule.PID >= 8191 ||
			rule.NoPacketSeconds < 1 || rule.NoPacketSeconds > 86400 || rule.StartForwardPackets < 0 || rule.StartForwardPackets > 65535 || rule.EndBackPackets < 0 || rule.EndBackPackets > 65535 {
			return fmt.Errorf("tscmboundaryrules[%d]: sidは1～4294967295、pidは1～8190、nopacketsecondsは1～86400、startforwardpacketsとendbackpacketsは0～65535で指定してください", i)
		}
		if seen[rule.SID] {
			return fmt.Errorf("tscmboundaryrules[%d]: SID %d が重複しています", i, rule.SID)
		}
		seen[rule.SID] = true
	}
	return nil
}

func currentTSCMBoundaryRule(sid uint) (TSCMBoundaryRule, bool) {
	configure.mux.RLock()
	defer configure.mux.RUnlock()
	for _, rule := range configure.TSCMBoundaryRules {
		if rule.SID == sid {
			return rule, true
		}
	}
	// 録画ログの上位16bitは放送種別。通常のサービスIDは下位16bitで照合する。
	// 放送種別込みのルールがあれば、上の完全一致を優先する。
	for _, rule := range configure.TSCMBoundaryRules {
		if rule.SID <= 0xffff && rule.SID == sid&0xffff {
			return rule, true
		}
	}
	return TSCMBoundaryRule{}, false
}

func (p *TvRockConf) Save(fpath string) error {
	p.mux.RLock()
	defer p.mux.RUnlock()
	return p.saveLocked(fpath)
}

// saveLockedを呼び出すには呼び出し元がmuxを保持している必要がある。設定の更新は永続化されるか
// あるいはロールバックされる。その際、読み取り側が失敗した途中の状態を目にすることはない。
func (p *TvRockConf) saveLocked(fpath string) error {
	log.Println("info: 設定ファイルの保存", fpath)
	buf, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(fpath, buf, 0600)
}

func CurrentRecorderSetting() RecorderSetting {
	configure.mux.RLock()
	defer configure.mux.RUnlock()
	return configure.RecorderSetting
}

func updateRecorderSetting(setting RecorderSetting) (RecorderSetting, error) {
	normalizeRecorderSetting(&setting)
	configure.mux.Lock()
	previous := configure.RecorderSetting
	configure.RecorderSetting = setting
	if err := configure.saveLocked(TvRockConfigureFile); err != nil {
		configure.RecorderSetting = previous
		configure.mux.Unlock()
		return previous, err
	}
	configure.mux.Unlock()
	deviceController.Notify(ControllerUpdateRecorderSetting, "")
	select {
	case tsAnalysisWake <- struct{}{}:
	default:
	}
	return setting, nil
}
