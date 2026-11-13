package tvrock

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

const (
	OnDevMsgDeviceInfo   = "DIF:"
	OnDevMsgStatus       = "STA:"
	OnDevMsgPF           = "CPF:"
	OnDevMsgSDT          = "SDT:"
	OnDevMsgCDT          = "CDT:"
	OnDevMsgDSM          = "DSM:"
	OnDevMsgEIT          = "EIT:"
	OnDevMsgEventRelay   = "EVR:"
	OnDevMsgCaption      = "CAP:"
	OnDevMsgStreamChange = "STC:"
	OnDevMsgCH2File      = "CH2:"
	OnDevMsgLoopBack     = "LTC:"
	OnDevMsgPacketInfo   = "PKI:"
	OnDevMsgRenameFile   = "RNF:"
	OnDevMsgStreamStatus = "STR:"
	OnDevMsgDebug        = "DBG:"
)

type PacketInfo struct {
	Pid      int64 `json:"pid"`
	Total    int64 `json:"total"`
	Drop     int64 `json:"drop"`
	Scramble uint  `json:"scramble"`
}
type PacketSDT struct {
	TableId     uint `json:"TableId"`
	Tsid        uint `json:"Tsid"`
	Onid        uint `json:"Onid"`
	Sid         uint `json:"Sid"`
	Version     uint `json:"Version"`
	Section     uint `json:"Section"`
	LastSection uint `json:"LastSection"`
	// 0x01	1	デジタルTVサービス
	// 0x02	2	デジタル音声サービス
	// 0xA1	161	臨時映像サービス
	// 0xA4	164	エンジニアリングサービス
	// 0xC0	192	データサービス（OneSegでワンセグと確定した局も含む）
	ServiceType         uint   `json:"ServiceType"`
	OneSeg              int    `json:"OneSeg"` // -1の時はまだ確定していない
	ProviderName        string `json:"ProviderName"`
	ServiceName         string `json:"ServiceName"`
	EitSchedule         uint   `json:"EitSchedule"`
	EitPresentFollowing uint   `json:"EitPresentFollowing"`
	RunningStatus       uint   `json:"RunningStatus"`
	FreeCA              uint   `json:"FreeCA"`
}

