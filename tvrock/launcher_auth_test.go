package tvrock

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestLauncherTokenGenerationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tvrock.yml")
	conf := TvRockConf{SessionSecretKey: "separate-session-secret"}
	if err := conf.ensureLauncherToken(path); err != nil {
		t.Fatal(err)
	}
	if !validLauncherToken(conf.LauncherToken) || conf.LauncherToken == conf.SessionSecretKey {
		t.Fatal("launcher token was not generated independently")
	}
	if conf.SessionSecretKey != "separate-session-secret" {
		t.Fatal("session secret changed")
	}
	first := conf.LauncherToken
	var loaded TvRockConf
	if err := loaded.Load(path); err != nil {
		t.Fatal(err)
	}
	if loaded.LauncherToken != first {
		t.Fatal("launcher token was not persisted")
	}
	if err := loaded.ensureLauncherToken(path); err != nil {
		t.Fatal(err)
	}
	if loaded.LauncherToken != first {
		t.Fatal("launcher token changed on reload")
	}
	other := TvRockConf{}
	if err := other.ensureLauncherToken(filepath.Join(t.TempDir(), "other.yml")); err != nil {
		t.Fatal(err)
	}
	if other.LauncherToken == first {
		t.Fatal("independent installations share a token")
	}
	encoded, err := json.Marshal(&conf)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), first) || strings.Contains(string(encoded), "LauncherToken") {
		t.Fatal("launcher token leaked through JSON")
	}
}

func TestLauncherTokenSaveFailureAndInvalidConfiguration(t *testing.T) {
	conf := TvRockConf{}
	if err := conf.ensureLauncherToken(filepath.Join(t.TempDir(), "missing", "tvrock.yml")); err == nil {
		t.Fatal("saving to a missing directory succeeded")
	}
	if conf.LauncherToken != "" {
		t.Fatal("unsaved token remained usable")
	}
	path := filepath.Join(t.TempDir(), "tvrock.yml")
	for _, invalid := range []string{"short", strings.Repeat("g", 64), strings.Repeat("a", 63)} {
		conf.LauncherToken = invalid
		err := conf.ensureLauncherToken(path)
		if err == nil {
			t.Fatal("invalid token accepted")
		}
		if strings.Contains(err.Error(), invalid) {
			t.Fatal("invalid token leaked in error")
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid configuration was saved")
	}
}

func TestLauncherAuthenticationBeforeUpgrade(t *testing.T) {
	oldToken, oldAuth := configure.LauncherToken, configure.Authentication
	t.Cleanup(func() { configure.LauncherToken, configure.Authentication = oldToken, oldAuth })
	token := strings.Repeat("ab", 32)
	var logs bytes.Buffer
	oldLogOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(oldLogOutput) })
	for _, browserAuth := range []bool{false, true} {
		configure.Authentication = browserAuth
		for _, tc := range []struct {
			name       string
			auth       []string
			ids        []string
			configured string
			query      string
			status     int
		}{
			{"missing", nil, []string{"internal"}, token, "", 401},
			{"legacy-hostname", []string{"Bearer computer-name"}, []string{"internal"}, token, "", 401},
			{"wrong", []string{"Bearer " + strings.Repeat("cd", 32)}, []string{"internal"}, token, "", 401},
			{"wrong-scheme", []string{"Basic " + token}, []string{"internal"}, token, "", 401},
			{"extra-value", []string{"Bearer " + token + " extra"}, []string{"internal"}, token, "", 401},
			{"duplicate-auth", []string{"Bearer " + token, "Bearer " + token}, []string{"internal"}, token, "", 401},
			{"missing-id", []string{"Bearer " + token}, nil, token, "", 401},
			{"duplicate-id", []string{"Bearer " + token}, []string{"one", "two"}, token, "", 401},
			{"not-initialized", []string{"Bearer " + token}, []string{"internal"}, "", "", 401},
			{"query-not-allowed", nil, []string{"internal"}, token, "?token=" + token, 401},
			{"valid", []string{"Bearer " + token}, []string{"internal"}, token, "", 400},
		} {
			t.Run(tc.name+map[bool]string{true: "/browser-auth", false: "/no-browser-auth"}[browserAuth], func(t *testing.T) {
				configure.LauncherToken = tc.configured
				r := gin.New()
				// nilでも認証失敗はUpgradeやランチャーの状態操作へ進まない。
				var launcher *Launcher
				r.GET(PathLauncher, launcher.SockLauncherHandler)
				req := httptest.NewRequest(http.MethodGet, PathLauncher+tc.query, nil)
				req.Header.Set("User-Agent", HEADER_CLIENT_TVROCK_LAUNCHER+"/dev")
				for _, value := range tc.auth {
					req.Header.Add("Authorization", value)
				}
				for _, value := range tc.ids {
					req.Header.Add(HEADER_CLIENT_TVROCK_LAUNCHER_HOST_ID, value)
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != tc.status {
					t.Fatalf("status=%d, want %d", w.Code, tc.status)
				}
				if w.Header().Get("Upgrade") != "" {
					t.Fatal("unauthorized request was upgraded")
				}
				if strings.Contains(w.Body.String(), token) {
					t.Fatal("response leaked token")
				}
			})
		}
	}
	if strings.Contains(logs.String(), token) {
		t.Fatal("log leaked token")
	}
}

func TestLauncherAuthenticatedWebSocket(t *testing.T) {
	previous := configure.LauncherToken
	configure.LauncherToken = strings.Repeat("12", 32)
	t.Cleanup(func() { configure.LauncherToken = previous })
	l := &Launcher{}
	if err := l.Initialize(); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET(PathLauncher, l.SockLauncherHandler)
	// テスト専用のループバックHTTPサーバー。運用設定・録画アプリは使用しない。
	server := httptest.NewServer(r)
	defer server.Close()
	headers, err := launcherRequestHeaders("internal-launcher")
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("Authorization") != "Bearer "+configure.LauncherToken || headers.Get(HEADER_CLIENT_TVROCK_LAUNCHER_HOST_ID) != "internal-launcher" {
		t.Fatal("client credentials were not separated from host identity")
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+PathLauncher, headers)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if !l.WaitForConnection(context.Background(), time.Second) {
		t.Fatal("authenticated launcher did not register")
	}
	l.RLock()
	_, exists := l.Pool["internal-launcher"]
	_, leaked := l.Pool[configure.LauncherToken]
	l.RUnlock()
	if !exists || leaked {
		t.Fatal("connection identity contains credentials")
	}
	l.BootDevice(`{"registryid":"test-only","boottype":1}`)
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	_, msg, err := conn.ReadMessage()
	if err != nil || !strings.HasPrefix(string(msg), LauncherComBootDevice) {
		t.Fatalf("boot notification: %q, %v", msg, err)
	}
	// この要求はテスト接続で受信するだけで、録画アプリの起動処理へは渡さない。
}
