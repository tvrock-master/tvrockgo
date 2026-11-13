package tvrock

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	LauncherComBootDevice = "bdv:"
	LauncherComExit       = "ext:"
	LauncherComDeviceOn   = "EDB:"
	LauncherComDeviceOff  = "EDF:"
)

type LauncherConnection struct {
	HostName string      `json:"hostname"`
	Com      chan string `json:"-"`
}

type LaunchMessage struct {
	RegistryID string `json:"registryid"`
	DeviceID   string `json:"deviceid"`
	BootType   uint   `json:"boottype"`
	Sid        uint   `json:"sid"`
}

type Launcher struct {
	Pool           map[string]*LauncherConnection
	Com            chan string
	Booting        map[string]bool
	connectWaiters []chan struct{}
	sync.RWMutex
}

var launcher Launcher

func (p *Launcher) Initialize() error {

	p.Pool = map[string]*LauncherConnection{}
	p.Com = make(chan string, 256)
	p.Booting = map[string]bool{}
	p.connectWaiters = nil

	return nil
}

func (p *Launcher) reserveBoot(rid string) bool {
	p.Lock()
	defer p.Unlock()
	if p.Booting == nil {
		p.Booting = map[string]bool{}
	}
	if p.Booting[rid] {
		return false
	}
	p.Booting[rid] = true
	return true
}

func (p *Launcher) releaseBoot(rid string) {
	p.Lock()
	defer p.Unlock()
	delete(p.Booting, rid)
}

func (p *Launcher) BootDevice(msg string) {
	com := LauncherComBootDevice + msg
	connections := []*LauncherConnection{}
	p.RLock()
	for _, val := range p.Pool {
		connections = append(connections, val)
	}
	p.RUnlock()

	if len(connections) == 0 {
		log.Printf("warn: launcher is not connected")
		p.execBootMessageLocally("no connected launcher", msg)
		return
	}

	sent := false
	for _, val := range connections {
		select {
		case val.Com <- com:
			sent = true
		default:
			log.Printf("warn: launcher command channel is full: %s", val.HostName)
		}
	}

	if !sent && configure.BootLauncher {
		p.execBootMessageLocally("launcher command channel send failed", msg)
	}
}

func (p *Launcher) execBootMessageLocally(reason string, msg string) {
	if !configure.BootLauncher {
		return
	}

	var lms LaunchMessage
	if err := json.Unmarshal([]byte(msg), &lms); err != nil {
		log.Println("error:", err)
		return
	}

	log.Printf("warn: executing boot request locally (%s): %s", reason, msg)
	p.Exec(&lms)
}