type PacketEventRelay struct {
	Onid      uint `json:"Onid"`
	Tsid      uint `json:"Tsid"`
	Sid       uint `json:"Sid"`
	Eid       uint `json:"Eid"`
	GroupType uint `json:"GroupType"`
	RelayOnid uint `json:"RelayOnid"`
	RelayTsid uint `json:"RelayTsid"`
	RelaySid  uint `json:"RelaySid"`
	RelayEid  uint `json:"RelayEid"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {

		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		if err == nil && strings.EqualFold(u.Host, r.Host) {
			return true
		}
		for _, allowed := range configure.AllowOrigins {
			if origin == allowed {
				return true
			}
		}
		if allowOriginPrefix(origin) {
			return true
		}

		return false
	},
}

// 呼び出し元の単一writerで使用する。送信の停滞をOSのTCPタイムアウトまで待たない。
func writeSocketMessage(c *websocket.Conn, message []byte) error {
	if err := c.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	return c.WriteMessage(websocket.TextMessage, message)
}

func (p *RockDevice) UpdateChannelInfo(cha []string) TuningSpace {
	tsp := TuningSpace(0)
	for _, v := range cha {
		var _tsp TuningSpace
		var err error
		if _tsp, err = station.ReadChFile(v, tsp); err != nil {
			log.Println("error:", err)
			break
		}
		previousTsp := tsp
		tsp = _tsp
		if tsp != TuningSpaceNone && tsp != previousTsp {
			switch tsp {
			case TuningSpaceTS, TuningSpaceOS:
				p.TunerType = TunerTypeTS
			case TuningSpaceBS, TuningSpaceCS:
				p.TunerType = TunerTypeBSCS
			}
			log.Printf("debug: Device: %s, TuningSpace: %d, TunerType: %d", p.RegistryCode, tsp, p.TunerType)
		}
	}
	station.updateServiceParents()

	if !p.sendReady() {
		return tsp
	}

	log.Println("debug: .ch2 file load finish.")

	if n, _ := station.UpdateStation(); n > 0 {
		deviceController.Notify(ControllerUpdateStation, "")
		deviceController.Notify(ControllerSendDevice, p.RegistryCode)
	}

	return tsp
}

// 局情報の読み込み結果とは独立して接続の初期化を完了する
func (p *RockDevice) sendReady() bool {
	slot.setConnectionState(p, StateReady)
	return p.sendServiceList()
}

// Readyの再送で使用対象サービスを更新する際は録画中などの状態を変更しない
func (p *RockDevice) sendServiceList() bool {
	stt := []uint{uint(p.TunerType)}
	sids := station.GetValidServiceID()
	for i, v := range sids {
		sids[i] = v & 0xffff
	}
	stt = append(stt, sids...)
	return slot.sendDevice(p, RequestMessage{ID: p.ID, Command: "Ready", Message: joinUintToString(stt, ",")})
}

func (p *RockDevice) UpdateServiceInfo(sdt []PacketSDT, tsp TuningSpace) {
	// ワンセグの対応先は同じTSIDの最小SIDのフルセグ候補とする。
	// 地上波の親子関係は登録後にキャッシュ全体で再判定する。
	parents := make(map[uint]uint)
	for _, v := range sdt {
		if v.OneSeg != 0 || v.ServiceType != 0x01 {
			continue
		}
		sid := uint(tsp)<<16 | v.Sid
		if parent, ok := parents[v.Tsid]; !ok || sid < parent {
			parents[v.Tsid] = sid
		}
	}
	_view := uint(1)
	registered := 0
	for i, v := range sdt {
		// ワンセグと確定した0xC0も登録する。通常のデータサービスは除外。
		if v.OneSeg < 0 || (v.ServiceType != 0x01 && !(v.OneSeg > 0 && v.ServiceType == 0xc0)) {
			log.Printf("debug: SDT service skipped: device=%s tsp=%d sid=%d tsid=%d type=0x%02x oneseg=%d", p.ID, tsp, v.Sid, v.Tsid, v.ServiceType, v.OneSeg)
			continue
		}

		_tsp := tsp
		psid := uint(0)
		// ワンセグ判定
		if v.OneSeg > 0 {
			_tsp = TuningSpaceOS
			psid = parents[v.Tsid]
		}
		sid := uint(_tsp)<<16 | uint(v.Sid)

		ch := uint(0x0)
		// 既に登録されているか調べる
		sta, b := station.Get(sid)
		if b {
			ch = sta.Ch
			if v.OneSeg == 0 {
				psid = sta.Parent
			}
		}
		if sid == psid {
			psid = 0
		}

		tvs := TvStation{
			Sid:    sid,
			Tsid:   v.Tsid,
			Ch:     ch,
			Tsp:    uint(_tsp),
			Parent: psid,
			Name:   v.ServiceName,
			View:   _view,
		}
		station.ModifyStation2(&tvs)
		registered++

		log.Println("debug: service found ", i, v)
	}
	if registered == 0 {
		log.Printf("warn: SDTに登録対象のサービスがありません: device=%s tsp=%d received=%d（判定済みTV・ワンセグが対象）", p.ID, tsp, len(sdt))
	}

	station.updateServiceParents()
	if n, err := station.UpdateStation(); n > 0 {
		deviceController.Notify(ControllerUpdateStation, "")
		deviceController.Notify(ControllerSendDevice, p.RegistryCode)
	} else if err != nil {
		log.Println("error: ", err)
	}
}

func (p *RockDevice) UpdateTVProgram(tvplist []TvProgram) {
	updated := false
	defer func() {
		if updated {
			scheduler.UpdateScheduler()
			deviceController.Notify(ControllerUpdateTimeTable, "")
		}
	}()
	batch := make([]TvProgram, 0, 32)
	pending := map[[2]uint]bool{}
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := tvprogram.InsertBatch(CurrentSlot, batch); err != nil {
			log.Printf("error: EPG一括保存 device:%s: %v", p.ID, err)
			for i := range batch {
				if err := tvprogram.Insert(CurrentSlot, &batch[i]); err != nil {
					log.Println("error:", err)
				} else {
					updated = true
				}
			}
		} else {
			updated = true
		}
		batch = batch[:0]
		clear(pending)
	}
	for _, tvp := range tvplist {
		tvp.Sid = (p.CurrentService & 0xffff0000) | (tvp.Sid & 0xffff)
		tvp.normalizeCommonEvent()

		// 局情報がなければ無視
		if _, ok := station.Get(tvp.Sid); !ok {
			continue
		}

		// 未定番組
		if tvp.Eid >= 0xfff0 && tvp.Eid <= 0xffff {
			log.Println("info: 緊急番組の検出", tvp.Sid, tvp.Eid, tvp.Title)
			continue
		}

		// 範囲外であれば無視
		threshold := Now().Unix() - configure.CurrentProgramThreshold
		if tvp.Start < threshold || tvp.End < threshold {
			log.Println("error: 取り扱い範囲外番組の検出", tvp)
			continue
		}

		// 現在の番組であれば無視
		if (p.CurrentProgram != nil && p.CurrentProgram.Sid == tvp.Sid && p.CurrentProgram.Eid == tvp.Eid) ||
			(p.NextProgram != nil && p.NextProgram.Sid == tvp.Sid && p.NextProgram.Eid == tvp.Eid) {
			continue
		}

		// Contentが無い状態で送らてくることもあるので以前にContentがあれば残す
		key := [2]uint{tvp.Sid, tvp.Eid}
		if pending[key] {
			flush()
		}
		var ctvp *TvProgram
		tvps, err := tvprogram.getStoredProgram(CurrentSlot, tvp.Sid, tvp.Eid)
		if err != nil {
			log.Println("error:", err)
			continue
		}
		if len(tvps) > 0 {
			ctvp = tvps[0]
			if tvp.Eid == ctvp.Eid {
				if len(tvp.Content) < 1 && len(ctvp.Content) > 0 {
					tvp.Content = ctvp.Content
				}
				// Preserve a known genre only when the incoming event has no genre.
				if tvp.CnibA == 15 && tvp.CnibB == 15 {
					tvp.CnibA = ctvp.CnibA
					tvp.CnibB = ctvp.CnibB
				}
			}
		}

		tvp.Title = html.UnescapeString(tvp.Title)
		tvp.Subtitle = html.UnescapeString(tvp.Subtitle)
		tvp.Content = html.UnescapeString(tvp.Content)

		if ctvp.IsMatchProgram(&tvp) {
			continue
		}

		batch = append(batch, tvp)
		pending[key] = true
		if len(batch) == cap(batch) {
			flush()
		}
	}
	flush()
}

func GetSelectedSchedule(sid, eid uint) (*RecordingSchedule, error) {
	schlist, err := scheduler.GetScheduleAll()
	if err != nil {
		return nil, err
	}
	for _, v := range schlist {
		if v.Validate && v.Sid == sid && v.Eid == eid {
			return v, nil
		}
	}
	return nil, nil
}

func OnidToTuningSpace(onid uint) uint {
	if onid >= 0x7e80 && onid <= 0x7fe8 {
		// 地上デジタル (地デジ)	0x7E80 〜 0x7FE8	32384 〜 32744	都道府県・地域・広域圏ごとに割り当て
		return uint(TuningSpaceTS)
	} else if onid == 4 {
		//BSデジタル	0x0004	4	各BSサービスで共通
		return uint(TuningSpaceBS)
	} else if onid == 6 || onid == 7 {
		// 広帯域CS	0x0006 / 0x0007	6 / 7	東経110度CS放送
		return uint(TuningSpaceCS)
	} else if onid == 11 || onid == 12 {
		// BS4K8K / CS4K	0x000B / 0x000C	11 / 12	4K8K高度デジタル放送
		return uint(TuningSpace4K)
	}
	return 0
}

func (p *RockDevice) UpdateEventRelay(tsp TuningSpace, evrlist []PacketEventRelay) error {
	// ロックを解放する前に（予約情報を）永続化しておく。これにより、スケジューラが処理を行う前であっても
	// 後続のパケット（他のデバイスからのパケットを含む）がその予約を認識できるようになる。
	scheduler.muEventRelay.Lock()
	changed := false
	defer func() {
		scheduler.muEventRelay.Unlock()
		if changed {
			scheduler.UpdateScheduler()
		}
	}()
	for _, evr := range evrlist {
		if evr.GroupType != 2 && evr.GroupType != 4 {
			continue
		}

		fromSid := uint(0)
		toSid := uint(0)

		switch evr.GroupType {
		case 2:
			// 同一ネットワーク内リレー (GroupType: 2):
			// 通常の地デジメインch→サブchへの移行
			fromSid = uint(tsp<<16) | evr.Sid
			toSid = uint(tsp<<16) | evr.RelaySid

		case 4:
			// 他ネットワークへのリレー (GroupType: 4):
			// 地上波の放送からBS放送へリレーする場合や、別の中継局・別周波数へ引き継ぐ場合です。

			// BSデジタル	0x0004	4	各BSサービスで共通
			// 広帯域CS	0x0006 / 0x0007	6 / 7	東経110度CS放送
			// BS4K8K / CS4K	0x000B / 0x000C	11 / 12	4K8K高度デジタル放送
			// 地上デジタル (地デジ)	0x7E80 〜 0x7FE8	32384 〜 32744	都道府県・地域・広域圏ごとに割り当て
			fromSid = OnidToTuningSpace(evr.Onid)<<16 | evr.Sid
			toSid = OnidToTuningSpace(evr.RelayOnid)<<16 | evr.RelaySid
		}

		sch, err := GetSelectedSchedule(fromSid, evr.Eid)
		if err != nil {
			return err
		}
		if sch == nil {
			continue
		}

		target, err := GetSelectedSchedule(toSid, evr.RelayEid)
		if err != nil {
			return err
		}
		if target != nil {
			// 既にあれば予約しない
			continue
		}

		tvp, err := tvprogram.Get(CurrentSlot, toSid, evr.RelayEid)
		if err != nil {
			return err
		}
		rs := scheduler.New()
		rs.Sid = toSid
		rs.Eid = evr.RelayEid
		if len(tvp) > 0 {
			p := tvp[0]
			rs.Title = p.Title
			rs.StartTime = p.Start
			rs.EndTime = p.End
		} else {
			rs.Title = "未定"
			// ターゲットの探索は、ディスクリプタを最初に受信した時点（リレーの数時間前である可能性がある）ではなく、
			// ソースの終了付近で開始する。
			rs.StartTime = max(Now().Unix(), sch.EndTime)
			rs.EndTime = rs.StartTime + int64((10*time.Minute)/time.Second)
		}
		if _, err := scheduler.Insert(CurrentSlot, rs); err != nil {
			return err
		}
		changed = true
		log.Print("info: イベントリレー情報を検出して録画予約を実行 ", fromSid, "->", rs.Sid, ":", rs.Eid, " ", rs.Title)
	}

	return nil
}

func (p *RockDevice) UpdateCaption(cap *PacketCaption) error {
	if p.CurrentProgram == nil || p.CurrentProgram.Sid == 0 || cap == nil {
		return nil
	}

	// HistoryIDはデータベースによって割り当てられるものであり、キャプションの送信者が割り当てることはありません
	cap.HistoryID = 0
	if configure.CaptionRecord {
		if err := rockDB.InsertCaption(CurrentSlot,
			p.CurrentProgram.Sid,
			p.CurrentProgram.Eid,
			p.CurrentProgram.Start,
			cap); err != nil {
			return err
		}
	}

	return SendCaption(p.ID, p.CurrentProgram, cap)
}

func getFetchTimeOutSec(tt TunerType, tsp TuningSpace) int {
	tosec := int(0)
	if tsp != TuningSpaceNone {
		switch tsp {
		case TuningSpaceTS:
			tosec = configure.FetchProgram.TimeoutTS
		case TuningSpaceBS:
			tosec = configure.FetchProgram.TimeoutBS
		case TuningSpaceCS:
			tosec = configure.FetchProgram.TimeoutCS
		case TuningSpaceOS:
			tosec = configure.FetchProgram.TimeoutOS
		case TuningSpace4K:
			tosec = configure.FetchProgram.TimeoutCS
		}
	} else {
		switch tt {
		case TunerTypeTS:
			tosec = configure.FetchProgram.TimeoutTS
		case TunerTypeBSCS:
			tosec = configure.FetchProgram.TimeoutCS
		case TunerType4K:
			tosec = configure.FetchProgram.TimeoutCS
		}
	}
	return tosec
}

func SockDeviceTerminal(g *gin.Context) {
	if !acceptClientIdentity(g, false, HEADER_CLIENT_TVROCK_DTV, HEADER_CLIENT_TVROCK_DTV_SIBLING) {
		return
	}

	if g.GetHeader(HEADER_CLIENT_TVROCK_DTV_PARENT) != "" {
		if playback.receiver(g) {
			return
		}
		remoteWatching.receiver(g)
		return
	}

	c, err := upgrader.Upgrade(g.Writer, g.Request, nil)
	if err != nil {
		log.Println("error: upgradeエラー", err)
		return
	}
	defer c.Close()

	auth := g.Request.Header.Get("Authorization")
	if auth == "" {
		log.Println("error: 認証の失敗", g.Request.RemoteAddr)
		return
	}
	token := strings.Split(auth, " ")

	if len(token) < 2 {
		log.Println("error: 認証の失敗", g.Request.RemoteAddr)
		return
	}

	rcode := token[1]
	dev, ok := slot.GetByRID(rcode)
	if !ok {
		log.Println("error: 使用できないデバイスコード", rcode)
		return
	}

	ctx, cancel := context.WithCancel(globalContext)
	defer cancel()

	queue := make(chan RequestMessage, max(1, configure.DeviceRequestQueueSize))
	isReconnect, accepted := slot.attachConnection(dev, queue, ctx.Done())
	if !accepted {
		log.Println("warn: 使用中のデバイス接続を拒否", dev.ID)
		return
	}
	defer func() {
		// 他の送信者がキューを保持している可能性があります。キャンセルを行うとそれらが解放されます。
		cancel()
		slot.detachConnection(dev, queue)
		deviceController.Notify(ControllerSendDevice, dev.RegistryCode)
	}()

	log.Println("info: デバイスの接続", dev.ID, dev.RegistryCode)

	deviceController.Notify(ControllerSendDevice, dev.RegistryCode)

	// ループバック
	select {
	case queue <- RequestMessage{ID: dev.ID, Command: "LoopBack", Message: fmt.Sprintf("%s%d", OnDevMsgLoopBack, Now().UnixNano())}:
	case <-ctx.Done():
		return
	}

	if isReconnect {
		log.Println("info: デバイスの再接続を検出", dev.ID, dev.RegistryCode)
	}

	tsp := TuningSpaceNone
	dsm := newDSMReceiver()

	epg := newDeviceEPGQueue(dev.ID)
	if !serviceWorkers.start(func() { epg.run(ctx) }) {
		return
	}
	readFinished := make(chan struct{})
	if !serviceWorkers.start(func() {
		defer close(readFinished)
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
				// CurrentPF
				switch com {

				// デバイス情報
				case OnDevMsgDeviceInfo:
					eventCount := dev.DeviceInfo.Events
					if err := json.Unmarshal(msg[4:], &dev.DeviceInfo); err != nil {
						log.Println("error:", err)
						break
					}

					// 遅延を計算
					if dev.DeviceInfo.UnixTime > 0 {
						t1 := time.Now().UnixNano()
						t2 := dev.DeviceInfo.UnixTime * int64(time.Second)
						dev.DeviceInfo.Delay = t2 - t1
					}

					// イベント数が一定時間以上増えない場合は検索予約を更新する
					if eventCount < dev.DeviceInfo.Events {
						dev.EventsAt = Now().Unix()
					}

					timeoutSec := getFetchTimeOutSec(dev.TunerType, tsp)
					if timeoutSec > 0 && dev.EventsAt > 0 && Now().Unix()-dev.EventsAt >= int64(timeoutSec) {
						if err := scheduler.AddAllKeywordReserveSchedules(); err != nil {
							log.Println("error: ", err)
						}
						dev.EventsAt = 0
					}

					delay := dev.DeviceInfo.Delay     // float32(dev.DeviceInfo.Delay/int64(time.Millisecond)) * 0.001
					latency := dev.DeviceInfo.Latency // float32(dev.DeviceInfo.Latency/int64(time.Millisecond)) * 0.001
					log.Printf("debug: Device Info: uxtm=%s(now=%s), delay=%d, latency=%d, events=%d, ct=%d, scard=%d, signal=%d, bitrate=%d, error=%d, scramble=%d",
						time.Unix(dev.DeviceInfo.UnixTime, 0).String(),
						Now().String(),
						delay,
						latency,
						dev.DeviceInfo.Events,
						dev.DeviceInfo.Ct,
						dev.DeviceInfo.SmartCard,
						dev.DeviceInfo.Signal,
						dev.DeviceInfo.Bitrate,
						dev.DeviceInfo.Error,
						dev.DeviceInfo.Scramble,
					)

				// ステータス
				case OnDevMsgStatus:
					s, err := strconv.ParseInt(string(msg[4:]), 10, 32)
					if err != nil {
						log.Println("error:", err)
						break
					}
					ss := DeviceState(s)
					if ss >= StateOFF && ss <= StateShutdown {
						slot.setConnectionState(dev, ss)
						deviceController.Notify(ControllerSendDevice, dev.RegistryCode)
					}

				// PF情報
				case OnDevMsgPF:
					var cpf []TvProgram
					if err := json.Unmarshal(msg[4:], &cpf); err != nil {
						log.Println("error:", err)
						break
					}

					for i := range 2 {
						cpf[i].Sid = (dev.CurrentService & 0xffff0000) | (cpf[i].Sid & 0xffff)
						cpf[i].normalizeCommonEvent()
						cpf[i].Title = html.UnescapeString(cpf[i].Title)
						cpf[i].Subtitle = html.UnescapeString(cpf[i].Subtitle)
						cpf[i].Content = html.UnescapeString(cpf[i].Content)
						cpf[i].CnibA = 0
						cpf[i].CnibB = 0
					}

					if cpf[0].Sid != 0 && cpf[0].Eid != 0 {
						log.Printf("debug: 現在の番組を検出 %d:%d %s, %s, %s", cpf[0].Sid, cpf[0].Eid, cpf[0].Title, time.Unix(cpf[0].Start, 0), time.Unix(cpf[0].End, 0))
					}
					if cpf[1].Sid != 0 && cpf[1].Eid != 0 {
						log.Printf("debug: 次の番組を検出 %d:%d %s, %s, %s", cpf[1].Sid, cpf[1].Eid, cpf[1].Title, time.Unix(cpf[1].Start, 0), time.Unix(cpf[1].End, 0))
					}

					_, err := dev.UpdatePFRecordings(&cpf[0], &cpf[1])
					if err != nil {
						log.Println("error:", err)
					}

				// サービス情報
				case OnDevMsgSDT:
					var svlist []PacketSDT
					if err := json.Unmarshal(msg[4:], &svlist); err != nil {
						log.Printf("error: SDT decode: device=%s tsp=%d: %v", dev.ID, tsp, err)
						break
					}

					if tsp == TuningSpaceNone && len(svlist) > 0 {
						onid := svlist[0].Onid
						allSame := true
						// 他ストリームのSDTでは接続全体の放送種別を確定しない。
						for _, sv := range svlist {
							if sv.TableId != 0x42 || sv.Onid != onid {
								allSame = false
								break
							}
						}
						if allSame {
							tsp = TuningSpace(OnidToTuningSpace(onid))
						}
					}

					// 地上波・BS以外はサブチャンネルの情報を登録しない
					if tsp != TuningSpaceTS && tsp != TuningSpaceBS {
						log.Printf("debug: SDT ignored: device=%s tsp=%d（地上波・BS以外）", dev.ID, tsp)
						break
					}

					before := station.GetValidServiceID()
					dev.UpdateServiceInfo(svlist, tsp)
					after := station.GetValidServiceID()
					slices.Sort(before)
					slices.Sort(after)
					// .ch2にないサブチャンネルも、SDTで見つかり次第EPG送信対象へ追加する。
					// ReadyでSDTも再送されるため一覧が変わった時だけ返し、録画状態は維持する。
					if !slices.Equal(before, after) {
						dev.sendServiceList()
					}

				// EIT情報
				case OnDevMsgEIT:
					var tvplist []TvProgram
					if err := json.Unmarshal(msg[4:], &tvplist); err != nil {
						log.Println("error:", err)
						break
					}

					epg.receive(dev, tvplist, len(msg))

				// イベントリレー
				case OnDevMsgEventRelay:
					var evrlist []PacketEventRelay
					if err := json.Unmarshal(msg[4:], &evrlist); err != nil {
						log.Println("error:", err)
						break
					}

					if err := dev.UpdateEventRelay(tsp, evrlist); err != nil {
						log.Println("error:", err)
					}

				// 局ロゴ
				case OnDevMsgDSM:
					var packet PacketDSM
					if len(msg) > 32768 {
						break
					}
					if err := json.Unmarshal(msg[4:], &packet); err != nil {
						log.Println("error: DSM:", err)
						break
					}
					if err := dsm.receive(dev, uint(tsp), packet); err != nil {
						log.Printf("error: DSM: device=%s sid=%d tsid=%d pid=%04x components=%v: %s", dev.ID, packet.Sid, packet.Tsid, packet.Pid, packet.Components, strconv.QuoteToASCII(err.Error()))
					}

				case OnDevMsgCDT:
					var logo PacketCDT
					if err := json.Unmarshal(msg[4:], &logo); err != nil {
						log.Println("error: CDT:", err)
						break
					}
					logo.Sid = uint32(tsp<<16) | (logo.Sid & 0xffff)
					if err := receiveCDT(logo); err != nil {
						log.Println("error: CDT:", err)
					}

					log.Println(logo)

				// 字幕
				case OnDevMsgCaption:
					var cap PacketCaption
					if err := json.Unmarshal(msg[4:], &cap); err != nil {
						log.Println("error:", err)
						break
					}

					if err := dev.UpdateCaption(&cap); err != nil {
						log.Println("error:", err)
						break
					}

				// .ch2ファイル読み取り
				case OnDevMsgCH2File:
					chd := strings.Replace(strings.Replace(string(msg[4:]), "\n", "", -1), "\r", "", -1)
					if chd == "file not found" {
						log.Printf("error: .ch2ファイルが見つかりません: device=%s", dev.ID)

						log.Printf("warn: 不定選局状態でデバイスを起動します（.ch2未読込、局情報は既存DB・SDTで更新）: device=%s", dev.ID)
						// .ch2による種別判定ができないため設定値を使う。
						// 以後はSTC通知で現在の放送種別を更新する。
						if dev.TunerType == TunerTypeTS {
							tsp = TuningSpaceTS
						} else if dev.TunerType == TunerTypeBSCS {
							tsp = TuningSpaceBS
						} else if dev.TunerType == TunerType4K {
							tsp = TuningSpace4K
						}
						dev.sendReady()
						deviceController.Notify(ControllerSendDevice, dev.RegistryCode)
						break
					}
					if len(chd) < 4 {
						log.Println("error: 使用できない.ch2ファイル", chd)
						deviceController.Notify(ControllerSendDevice, dev.RegistryCode)
						break
					}
					cha := strings.Split(chd[4:], "|")
					tsp = dev.UpdateChannelInfo(cha)
					//					DeviceNotify(dev.RegistryCode)

				// ストリーム切り替え
				case OnDevMsgStreamChange:
					dsm = newDSMReceiver()
					sst := strings.Split(string(msg[4:]), ",")
					if len(sst) < 2 {
						log.Println("error: 選局に失敗しました", msg[4:])
						break
					}
					channel, err := strconv.ParseUint(sst[0], 10, 32)
					if err != nil {
						log.Println("error:", err)
						break
					}
					sid, err := strconv.ParseUint(sst[1], 10, 32)
					if err != nil {
						log.Println("error:", err)
						break
					}

					slot.Lock()
					dev.CurrentService = calcLocalSID(uint(channel), uint(sid))
					tsp = TuningSpace(dev.CurrentService >> 16)
					slot.Unlock()

					deviceController.Notify(ControllerSendDevice, dev.RegistryCode)

					log.Printf("debug: デバイスの選局を受信 %s channel:%d, tsp:%d sid:%d", dev.ID, channel, dev.CurrentService>>16, dev.CurrentService&0xffff)

				// 録画ファイル名変更
				case OnDevMsgRenameFile:
					log.Printf("info: 録画ファイル名の変更 %s, %s", dev.ID, string(msg[4:]))
					if sch, b := scheduler.GetCurrentRecording(dev.ID); b {
						sch.FileName = string(msg[4:])
						if err := scheduler.FileNameEx(CurrentSlot, sch.ID, sch.FileName); err != nil {
							log.Println("error:", err)
						}
					}

				// ストリーム状態
				case OnDevMsgStreamStatus:
					remoteWatching.statusWithQuality(dev.RegistryCode, msg[4:], dev.CurrentService, dev.DeviceInfo.Error)

				// ループバック・遅延確認
				case OnDevMsgLoopBack:
					lbk := strings.Split(string(msg[4:]), ",")
					n := len(lbk)
					if n < 1 || n > 2 {
						log.Println("error: loopback value does not match.")
						break
					}

					latency, err := strconv.ParseInt(lbk[0], 10, 64)
					if err != nil {
						log.Println("error:", err)
						break
					}
					dev.DeviceInfo.Latency = Now().UnixNano() - latency

					if n == 1 {
						log.Println("warn: unknown version")
						break
					}
					log.Println("info: dtv client version", lbk[1])

				case OnDevMsgPacketInfo:
					var pki []PacketInfo
					if err := json.Unmarshal(msg[4:], &pki); err != nil {
						log.Println("error:", err)
						break
					}
					dev.PacketInfo = pki

				case OnDevMsgDebug:
					log.Printf("debug: [%s]: %s\n", rcode, string(msg[4:]))
				}
			}
		}

		log.Printf("info: レシーバの終了 %s, %s", dev.ID, dev.RegistryCode)
	}) {
		return
	}
	defer func() {
		cancel()
		_ = c.Close()
		<-readFinished
	}()
L:
	for {
		select {
		case <-ctx.Done():
			log.Println("debug: exit, context done")
			break L
		case com := <-queue:
			if com.shutdown != nil {
				if err := com.shutdown.write(c, dev, queue); err != nil {
					log.Printf("warn: デバイス終了指示の送信失敗 device:%s: %v", dev.ID, err)
					break L
				}
				continue
			}
			if com.Command == "exit" {
				log.Println("info: 終了を検出", com.Message)
				break L
			}

			js, err := json.Marshal(com)
			if err != nil {
				log.Println("error:", err)
				break
			}

			err = writeSocketMessage(c, js)
			if err != nil {
				log.Println("error: メッセージ受信エラー", err)
				break L
			}

			log.Printf("debug: Send %s, %s", com.ID, com.Command)
		}
	}
}

func calcLocalSID(channel, sid uint) uint {
	// 上位16bit=1は地上波（下位は従来のチャンネル値）。
	// フルセグ・ワンセグの区別には.ch2・SDTから登録した局情報を使う。
	// BS/CSの通知は上位16bitが放送種別、下位16bitがTSID。
	// TSID未取得でもSIDと放送種別だけで局を特定でき、周波数表は不要。
	band := TuningSpace(channel >> 16)
	if band == TuningSpaceBS || band == TuningSpaceCS {
		return uint(band)<<16 | sid&0xffff
	}
	if band == TuningSpaceTS {
		// 地上波通知のSIDはフルセグ・ワンセグだけで照合する。
		// 同じSIDのBS/CS局や物理チャンネル番号に一致する周波数表は使わない。
		localSID := sid & 0xffff
		found := false
		for _, tsp := range []TuningSpace{TuningSpaceTS, TuningSpaceOS} {
			candidate := uint(tsp)<<16 | sid&0xffff
			if _, ok := station.Get(candidate); ok {
				if found {
					// 両種別に同じSIDがある場合は、種別未確定のまま返す。
					return sid & 0xffff
				}
				localSID, found = candidate, true
			}
		}
		return localSID
	}

	// 更新前のクライアントの通知（地上波は物理チャンネル番号のみ）も扱う。
	tsp := uint(0)
	tsid := uint(channel & 0xffff)
	_sid := uint(sid & 0xffff)

	for _, v := range configure.TsidList {
		if v.TSID == tsid {
			tsp = v.TSP
			break
		}
	}

	if tsp == 0 {
		for _, v := range station.GetStationList() {
			if v.Sid&0xffff == _sid {
				tsp = v.Tsp
				break
			}
		}
	}

	return tsp<<16 | _sid
}
