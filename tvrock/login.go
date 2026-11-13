package tvrock

import (
	"crypto/subtle"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// バージョンはビルド時に -ldflags を通じて設定されます
var Version = "dev"

const (
	PathLogin        = "/login"
	PathLogout       = "/logout"
	PathHealthCheck  = "/hc"
	PathFavicon      = "/favicon.ico"
	PathTvRockGOIcon = "/static/image/tvrockgo180x180.png"
	PathRockbarIcon  = "/static/rockbar/icons/rockbar180x180.png"
	PathManifest     = "/manifest.json"
	PathDevice       = "/dev"
	PathController   = "/con"
	PathLauncher     = "/launcher"

	templateLogin = "login.html"
	staticIndex   = "/static/index.html"
	UserData      = "userdata"
)

const (
	loginMaxFailures = 10
	loginLockout     = 15 * time.Minute
)

type UserSession struct {
	Revision string
	UserID   string
	Data     *MySessionData
	Invite   map[string]bool
}

type loginFailure struct {
	Count       int
	LockedUntil time.Time
}

type loginFailureStore struct {
	mux   sync.Mutex
	items map[string]loginFailure
}

var loginFailures = loginFailureStore{
	items: map[string]loginFailure{},
}

func addNoStoreHeader(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, private, max-age=0, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

func (p *loginFailureStore) isLocked(key string, now time.Time) bool {
	p.mux.Lock()
	defer p.mux.Unlock()

	failure, ok := p.items[key]
	if !ok || failure.LockedUntil.IsZero() {
		return false
	}
	if now.Before(failure.LockedUntil) {
		return true
	}
	delete(p.items, key)
	return false
}

func (p *loginFailureStore) recordFailure(key string, now time.Time) {
	p.mux.Lock()
	defer p.mux.Unlock()

	failure := p.items[key]
	failure.Count++
	if failure.Count >= loginMaxFailures {
		failure.LockedUntil = now.Add(loginLockout)
	}
	p.items[key] = failure
}

func (p *loginFailureStore) reset(key string) {
	p.mux.Lock()
	defer p.mux.Unlock()
	delete(p.items, key)
}

func initializeUserSession(c *gin.Context, uid string) error {
	session := sessions.Default(c)
	us := UserSession{
		Revision: ssRevision,
		UserID:   uid,
		Data:     newSessionData(),
	}
	session.Clear()
	session.Set(UserData, &us)
	return session.Save()
}

func authenticateUser(uid, pwd string) bool {
	configuredPassword, ok := configure.UserAccount[uid]
	if !ok {
		return false
	}
	if isPasswordHash(configuredPassword) {
		return bcrypt.CompareHashAndPassword([]byte(configuredPassword), []byte(pwd)) == nil
	}
	return subtle.ConstantTimeCompare([]byte(configuredPassword), []byte(pwd)) == 1
}

func IsLogin() gin.HandlerFunc {
	return func(c *gin.Context) {
		addNoStoreHeader(c)

		// スルーパス
		if c.Request.URL.Path == PathLogin ||
			c.Request.URL.Path == PathLogout ||
			c.Request.URL.Path == PathHealthCheck ||
			c.Request.URL.Path == PathFavicon ||
			c.Request.URL.Path == PathTvRockGOIcon ||
			c.Request.URL.Path == PathRockbarIcon ||
			c.Request.URL.Path == PathManifest ||
			c.Request.URL.Path == PathDevice ||
			c.Request.URL.Path == PathLauncher {
			c.Next()
			return
		}

		session := sessions.Default(c)
		us, ok := session.Get(UserData).(*UserSession)
		valid := ok && us != nil && us.Data != nil && us.Revision == ssRevision
		if valid && configure.Authentication {
			_, valid = configure.UserAccount[us.UserID]
			valid = valid && us.UserID != ""
		}
		if valid {
			c.Next()
			return
		}
		if !configure.Authentication {
			if err := initializeUserSession(c, ""); err != nil {
				log.Println("error: session save error.", err)
				c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
				c.Abort()
				return
			}
			c.Next()
			return
		}
		session.Clear()
		if err := session.Save(); err != nil {
			log.Println("error: session save error.", err)
		}
		c.Redirect(http.StatusFound, PathLogin)
		c.Abort()
	}
}

func GetLogin(c *gin.Context) {
	c.HTML(http.StatusOK, templateLogin, gin.H{"Version": Version})
}

func GetLogout(c *gin.Context) {
	addNoStoreHeader(c)
	c.Header("Clear-Site-Data", "\"cache\"")

	// セッション消去
	session := sessions.Default(c)
	session.Clear()
	session.Options(sessions.Options{
		Domain:   sessionCookieDomain(),
		Path:     configure.CookiePath,
		MaxAge:   -1,
		Secure:   configure.SessionSecure,
		HttpOnly: configure.SessionHTTPOnly,
		SameSite: http.SameSiteLaxMode,
	})
	if err := session.Save(); err != nil {
		log.Println("error: session save error.", err)
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		return
	}

	c.Redirect(http.StatusFound, PathLogin)
}

func sessionCookieDomain() string {
	if configure.CookieDomain == "localhost" || configure.CookieDomain == "127.0.0.1" {
		return ""
	}
	return configure.CookieDomain
}

func PostLogin(c *gin.Context) {
	clientIP := c.ClientIP()
	if loginFailures.isLocked(clientIP, time.Now()) {
		log.Println("error: too many login failures.")
		c.HTML(http.StatusTooManyRequests, templateError403, gin.H{})
		return
	}

	var b bool
	var uid, pwd string
	uid, b = c.GetPostForm("username")
	if !b || len(uid) < 3 || len(uid) > 64 {
		log.Println("error: unknown parameter.(user-id)")
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		return
	}
	pwd, b = c.GetPostForm("password")
	if !b || len(pwd) == 0 || len(pwd) > 256 {
		log.Println("error: unknown parameter.(password)")
		c.HTML(http.StatusBadRequest, templateError403, gin.H{})
		return
	}

	if !authenticateUser(uid, pwd) {
		log.Println("error: authentication error.")
		loginFailures.recordFailure(clientIP, time.Now())
		c.HTML(http.StatusForbidden, templateError403, gin.H{})
		return
	}
	loginFailures.reset(clientIP)

	if err := initializeUserSession(c, uid); err != nil {
		log.Println("error: session save error.", err)
		c.HTML(http.StatusInternalServerError, templateError500, gin.H{})
		return
	}
	c.Redirect(http.StatusFound, staticIndex)
}
