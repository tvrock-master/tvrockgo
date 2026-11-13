package tvrock

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// クライアント識別子は、既存の認証およびオリジンチェックを補完するものです。
func validClientIdentity(identity string, allowed ...string) bool {
	name, version, ok := strings.Cut(identity, "/")
	if !ok || version == "" {
		return false
	}
	// devトークンおよびバージョン・トークン（プレリリース／ビルド・メタデータを含む）を受け入れる
	for _, ch := range version {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' ||
			ch >= '0' && ch <= '9' || strings.ContainsRune("-._+", ch)) {
			return false
		}
	}
	for _, client := range allowed {
		if name == client {
			return true
		}
	}
	return false
}

func acceptClientIdentity(g *gin.Context, allowQuery bool, allowed ...string) bool {
	var identity string
	if values, exists := g.Request.Header[http.CanonicalHeaderKey(HEADER_CLIENT_TVROCK_CLIENT)]; exists {
		if len(values) != 1 || !validClientIdentity(values[0], allowed...) {
			return rejectClientIdentity(g)
		}
		identity = values[0]
	}
	if allowQuery {
		if values, exists := g.Request.URL.Query()[HEADER_CLIENT_TVROCK_CLIENT]; exists {
			if len(values) != 1 || !validClientIdentity(values[0], allowed...) ||
				identity != "" && identity != values[0] {
				return rejectClientIdentity(g)
			}
			identity = values[0]
		}
	}
	if identity == "" {
		values := g.Request.Header.Values("User-Agent")
		if len(values) != 1 || !validClientIdentity(values[0], allowed...) {
			return rejectClientIdentity(g)
		}
	}
	return true
}

func rejectClientIdentity(g *gin.Context) bool {
	g.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "unsupported client"})
	return false
}

func requireAPIClientIdentity(g *gin.Context) {
	if acceptClientIdentity(g, false, HEADER_CLIENT_TVROCK_APP, HEADER_CLIENT_TVROCK_ROCKBAR) {
		g.Next()
	}
}