func (p *Launcher) Exec(msg *LaunchMessage) {

	log.Printf("debug: Exec %s/%s, %d\n", msg.DeviceID, msg.RegistryID, msg.BootType)

	for _, dev := range configure.Devices {
		app := ""
		opt := ""
		if dev.RegistryCode == msg.RegistryID || dev.ID == msg.DeviceID {
			if msg.BootType != DeviceBootOFF {
				// 手動起動・視聴への引き継ぎでも、以前のタスクの終了要求を破棄する。
				(&RecordingJob{DeviceID: dev.ID}).ShutdownCancel()
			}
			switch msg.BootType {
			case DeviceBootOFF:
				if keepWatchDevice(dev.ID, Now()) {
					continue
				}
				if len(msg.RegistryID) > 0 && !slot.IsBootingByRID(msg.RegistryID) {
					log.Println("debug: already shutdown", msg.RegistryID)
					continue
				}
				slot.SendByRID(&RequestMessage{
					ID:      msg.RegistryID,
					Command: "PowerOff",
					Message: "web controll",
				})
				continue
			case DeviceBootTypeWatch,
				DeviceBootTypeWatchOnly:
				if slot.IsBootingByRID(dev.RegistryCode) {
					log.Println("debug: already bootup", dev.RegistryCode)
					continue
				}
				app = dev.AppPath1
				opt = dev.Option1
			case DeviceBootTypeRecord,
				DeviceBootTypeFetchProgramTS,
				DeviceBootTypeFetchProgramBS,
				DeviceBootTypeFetchProgramCS:
				if slot.IsBootingByRID(dev.RegistryCode) {
					log.Println("debug: already bootup", dev.RegistryCode)
					continue
				}
				app = dev.AppPath2
				opt = dev.Option2
			case DeviceBootTypeOnesegWatch,
				DeviceBootTypeOnesegWatchOnly:
				if slot.IsBootingByRID(dev.RegistryCode) {
					log.Println("debug: already bootup", dev.RegistryCode)
					continue
				}
				app = dev.AppPath3
				opt = dev.Option3
			case DeviceBootTypeOnesegRecord,
				DeviceBootTypeFetchProgramOS:
				if slot.IsBootingByRID(dev.RegistryCode) {
					log.Println("debug: already bootup", dev.RegistryCode)
					continue
				}
				app = dev.AppPath4
				opt = dev.Option4
			}
		} else {
			continue
		}

		if len(app) < 1 {
			continue
		}
		if !p.reserveBoot(dev.RegistryCode) {
			log.Println("debug: already bootup", dev.RegistryCode)
			continue
		}

		// スレッドにして起動
		go func(app string, opt string, id string, rcode string, sid uint) {

			defer p.releaseBoot(rcode)

			opts := strings.Split(opt, " ")
			opts = append(opts, "/host", launcherClientHost(configure.Host))
			opts = append(opts, "/pid", rcode)
			if sid > 0 {
				opts = append(opts, "/sid", strconv.FormatUint(uint64(sid), 10))
			}

			dir, _ := filepath.Split(app)
			dir = pathRemoveFileSpec(dir)

			exe := exec.Command(app, opts...)
			exe.Dir = dir

			// マスターに起動情報を送る
			p.sendStatus(LauncherComDeviceOn + rcode)

			log.Printf("debug: Run>%s %s", app, opt)

			// 起動
			if err := exe.Run(); err != nil {
				p.sendStatus(LauncherComDeviceOff + rcode)
				log.Println(err)
				return
			}

			p.sendStatus(LauncherComDeviceOff + rcode)
			log.Println("Terminate: ", id, rcode)

		}(app, opt, dev.ID, dev.RegistryCode, msg.Sid&0xffff)
	}
}

func (p *Launcher) sendStatus(msg string) {
	if p.Com == nil {
		log.Printf("warn: launcher status channel is not initialized: %s", msg)
		p.applyStatus(msg)
		return
	}
	if !p.hasConnection() {
		log.Printf("warn: launcher is not connected; skipped status: %s", msg)
		p.applyStatus(msg)
		return
	}

	select {
	case p.Com <- msg:
	default:
		log.Printf("warn: launcher status channel is full: %s", msg)
		p.applyStatus(msg)
	}
}

func (p *Launcher) applyStatus(msg string) {
	if len(msg) < 4 {
		log.Printf("warn: launcher status message is too short: %s", msg)
		return
	}

	com := msg[:4]
	rid := string(msg[4:])
	switch com {
	case LauncherComDeviceOn:
		if v, ok := slot.GetByRID(rid); ok {
			slot.Lock()
			v.State = StateBooting
			v.processExited = false
			slot.Unlock()
			deviceController.Notify(ControllerSendDevice, rid)
			log.Println("info: デバイスの起動を要求", v.ID, rid)
		} else {
			log.Println("error: デバイスの起動でデバイスを認識できません", rid)
		}
	case LauncherComDeviceOff:
		if v, ok := slot.GetByRID(rid); ok {
			slot.Lock()
			v.processExited = true
			slot.setConnectionStateLocked(v, StateOFF)
			slot.Unlock()
			deviceController.Notify(ControllerSendDevice, rid)
			log.Println("info: デバイスの終了を要求", v.ID, rid)
		} else {
			log.Println("error: デバイスの終了でデバイスを認識できません", rid)
		}
	default:
		log.Printf("debug: unknown launcher status: %s", msg)
	}
}

