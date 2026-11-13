package tvrock

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rs/xid"
)

const (
	ControllerSendUID         = "uid:"
	ControllerSendDevice      = "dev:"
	ControllerResetSearch     = "rss:"
	ControllerSendSearch      = "res:"
	ControllerSendStorageInfo = "sti:"
	ControllerSendExit        = "ext:"
	ControllerSendCaption     = "cap:"

	ControllerUpdateTimeTable           = "utt:"
	ControllerUpdateStation             = "ust:"
	ControllerUpdateSchedule            = "usc:"
	ControllerUpdateRecorderSetting     = "urs:"
	ControllerUpdatePowerManagerSetting = "ups:"
	ControllerUpdateKeywordReserve      = "ukr:"

	ControllerComDeviceAll   = "REL:"
	ControllerComBootDevice  = "BDV:"
	ControllerComRequest     = "REQ:"
	ControllerCommand        = "COM:"
	ControllerComRemoteWatch = "WAT:"

	ControllerComRecordStart = "RecordStart"
	ControllerComRecordStop  = "RecordStop"
)

type DeviceController struct {
	Handler  map[string]*DevConQueue `json:"-"`
	Launcher *Launcher
	sync.RWMutex
}

var deviceController DeviceController

func (p *DeviceController) Initialize(lau *Launcher) error {

	p.Handler = map[string]*DevConQueue{}
	p.Launcher = lau

	return nil
}

func (p *DeviceController) Notify(com, msg string) {
	p.RLock()
	handlers := make([]*DevConQueue, 0, len(p.Handler))
	for _, val := range p.Handler {
		handlers = append(handlers, val)
	}
	p.RUnlock()

	for _, val := range handlers {
		val.send(com + msg)
	}
}

func (p *DeviceController) NotifyTo(uid, com, msg string) {
	p.RLock()
	val, ok := p.Handler[uid]
	p.RUnlock()
	if ok {
		val.send(com + msg)
	}
}

func arraySearch(arr []string, str string) bool {
	for _, v := range arr {
		if v == "" || v == str {
			return true
		}
	}
	return false
}

