package tvrock

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
)

func validLauncherToken(token string) bool {
	if len(token) != 64 {
		return false
	}
	_, err := hex.DecodeString(token)
	return err == nil
}

func currentLauncherToken() string {
	configure.mux.RLock()
	defer configure.mux.RUnlock()
	return configure.LauncherToken
}

func launcherRequestHeaders(hostID string) (http.Header, error) {
	token := currentLauncherToken()
	if !validLauncherToken(token) {
		return nil, fmt.Errorf("launcher credentials are not initialized")
	}
	headers := http.Header{
		"Content-Type":  {"application/json; charset=utf8"},
		"User-Agent":    {HEADER_CLIENT_TVROCK_LAUNCHER + "/" + Version},
		"Authorization": {"Bearer " + token},
	}
	headers.Set(HEADER_CLIENT_TVROCK_LAUNCHER_HOST_ID, hostID)
	return headers, nil
}

// Cookie・ブラウザ認証の有効無効にかかわらず機械間の秘密トークンを要求する。
// エラー応答やログへ受信した資格情報を出力しない。
func acceptLauncherAuthentication(g *gin.Context) (string, bool) {
	expected := currentLauncherToken()
	auth := g.Request.Header.Values("Authorization")
	ids := g.Request.Header.Values(HEADER_CLIENT_TVROCK_LAUNCHER_HOST_ID)
	if validLauncherToken(expected) && len(auth) == 1 && len(ids) == 1 {
		scheme, token, ok := strings.Cut(auth[0], " ")
		id := strings.TrimSpace(ids[0])
		if ok && strings.EqualFold(scheme, "Bearer") &&
			subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1 &&
			id != "" && len(id) <= 256 && strings.IndexFunc(id, unicode.IsControl) < 0 {
			return id, true
		}
	}
	g.Header("WWW-Authenticate", "Bearer")
	g.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "launcher authentication failed"})
	return "", false
}