func (p *Launcher) hasConnection() bool {
	return p.HasConnection()
}

func (p *Launcher) HasConnection() bool {
	p.RLock()
	defer p.RUnlock()
	return len(p.Pool) > 0
}

func (p *Launcher) notifyConnected() {
	p.Lock()
	waiters := p.connectWaiters
	p.connectWaiters = nil
	p.Unlock()

	for _, ch := range waiters {
		close(ch)
	}
}

// WaitForConnection はランチャーが接続されるまで待機します。
// 既に接続がある場合や、timeout時間内に接続された場合は true を返します。
// タイムアウトまたは context のキャンセルが発生した場合は false を返します。
func (p *Launcher) WaitForConnection(ctx context.Context, timeout time.Duration) bool {
	p.RLock()
	if len(p.Pool) > 0 {
		p.RUnlock()
		return true
	}
	p.RUnlock()

	ch := make(chan struct{})
	p.Lock()
	if len(p.Pool) > 0 {
		p.Unlock()
		return true
	}
	p.connectWaiters = append(p.connectWaiters, ch)
	p.Unlock()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ch:
		return true
	case <-timer.C:
		p.Lock()
		for i, w := range p.connectWaiters {
			if w == ch {
				p.connectWaiters = append(p.connectWaiters[:i], p.connectWaiters[i+1:]...)
				break
			}
		}
		hasConn := len(p.Pool) > 0
		p.Unlock()
		return hasConn
	case <-ctx.Done():
		p.Lock()
		for i, w := range p.connectWaiters {
			if w == ch {
				p.connectWaiters = append(p.connectWaiters[:i], p.connectWaiters[i+1:]...)
				break
			}
		}
		p.Unlock()
		return false
	}
}

func launcherClientHost(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return "localhost"
	}

	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		u, err := url.Parse(host)
		if err != nil {
			return host
		}
		u.Host = replaceUnspecifiedHost(u.Host)
		return strings.TrimPrefix(strings.TrimPrefix(u.String(), "http://"), "https://")
	}

	return replaceUnspecifiedHost(host)
}

func replaceUnspecifiedHost(host string) string {
	h, port, err := net.SplitHostPort(host)
	if err == nil {
		if isUnspecifiedHost(h) {
			return net.JoinHostPort("localhost", port)
		}
		return host
	}

	if strings.HasPrefix(host, ":") {
		return "localhost" + host
	}
	if isUnspecifiedHost(host) {
		return "localhost"
	}
	return host
}

func isUnspecifiedHost(host string) bool {
	host = strings.Trim(host, "[]")
	return host == "" || host == "0.0.0.0" || host == "::"
}

func (p *Launcher) ExecLauncher(host string, hostID string) {
	ctx, cancel := context.WithCancel(globalContext)
	defer cancel()

	for {
		if p.execLauncher(host, hostID) {
			return
		}

		select {
		case <-ctx.Done():
			log.Println("info: 終了通知を検出")
			return

		case <-time.After(5 * time.Second):
		}
	}
}

