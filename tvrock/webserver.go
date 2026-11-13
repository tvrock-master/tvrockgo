package tvrock

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/easonlin404/limit"
	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

const (
	HEADER_CLIENT_TVROCK_LAUNCHER         = "TvRockLauncher"
	HEADER_CLIENT_TVROCK_APP              = "TvRockApp"
	HEADER_CLIENT_TVROCK_DTV              = "TvRockDTV"
	HEADER_CLIENT_TVROCK_DTV_SIBLING      = "TvRockDTV-Sib"
	HEADER_CLIENT_TVROCK_ROCKBAR          = "TvRockRockbar"
	HEADER_CLIENT_TVROCK_LAUNCHER_HOST_ID = "X-TvRock-Launcher-ID"
	HEADER_CLIENT_TVROCK_CLIENT           = "X-TvRock-Client"
	HEADER_CLIENT_TVROCK_DTV_PARENT       = "X-TvRockDTV-Parent"
)

type TvpParam struct {
	Services []uint `json:"services"`
	Start    int64  `json:"start"`
	Hours    int    `json:"hours"`
	Week     bool   `json:"week"`
}

type ReserveParam struct {
	ID    uint  `json:"id"`
	Sid   uint  `json:"sid"`
	Eid   uint  `json:"eid"`
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Watch bool  `json:"watch"`
}

type SearchParam struct {
	RequestID     string `json:"requestId"` // ブラウザー内の検索世代（省略時は従来形式）
	UID           string `json:"uid"`       // リザルト用websocket uid
	Title         string `json:"title"`     // タイトル
	Content       string `json:"content"`   // 内容
	Start         int64  `json:"start"`
	Days          int64  `json:"days"`
	Sid           []uint `json:"sid"`
	Category      uint   `json:"category"`
	Week          uint8  `json:"week"`
	AndSearch     bool   `json:"andsearch"`
	FuzzySearch   *bool  `json:"fuzzysearch"`
	ArchiveSearch bool   `json:"archivesearch"`
	CaptionSearch bool   `json:"captionsearch"`
	StartHours    int64  `json:"starthours"`
	Hours         int64  `json:"hours"`
	StartTime     string `json:"starttime"`
	EndTime       string `json:"endtime"`
}

type SearchResult struct {
	Hits int `json:"hits"` //リザルト数
}

