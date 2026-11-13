package tvrock

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"golang.org/x/text/encoding/japanese"
)

func TestFilenameExUsesSubtitleWithoutSpace(t *testing.T) {
	prg := &TvProgram{Sid: 999, Eid: 999, Start: Now().Unix(), End: Now().Unix() + 3600, Subtitle: "subtitle"}
	if err := tvprogram.Replace(0, prg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { rockDB.Program[0].Exec("DELETE FROM program WHERE sid=? AND eid=?", prg.Sid, prg.Eid) })
	got, err := FilenameEx("@TT@sb", "title", "", "", prg.Sid, prg.Eid, 0, "", "")
	if err != nil || got != "titlesubtitle" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestFilenameExDoesNotQueryUnusedProgram(t *testing.T) {
	previous := tvprogram.db
	tvprogram.db = nil
	t.Cleanup(func() { tvprogram.db = previous })
	got, err := FilenameEx("@TT-@EC", "title", "", "", 0, 0, 7, "", "")
	if err != nil || got != "title-07" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestCreateIniTmpPreservesFinalDecodedBytes(t *testing.T) {
	dir := t.TempDir()
	src, dst := filepath.Join(dir, "source.ini"), filepath.Join(dir, "utf8.ini")
	want := "[サービス]\n名前=テスト"
	encoded, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(want))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	if err := CreateIniTmp(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != want {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestStationGetUsesReceiverCache(t *testing.T) {
	want := &TvStation{Sid: 123, Name: "test station"}
	local := &Station{Cache: map[uint]*TvStation{want.Sid: want}}
	got, ok := local.Get(want.Sid)
	if !ok || got != want {
		t.Fatalf("got %v, %v", got, ok)
	}
}

func TestJoinUintToStringUsesSeparator(t *testing.T) {
	if got := joinUintToString([]uint{1, 2, 3}, "|"); got != "1|2|3" {
		t.Fatal(got)
	}
}

func TestAllowOriginPrefixRequiresHostBoundary(t *testing.T) {
	previous := configure.AllowOriginPrefix
	configure.AllowOriginPrefix = []string{"http://localhost", "http://192.168."}
	t.Cleanup(func() { configure.AllowOriginPrefix = previous })
	for _, tt := range []struct {
		origin string
		want   bool
	}{
		{"http://localhost:8080", true},
		{"http://192.168.1.2:8080", true},
		{"http://localhost.example.com", false},
		{"http://localhost@evil.example", false},
		{"http://192.168.evil.example", false},
		{"https://localhost:8080", false},
	} {
		if got := allowOriginPrefix(tt.origin); got != tt.want {
			t.Errorf("%q: got %v, want %v", tt.origin, got, tt.want)
		}
	}
}

func TestDebugResponseOmitsPrivateConfiguration(t *testing.T) {
	previousDebug, previousKey, previousUsers := configure.Debug, configure.SessionSecretKey, configure.UserAccount
	configure.Debug, configure.SessionSecretKey = true, "private-session-secret"
	configure.UserAccount = map[string]string{"private-user": "private-password"}
	t.Cleanup(func() {
		configure.Debug, configure.SessionSecretKey, configure.UserAccount = previousDebug, previousKey, previousUsers
	})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	APIDebug(c)
	for _, private := range []string{"private-session-secret", "private-user", "private-password"} {
		if strings.Contains(w.Body.String(), private) {
			t.Fatalf("debug response leaked %q", private)
		}
	}
}

func TestLoginMiddlewareRejectsAnonymousAndInvalidSessions(t *testing.T) {
	previousAuth, previousUsers := configure.Authentication, configure.UserAccount
	configure.Authentication = true
	configure.UserAccount = map[string]string{"test-user": "test-password"}
	t.Cleanup(func() { configure.Authentication, configure.UserAccount = previousAuth, previousUsers })
	if err := InitializeSession(); err != nil {
		t.Fatal(err)
	}
	for _, data := range []interface{}{
		"invalid type",
		&UserSession{Revision: ssRevision, Data: newSessionData()},
		&UserSession{Revision: ssRevision, UserID: "removed-user", Data: newSessionData()},
	} {
		r := gin.New()
		r.Use(sessions.Sessions("test", cookie.NewStore([]byte("test-key"))))
		r.Use(func(c *gin.Context) { sessions.Default(c).Set(UserData, data) })
		r.Use(IsLogin())
		r.GET("/con", func(c *gin.Context) { c.Status(http.StatusOK) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/con", nil))
		if w.Code != http.StatusFound || w.Header().Get("Location") != PathLogin {
			t.Fatalf("invalid session reached protected handler: status %d", w.Code)
		}
	}
}

func TestProgramWriteReturnsCommitError(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	// A deferred foreign key violation succeeds at Exec and fails at Commit.
	for _, query := range []string{
		"PRAGMA foreign_keys=ON",
		"CREATE TABLE station_id (sid INTEGER PRIMARY KEY)",
		"CREATE TABLE program (sid INTEGER REFERENCES station_id(sid) DEFERRABLE INITIALLY DEFERRED, eid INTEGER, start INTEGER, end INTEGER, title TEXT, subtitle TEXT, content TEXT, cnib_a INTEGER, cnib_b INTEGER)",
	} {
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	store := TvProgramStore{db: &RockDB{Program: []*sql.DB{db}}}
	if err := store.Replace(0, &TvProgram{Sid: 1}); err == nil {
		t.Fatal("commit failure was hidden")
	}
}

func TestSessionPreferenceUpdatePreservesLogin(t *testing.T) {
	previousAuth, previousUsers := configure.Authentication, configure.UserAccount
	configure.Authentication = true
	configure.UserAccount = map[string]string{"test-user": "test-password"}
	t.Cleanup(func() { configure.Authentication, configure.UserAccount = previousAuth, previousUsers })
	InitializeSession()
	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("test-key"))))
	r.Use(IsLogin())
	r.GET(PathLogin, func(c *gin.Context) {
		if err := initializeUserSession(c, "test-user"); err != nil {
			t.Error(err)
		}
	})
	r.PUT("/ssv", APIUpdateSessionValue)
	r.GET("/ssv", APIGetSessionValue)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, PathLogin, nil))
	if len(w.Result().Cookies()) == 0 {
		t.Fatal("login did not set a cookie")
	}
	request := httptest.NewRequest(http.MethodPut, "/ssv", strings.NewReader(`{"hours":12,"viewstations":4}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(w.Result().Cookies()[0])
	w = httptest.NewRecorder()
	r.ServeHTTP(w, request)
	if w.Code != http.StatusOK || len(w.Result().Cookies()) == 0 {
		t.Fatalf("preference update failed: %d %s", w.Code, w.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/ssv", nil)
	request.AddCookie(w.Result().Cookies()[0])
	w = httptest.NewRecorder()
	r.ServeHTTP(w, request)
	var data MySessionData
	if err := json.Unmarshal(w.Body.Bytes(), &data); err != nil || w.Code != http.StatusOK || data.Hours != 12 {
		t.Fatalf("login or preferences lost: %d %s %v", w.Code, w.Body.String(), err)
	}
	if strings.Contains(w.Body.String(), "test-user") {
		t.Fatal("session response leaked account information")
	}
}

func TestConfigureResponseOmitsCommandPathsAndArguments(t *testing.T) {
	previous := configure.CustomCommand
	configure.CustomCommand = []CustomCommand{{ID: "test", AppPath: "/private/path", Options: []string{"private-token"}}}
	t.Cleanup(func() { configure.CustomCommand = previous })
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	APIGetConfigure(c)
	if strings.Contains(w.Body.String(), "/private/path") || strings.Contains(w.Body.String(), "private-token") {
		t.Fatal("command configuration leaked private fields")
	}
	if !strings.Contains(w.Body.String(), `"ID":"test"`) {
		t.Fatal("command ID missing from configuration")
	}
}

func TestDisconnectedControllerUnblocksPublisher(t *testing.T) {
	queue := &DevConQueue{Com: make(chan string), Done: make(chan struct{})}
	finished := make(chan struct{})
	go func() {
		queue.send("test")
		close(finished)
	}()
	close(queue.Done)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("publisher remained blocked after controller disconnect")
	}
}

func TestStorageInfoHandlesUnnamedAndMultiwordVolumes(t *testing.T) {
	for _, tt := range []struct {
		name string
		rows [][]string
	}{
		{"volume last as configured", [][]string{
			{"Caption", "FreeSpace", "Size", "VolumeName"},
			{"-------", "---------", "----", "----------"},
			{"C:", "20", "100"},
			{"D:", "40", "200", "Recorded", "Programs"},
			{"E:", "300", "100", "invalid"},
			{"F:", "not-a-number", "100", "invalid"},
			{"G:", "10", "invalid", "invalid"},
			{"H:", "50", "100", "Archive"},
			{"truncated"},
		}},
		{"volume between numeric columns", [][]string{
			{"Caption", "VolumeName", "FreeSpace", "Size"},
			{"---", "---", "---", "---"},
			{"C:", "20", "100"},
			{"D:", "Recorded", "Programs", "40", "200"},
			{"E:", "invalid", "300", "100"},
			{"H:", "Archive", "50", "100"},
			{"truncated"},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			info := parseStorageInfo(tt.rows)
			if len(info) != 3 || info["C:"] == nil || *info["C:"] != (StorageInfo{Using: 80, Size: 100}) ||
				info["D:Recorded Programs"] == nil || *info["D:Recorded Programs"] != (StorageInfo{Using: 160, Size: 200}) ||
				info["H:Archive"] == nil || *info["H:Archive"] != (StorageInfo{Using: 50, Size: 100}) {
				t.Fatalf("unexpected storage information: %+v", info)
			}
		})
	}
}