func (p *Launcher) execLauncher(host string, hostID string) bool {
	scm := "ws"
	connectHost := launcherClientHost(host)
	if strings.HasPrefix(host, "https:") {
		scm = "wss"
	}
	u := url.URL{Scheme: scm, Host: connectHost, Path: "/launcher"}
	log.Printf("info: ランチャの接続 %s", u.String())

	if len(hostID) < 1 {
		hostID, _ = os.Hostname()
	}
	headers, err := launcherRequestHeaders(hostID)
	if err != nil {
		log.Println("error:", err)
		return false
	}

	c, _, err := websocket.DefaultDialer.DialContext(serviceContext(), u.String(), headers)
	if err != nil {
		log.Println("error:", err)
		return false
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(globalContext)
	defer cancel()

	done := make(chan struct{})

	serviceWorkers.start(func() {
		defer close(done)

		go func() {
			<-ctx.Done()
			_ = c.Close()
		}()

		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				ReceiverError(ctx, err)
				break
			}
			if mt == websocket.TextMessage && len(msg) >= 4 {
				com := string(msg[:4])
				switch com {
				case LauncherComBootDevice:
					var lms LaunchMessage
					if err := json.Unmarshal([]byte(msg[4:]), &lms); err != nil {
						log.Println("error:", err)
						break
					}
					p.Exec(&lms)
				default:
					log.Printf("debug: Recv %s", msg)
				}
			}
		}

		log.Println("info: レシーバの終了")
	})

	//	ticker := time.NewTicker(time.Second)
	//	defer ticker.Stop()

	log.Printf("debug: remote launcher loop in.")

	for {
		select {
		case msg := <-p.Com:
			if err := c.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
				log.Println("error: パケット通信エラー", err)
				return false
			}

		case <-done:
			return false

		case <-ctx.Done():
			log.Println("info: 終了通知を検出")

			// クローズメッセージを送信し、サーバーが接続を閉じるのを（タイムアウト付きで）待機することで
			// 接続を適切に閉じます。
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("error:", err)
				return true
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return true
		}
	}
}

func (p *Launcher) SockLauncherHandler(g *gin.Context) {
	if !acceptClientIdentity(g, false, HEADER_CLIENT_TVROCK_LAUNCHER) {
		return
	}

	uid, authenticated := acceptLauncherAuthentication(g)
	if !authenticated {
		return
	}

	c, err := upgrader.Upgrade(g.Writer, g.Request, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer c.Close()

	p.Lock()
	_, ok := p.Pool[uid]
	p.Unlock()
	if ok {
		log.Println("error: 既に接続中", uid)
		return
	}

	lau := LauncherConnection{
		HostName: uid,
		Com:      make(chan string, 64),
	}
	p.Lock()
	p.Pool[uid] = &lau
	p.Unlock()
	p.notifyConnected()

	defer func() {
		// 接続コンテキストはレシーバーを停止させる。BootDeviceがこのキューの
		// スナップショットを保持している可能性があるため、Comを閉じると
		// 送信処理と競合する恐れがある。
		p.Lock()
		delete(p.Pool, uid)
		p.Unlock()
		log.Println("debug: exit launcher. ", uid)
	}()

	ctx, cancel := context.WithCancel(globalContext)
	defer cancel()

	serviceWorkers.start(func() {
		defer cancel()

		go func() {
			<-ctx.Done()
			_ = c.Close()
		}()

		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				ReceiverError(ctx, err)
				break
			}

			if mt == websocket.TextMessage && len(msg) >= 4 {
				com := string(msg[:4])
				switch com {
				case LauncherComDeviceOn, LauncherComDeviceOff:
					p.applyStatus(string(msg))
				}
				log.Printf("debug: Recv: %s", msg)
			}
		}

		log.Println("info: レシーバの終了")
		/*
			lau.Com <- LauncherComExit
		*/
	})

	log.Printf("debug: master launcher loop in.")

	//	dev.Com <- "dev:" // 全デバイスを送る

L:
	for {
		select {
		case <-ctx.Done():
			break L

		case msg, ok := <-lau.Com:
			if !ok {
				break L
			}
			com := msg[:4]
			switch com {
			case LauncherComExit:
				break L
			default:
				if err := c.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
					log.Println("error:", err)
					if strings.HasPrefix(msg, LauncherComBootDevice) {
						p.execBootMessageLocally("launcher websocket write failed", string(msg[4:]))
					}
					break L
				}
			}
		}
	}
}