type SearchProgress struct {
	Warning  string `json:"warning,omitempty"`
	Progress bool   `json:"progress"`
	Running  bool   `json:"running"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Current  int64  `json:"current"`
	Percent  int    `json:"percent"`
	Scanned  int    `json:"scanned"`
	Hits     int    `json:"hits"`
}

var server *http.Server
var serverMu sync.Mutex

var apiHeaders = map[string]string{
	"Access-Control-Allow-Credentials": "true",
	//	"Expires":         epoch,
	"Cache-Control":   "no-cache, private, no-store, must-revalidate, max-age=0",
	"Pragma":          "no-cache",
	"X-Accel-Expires": "0",
}

func allowOriginPrefix(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	for _, v := range configure.AllowOriginPrefix {
		allowed, err := url.Parse(v)
		if err != nil || allowed.Scheme != u.Scheme || allowed.Host == "" {
			continue
		}
		host, prefix := strings.ToLower(u.Hostname()), strings.ToLower(allowed.Hostname())
		if allowed.Port() != "" && allowed.Port() != u.Port() {
			continue
		}
		if host == prefix || (strings.HasSuffix(prefix, ".") && net.ParseIP(host) != nil && strings.HasPrefix(host, prefix)) {
			return true
		}
	}
	return false
}

func Service() {
	if err := RunService(nil); err != nil {
		log.Printf("error: HTTP service: %v", err)
	}
}

// RunServiceはリスナーで発生したエラーを報告しポートのバインド完了後に準備が整ったことを通知します。
// Closeメソッドを呼び出す責任は呼び出し側にあります。
func RunService(ready func()) (runErr error) {
	// 無効なテンプレートやテンプレートの欠落については、トレイモードであっても所有者に通知が届くようにしなければならない。
	defer func() {
		if recovered := recover(); recovered != nil {
			runErr = fmt.Errorf("HTTPサービスの初期化に失敗: %v", recovered)
		}
	}()
	if !serviceWorkers.enter() {
		return nil
	}
	defer serviceWorkers.wg.Done()

	// gin release
	if !configure.Debug {
		gin.SetMode(gin.ReleaseMode)
		gin.DefaultWriter = io.Discard // ログを表示しない
	}

	r := gin.Default()

	InitializeSession()

	// session
	store := cookie.NewStore([]byte(configure.SessionSecretKey))
	sessionName := configure.SessionName
	if configure.CookieName != "" {
		sessionName = configure.CookieName
	}
	store.Options(sessions.Options{
		Domain:   sessionCookieDomain(),
		Path:     configure.CookiePath,
		MaxAge:   configure.SessionMaxAge,
		Secure:   configure.SessionSecure,
		HttpOnly: configure.SessionHTTPOnly,
		SameSite: http.SameSiteLaxMode,
	})
	r.Use(sessions.Sessions(sessionName, store))
	r.Use(IsLogin())

	// limit the number of current requests
	r.Use(limit.Limit(configure.MaxConnection))

	// cors
	r.Use(cors.New(cors.Config{
		AllowOrigins:    configure.AllowOrigins,
		AllowOriginFunc: allowOriginPrefix,
		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"DELETE",
		},
		AllowHeaders: []string{
			HEADER_CLIENT_TVROCK_CLIENT,
			"Authorization",
			"Content-Type",
			"Content-Length",
			"Cookie",
		},
		AllowCredentials: true,
		MaxAge:           24 * time.Hour,
	}))

	ex, err := os.Executable()
	if err != nil {
		panic(err)
	}
	exPath := filepath.Dir(ex)
	staticPath := filepath.Join(exPath, "static")

	r.Use(cdtFileHandler(filepath.Join(staticPath, "cdt")))
	r.StaticFS("/static", http.Dir(staticPath))            // 静的ファイル
	r.LoadHTMLGlob(filepath.Join(exPath, "template", "*")) // テンプレートファイル
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/static/index.html")
	})

	r.GET("/login", GetLogin)
	r.POST("/login", PostLogin)
	r.GET("/logout", GetLogout)

	registerAPIRoutes(r.Group("", requireAPIClientIdentity))

	// web socket
	r.GET("/dev", SockDeviceTerminal)
	r.GET("/con", deviceController.SockControllerHandler)
	r.GET("/launcher", launcher.SockLauncherHandler)

	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if !serviceWorkers.enter() {
				http.Error(w, "server shutting down", http.StatusServiceUnavailable)
				return
			}
			defer serviceWorkers.wg.Done()
			r.ServeHTTP(w, req)
		}),
		BaseContext:       func(net.Listener) context.Context { return serviceContext() },
		Addr:              configure.Host,
		WriteTimeout:      time.Duration(configure.WriteTimeout) * time.Millisecond,
		ReadTimeout:       time.Duration(configure.ReadTimeout) * time.Millisecond,
		ReadHeaderTimeout: time.Duration(configure.ReadHeaderTimeout) * time.Millisecond,
		IdleTimeout:       time.Duration(configure.IdleTimeout) * time.Millisecond,
		//		MaxHeaderBytes:    1 << 20,
	}
	serverMu.Lock()
	if serviceContext().Err() != nil {
		serverMu.Unlock()
		return nil
	}
	listener, err := net.Listen("tcp", configure.Host)
	if err != nil {
		serverMu.Unlock()
		return err
	}
	server = httpServer
	serverMu.Unlock()
	defer listener.Close()
	if ready != nil {
		ready()
	}
	if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// LocalWebHostはトレイランチャーがHTTPリッスンを行うアドレスを返す
func LocalWebHost() string { return configure.Host }

func ShutdownService(ctx context.Context) error {
	serverMu.Lock()
	current := server
	serverMu.Unlock()
	if current == nil {
		return nil
	}
	if err := current.Shutdown(ctx); err != nil {
		// Shutdownはタイムアウト時にアクティブな接続をクローズしません。それらのI/Oを停止します。
		// DBのクローズ前に追跡対象のハンドラーおよびWebSocketレシーバーの完了を待ちます。
		if closeErr := current.Close(); closeErr != nil {
			log.Println("error: HTTP close:", closeErr)
		}
		return err
	}
	return nil
}

func registerAPIRoutes(r gin.IRoutes) {
	if configure.Debug {
		r.GET("/debug", APIDebug)
	}

	r.GET("/stl", APIGetStationList)
	r.PUT("/stl", APIUpdateStationList)

	r.PUT("/tvp", APIGetTvProgram)
	r.GET("/tvf", APIGetTvFeature)
	r.GET("/caption", APIGetCaptionHistory)

	r.GET("/sch", APIGetSchedule)
	r.POST("/sch", APIAddSchedule)
	r.PUT("/sch", APIUpdateSchedule)
	r.DELETE("/sch/:id", APIDeleteSchedule)
	r.DELETE("/sch/stop/:id", APIStopSchedule)

	r.GET("/rst", APIGetRecordingStatus)

	r.POST("/src", APIDoSearch)

	r.GET("/kwr/:id", APIGetKeywordReserve)
	r.POST("/kwr", APIAddKeywordReserve)
	r.POST("/kwr/:id/schedule", APIAddKeywordReserveSchedule)
	r.PUT("/kwr", APIUpdateKeywordReserve)
	r.DELETE("/kwr/:id", APIDeleteKeywordReserve)

	r.GET("/ssv", APIGetSessionValue)
	r.PUT("/ssv", APIUpdateSessionValue)

	r.GET("/dvs", APIGetDeviceAll)

	r.GET("/rcs", APIGetRecorderSetting)
	r.PUT("/rcs", APIUpdateRecorderSetting)

	r.GET("/pms", APIGetPowerManagerSetting)
	r.PUT("/pms", APIUpdatePowerManagerSetting)

	r.GET("/fps", APIGetFetchProgramScheduleSetting)
	r.PUT("/fps", APIUpdateFetchProgramScheduleSetting)
	r.POST("/fps/immediate", APIUpdateFetchProgramScheduleImmediately)

	r.POST("/playback", APIPlaybackStart)
	r.GET("/playback/recordings", APIPlaybackRecordings)

	r.GET("/cnf", APIGetConfigure)
	r.GET("/log/:date", APIGetSystemLog)
}
