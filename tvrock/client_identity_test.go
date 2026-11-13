package tvrock

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAPIClientIdentityOnAllRoutes(t *testing.T) {
	previousDebug := configure.Debug
	configure.Debug = true
	t.Cleanup(func() { configure.Debug = previousDebug })
	r := gin.New()
	// Stop after identity validation so no API accesses settings or the DB.
	registerAPIRoutes(r.Group("", requireAPIClientIdentity, func(g *gin.Context) {
		g.AbortWithStatus(http.StatusNoContent)
	}))
	for _, route := range r.Routes() {
		path := strings.NewReplacer(":id", "1", ":date", "2026-01-01").Replace(route.Path)
		for _, tc := range []struct {
			name, agent, client string
			status              int
		}{
			{"missing", "", "", http.StatusForbidden},
			{"browser-only", "Mozilla/5.0", "", http.StatusForbidden},
			{"app-agent", "TvRockApp/dev", "", http.StatusNoContent},
			{"rockbar-agent", "TvRockRockbar/1.2.3-rc.1+build", "", http.StatusNoContent},
			{"app-header", "Mozilla/5.0", "TvRockApp/2.0.0", http.StatusNoContent},
			{"rockbar-header", "Mozilla/5.0", "TvRockRockbar/dev", http.StatusNoContent},
			{"device", "TvRockDTV/1.0", "", http.StatusForbidden},
			{"launcher", "TvRockLauncher/dev", "", http.StatusForbidden},
			{"invalid-header-with-valid-agent", "TvRockApp/dev", "Unknown/dev", http.StatusForbidden},
		} {
			t.Run(route.Method+path+"/"+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(route.Method, path, nil)
				if tc.agent != "" {
					req.Header.Set("User-Agent", tc.agent)
				}
				if tc.client != "" {
					req.Header.Set(HEADER_CLIENT_TVROCK_CLIENT, tc.client)
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != tc.status {
					t.Fatalf("status = %d, want %d: %s", w.Code, tc.status, w.Body.String())
				}
			})
		}
	}
}

func TestWebSocketClientIdentityBeforeUpgrade(t *testing.T) {
	previousToken := configure.LauncherToken
	configure.LauncherToken = strings.Repeat("ab", 32)
	t.Cleanup(func() { configure.LauncherToken = previousToken })
	r := gin.New()
	var controller *DeviceController
	var launcher *Launcher
	r.GET(PathDevice, SockDeviceTerminal)
	r.GET(PathController, controller.SockControllerHandler)
	r.GET(PathLauncher, launcher.SockLauncherHandler)
	for _, tc := range []struct {
		path, agent, client string
		accepted            bool
	}{
		{PathDevice, "TvRockDTV/1.2.3", "", true},
		{PathDevice, "TvRockDTV-Sib/dev", "", true},
		{PathDevice, "", "TvRockDTV/dev", true},
		{PathDevice, "TvRockApp/dev", "", false},
		{PathDevice, "TvRockLauncher/dev", "", false},
		{PathDevice, "", "", false},
		{PathDevice + "?X-TvRock-Client=TvRockDTV/dev", "Mozilla/5.0", "", false},
		{PathController, "TvRockApp/dev", "", true},
		{PathController, "TvRockRockbar/3.0.0", "", true},
		{PathController, "Mozilla/5.0", "TvRockApp/dev", true},
		{PathController + "?X-TvRock-Client=TvRockRockbar%2Fdev", "Mozilla/5.0", "", true},
		{PathController + "?X-TvRock-Client=TvRockApp/dev", "Mozilla/5.0", "TvRockApp/dev", true},
		{PathController + "?X-TvRock-Client=TvRockApp/other", "Mozilla/5.0", "TvRockApp/dev", false},
		{PathController + "?X-TvRock-Client=Unknown/dev", "TvRockApp/dev", "", false},
		{PathController + "?X-TvRock-Client=TvRockApp/dev&X-TvRock-Client=TvRockRockbar/dev", "", "", false},
		{PathController, "TvRockDTV/dev", "", false},
		{PathController, "", "", false},
		{PathLauncher, "TvRockLauncher/dev", "", true},
		{PathLauncher, "", "TvRockLauncher/2.0.0", true},
		{PathLauncher, "TvRockDTV/dev", "", false},
		{PathLauncher, "TvRockRockbar/dev", "", false},
		{PathLauncher, "", "", false},
	} {
		t.Run(tc.path+"/"+tc.agent+tc.client, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("User-Agent", tc.agent)
			if tc.path == PathLauncher {
				req.Header.Set("Authorization", "Bearer "+configure.LauncherToken)
				req.Header.Set(HEADER_CLIENT_TVROCK_LAUNCHER_HOST_ID, "test-launcher")
			}
			if tc.client != "" {
				req.Header.Set(HEADER_CLIENT_TVROCK_CLIENT, tc.client)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			want := http.StatusForbidden
			if tc.accepted {
				// Valid identities reach Upgrade, which rejects this plain HTTP request.
				want = http.StatusBadRequest
			}
			if w.Code != want {
				t.Fatalf("status = %d, want %d: %s", w.Code, want, w.Body.String())
			}
		})
	}
}

func TestClientIdentityRejectsMalformedAndAmbiguousValues(t *testing.T) {
	r := gin.New()
	r.GET("/api", requireAPIClientIdentity, func(g *gin.Context) { g.Status(http.StatusNoContent) })
	for _, value := range []string{"", "TvRockApp", "TvRockApp/", "TvRockAppExtra/dev", "tvrockapp/dev",
		"TvRockApp/dev extra", "TvRockApp/dev/other", "TvRockApp/dev,TvRockRockbar/dev", "TvRockApp/日本語"} {
		t.Run(value, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api", nil)
			req.Header.Set("User-Agent", "TvRockApp/dev")
			req.Header.Set(HEADER_CLIENT_TVROCK_CLIENT, value)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusForbidden {
				t.Fatalf("malformed identity accepted: %q", value)
			}
		})
	}
	for _, header := range []string{"User-Agent", HEADER_CLIENT_TVROCK_CLIENT} {
		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Add(header, "TvRockApp/dev")
		req.Header.Add(header, "TvRockRockbar/dev")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Fatalf("duplicate %s accepted", header)
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api?X-TvRock-Client=TvRockApp/dev", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatal("API must not accept query identity")
	}
}