func (p *DeviceController) SockControllerHandler(g *gin.Context) {
	if !acceptClientIdentity(g, true, HEADER_CLIENT_TVROCK_APP, HEADER_CLIENT_TVROCK_ROCKBAR) {
		return
	}

	c, err := upgrader.Upgrade(g.Writer, g.Request, nil)
	if err != nil {
		log.Print("error: upgradeエラー", err)
		return
	}
	defer c.Close()

	// クライアント個別識別のためのユニークID
	uid := xid.New().String()
	log.Println("debug: entry device ", uid)

	dev := DevConQueue{
		Com:      make(chan string, 4096),
		Captions: make(chan string, 64),
		Done:     make(chan struct{}),
	}
	p.Lock()
	p.Handler[uid] = &dev
	p.Unlock()

	defer func() {
		// Handlerから離れた後もパブリッシャーがこのキューを保持し続ける可能性があります。
		// 送信先チャネルをクローズすることなく保留中の送信をキャンセルします。
		close(dev.Done)
		p.Lock()
		delete(p.Handler, uid)
		p.Unlock()
		log.Println("debug: exit device controller. ", uid)
		log.Printf("info: クライアント通知の終了 uid:%s stats:%+v", uid, dev.stats())
	}()

	ctx, cancel := context.WithCancel(globalContext)
	defer cancel()
	defer remoteWatching.disconnect(uid)
	defer playback.disconnect(uid)
	identity := g.GetHeader(HEADER_CLIENT_TVROCK_CLIENT)
	if identity == "" {
		identity = g.Query(HEADER_CLIENT_TVROCK_CLIENT)
	}
	if identity == "" {
		identity = g.GetHeader("User-Agent")
	}
	isRockbar := validClientIdentity(identity, HEADER_CLIENT_TVROCK_ROCKBAR)

	serviceWorkers.start(func() {
		defer cancel()

		go func() {
			select {
			case <-ctx.Done():
			case <-dev.failure():
				log.Printf("warn: クライアント通知キューの超過により切断 uid:%s", uid)
				cancel()
			}
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
				case "PLY:":
					if isRockbar {
						playback.command(uid, watchPeer(g.Request.RemoteAddr), msg[4:])
					}
				case ControllerComRemoteWatch:
					if strings.HasPrefix(g.Request.Header.Get("User-Agent"), "TvRockRockbar/") {
						remoteWatching.command(uid, watchPeer(g.Request.RemoteAddr), msg[4:])
					}

				case ControllerComDeviceAll: // デバイス情報再送コマンド
					playbackNotify(uid, map[string]any{"action": "state", "devices": playback.devicesFor(uid)})
					dev.send(ControllerSendDevice) // 全デバイスを送る

				case ControllerComBootDevice: // launcherへ転送
					if p.Launcher != nil {
						p.Launcher.BootDevice(string(msg[4:]))
					}

				case ControllerComRequest: // deviceserverを経由してdtvclientへ転送
					var req RequestMessage
					if err := json.Unmarshal(msg[4:], &req); err != nil {
						log.Println("error:", err)
						break
					}
					if req.Command == "StreamStart" ||
						req.Command == "StreamStop" ||
						req.Command == "GetStreamStatus" ||
						req.Command == "StreamPace" ||
						req.Command == "ReceiverService" {
						break
					}
					if d, ok := slot.GetByRID(req.ID); ok {
						slot.RLock()
						recording := d.State == StateRecording
						slot.RUnlock()
						if req.Command == "StreamChange" && recording {
							break
						}
						if req.Command == "PowerOff" && keepWatchDevice(d.ID, Now()) {
							break
						}
					}
					if req.Command == "Capture" && remoteWatching.captureDevice(req.ID) {
						break
					}
					slot.SendByRID(&req)

				case ControllerCommand:
					var req RequestMessage
					if err := json.Unmarshal(msg[4:], &req); err != nil {
						log.Println("error:", err)
						break
					}
					switch req.Command {

					case ControllerComRecordStart:
						d, ext := slot.GetByRID(req.ID)
						if !ext {
							log.Println("error: デバイスが見つかりません", req.ID)
							break
						}
						if d.CurrentService == 0 || d.CurrentProgram == nil {
							log.Println("error: 使用できないサービス", d.CurrentService)
							break
						}
						scheduler.RecordingNow(req.Message, d.CurrentService, d.CurrentProgram.Eid)

					case ControllerComRecordStop:
						_, ext := slot.GetByRID(req.ID)
						if !ext {
							log.Println("error: デバイスが見つかりません", req.ID)
							break
						}
						scheduler.RecordingStop(req.Message)

					default:
						log.Println("error: 使用できないコマンド", msg)
					}
				}
			}
		}

		log.Printf("info: レシーバの終了")
	})

	log.Printf("debug: device controller loop in.")

	dev.send(ControllerSendUID + uid) // uidを送る
	//	dev.Com <- ControllerSendDevice    // 全デバイスを送る

	SendStorageInfo()
	write := func(message []byte) error {
		if err := writeSocketMessage(c, message); err != nil {
			return err
		}
		dev.sent.Add(1)
		dev.bytes.Add(uint64(len(message)))
		return nil
	}
L:
	for {
		messages, ok := dev.receive(ctx)
		if !ok {
			break
		}
		for _, msg := range messages {
			if len(msg) < 4 {
				continue
			}
			com := string(msg[:4])
			switch com {
			case ControllerSendExit:
				break L

			case ControllerUpdateTimeTable, ControllerUpdateStation, ControllerUpdateSchedule, ControllerUpdateRecorderSetting, ControllerUpdatePowerManagerSetting:
				err = write([]byte(com))
				if err != nil {
					log.Println("error:", err)
					break L
				}

			case ControllerSendDevice:
				rid := strings.Split(string(msg[4:]), ",")
				writeFailed := false
				devices := []*RockDevice{}
				slot.RLock()
				for _, val := range slot.Device {
					if arraySearch(rid, val.RegistryCode) {
						copy := *val
						devices = append(devices, &copy)
					}
				}
				slot.RUnlock()
				for _, val := range devices {
					if resolved, resolveErr := tvprogram.resolveDevicePrograms(val); resolveErr == nil {
						val = resolved
					} else {
						log.Printf("error: device common event: %v", resolveErr)
					}
					js, err := json.Marshal(val)
					if err != nil {
						log.Println("error:", err)
						break
					}
					err = write(append([]byte(ControllerSendDevice), js...))
					if err != nil {
						log.Println("error:", err)
						writeFailed = true
						break
					}
				}
				if writeFailed {
					break L
				}

			default:
				err = write([]byte(msg))
				if err != nil {
					log.Println("error:", err)
					break L
				}
			}
		}
	}
}
