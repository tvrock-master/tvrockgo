package tvrock

// DSMはTSパケットではなくCRCチェック済みの完全なセクションを転送します。
// セクションの組み立ては特定の認証済み接続に紐付くため、
// 2つのチューナー間でモジュールバージョンが混在することはありません。
import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"sort"
	"strings"
	"time"
)

const dsmModuleLimit = 2 * 1024 * 1024
const dsmMemoryLimit = 16 * 1024 * 1024

type DSMComponent struct {
	Pid             uint16
	ComponentTag    uint8
	DataComponentId uint16
	Info            string
}
type PacketDSM struct {
	Kind                 string
	PCRBase              *uint64
	PCRExtension         uint16
	TimeUnixMillis       *int64
	Sid, Tsid, Onid, Pid uint16
	Components           []DSMComponent
	Data                 string
}
type dsmModule struct {
	id                uint16
	version           byte
	size, blockSize   int
	name, contentType string
	compression       byte
	originalSize      uint32
	compressed        bool
	descriptor        string
	blocks            map[uint16][]byte
	received          int
	done              bool
	logoSaved         bool
	logoAckAt         time.Time
	ignored           bool
	files             []dsmFile
}
type dsmCarousel struct {
	download      uint32
	modules       map[uint16]*dsmModule
	dii           []byte
	returnToEntry bool
}

type dsmKey struct{ sid, tsid, pid uint16 }

type dsmReceiver struct {
	notify      func(string, string)
	saveLogo    func(PacketCDT) error
	ackLogo     func(*RockDevice, dsmLogoReceipt) bool
	diiSections map[dsmKey]*dsmDIISections
	streams     map[dsmKey]*dsmCarousel
	session     int64
	lastPublish time.Time
	dirty       bool
	service     uint
}

type dsmLogoReceipt struct {
	Sid        uint16 `json:"sid"`
	Onid       uint16 `json:"onid"`
	Tsid       uint16 `json:"tsid"`
	Pid        uint16 `json:"pid"`
	DownloadID uint32 `json:"downloadId"`
	ModuleID   uint16 `json:"moduleId"`
	Version    byte   `json:"version"`
}

type dsmFile struct {
	Location *string        `json:"contentLocation"`
	Type     map[string]any `json:"contentType"`
	Data     string         `json:"dataBase64"`
	HTML     string         `json:"html,omitempty"`
}

type dsmDIISections struct {
	transaction uint32
	version     byte
	last        byte
	parts       map[byte][]byte
}

var dsm16 = binary.BigEndian.Uint16
var dsm32 = binary.BigEndian.Uint32

func newDSMReceiver() *dsmReceiver {
	return &dsmReceiver{notify: deviceController.Notify,
		saveLogo:    receiveCDT,
		ackLogo:     sendDSMLogoReceipt,
		diiSections: make(map[dsmKey]*dsmDIISections),
		streams:     make(map[dsmKey]*dsmCarousel),
		session:     time.Now().UnixNano()}
}

func dsmCRC(b []byte) uint32 {
	c := uint32(0xffffffff)
	for _, v := range b {
		c ^= uint32(v) << 24
		for i := 0; i < 8; i++ {
			if c&0x80000000 != 0 {
				c = c<<1 ^ 0x04c11db7
			} else {
				c <<= 1
			}
		}
	}
	return c
}

func (r *dsmReceiver) receive(dev *RockDevice, tsp uint, p PacketDSM) error {
	if r.service != dev.CurrentService {
		// 1つのトランスポートストリームまたは接続内でサービスが変更される場合があります
		r.streams = make(map[dsmKey]*dsmCarousel)
		r.diiSections = make(map[dsmKey]*dsmDIISections)
		r.service = dev.CurrentService
		r.dirty = true
	}
	if p.Kind == "pmt" {
		return r.manifest(dev, tsp, p)
	}
	if p.PCRBase != nil {
		if *p.PCRBase >= 1<<33 || p.PCRExtension >= 300 {
			return fmt.Errorf("invalid PCR")
		}
		messages := []any{map[string]any{"type": "pcr", "pcrBase": *p.PCRBase, "pcrExtension": p.PCRExtension}}
		if p.TimeUnixMillis != nil {
			messages = append(messages, map[string]any{"type": "currentTime", "timeUnixMillis": *p.TimeUnixMillis})
		}
		return r.notifyEvent(dev, tsp, p, messages)
	}
	if len(p.Data) > 8192 || len(p.Components) > 32 || p.Pid >= 8191 || p.Sid == 0 {
		return fmt.Errorf("invalid DSM metadata")
	}
	var component *DSMComponent
	for i := range p.Components {
		if p.Components[i].Pid == p.Pid {
			component = &p.Components[i]
		}
	}
	if component == nil {
		return fmt.Errorf("DSM PID absent from PMT")
	}
	s, err := hex.DecodeString(p.Data)
	if err != nil || len(s) < 12 || len(s) != 3+int(dsm16(s[1:])&4095) || dsmCRC(s) != 0 {
		return fmt.Errorf("invalid DSM section/CRC")
	}
	if s[0] != 0x3b && s[0] != 0x3c && s[0] != 0x3d {
		return fmt.Errorf("invalid DSM table")
	}
	// 受信機向けダウンロードは、全受信機共通データのロゴだけを対象にする。
	// その他のコンポーネントとイベントは解析・保持しない。
	receiverDownload := component.DataComponentId == 0x0009
	if receiverDownload && ((component.ComponentTag != 0x79 && component.ComponentTag != 0x7a) || s[0] == 0x3d) {
		return nil
	}
	// イベントメッセージ・セクションは、独自のディスクリプタ・レイアウトを持っています
	if s[0] == 0x3d {
		event, err := parseDSMEvents(s, component.ComponentTag)
		if err != nil {
			return err
		}
		return r.notifyEvent(dev, tsp, p, []any{event})
	}
	if len(s) < 24 {
		return fmt.Errorf("short DSM download header")
	}
	for k := range r.diiSections {
		present := false
		for _, c := range p.Components {
			present = present || k.pid == c.Pid
		}
		if k.tsid != p.Tsid || (k.sid == p.Sid && !present) {
			delete(r.diiSections, k)
		}
	}
	for k := range r.streams {
		if k.tsid != p.Tsid {
			delete(r.streams, k)
			delete(r.diiSections, k)
			continue
		}
		if k.sid == p.Sid {
			found := false
			for _, c := range p.Components {
				if k.pid == c.Pid {
					found = true
				}
			}
			if !found {
				delete(r.streams, k)
				delete(r.diiSections, k)
				r.dirty = true
			}
		}
	}
	h := s[8 : len(s)-4]
	if h[0] != 0x11 || h[1] != 3 {
		return fmt.Errorf("invalid DSM header")
	}
	n := int(dsm16(h[10:]))
	adapt := int(h[9])
	if n < adapt || 12+n != len(h) {
		return fmt.Errorf("invalid DSM message length: message=%04x declared=%d actual=%d adaptation=%d", dsm16(h[2:]), n, len(h)-12, adapt)
	}
	body := h[12+adapt:]
	key := dsmKey{p.Sid, p.Tsid, p.Pid}
	switch dsm16(h[2:]) {
	case 0x1002:
		if s[0] != 0x3b {
			return fmt.Errorf("DII table mismatch")
		}
		old := r.streams[key]
		next, changed, err := r.collectDII(key, s, h, body)
		if err != nil {
			return err
		}
		if changed {
			// DIIメタデータは保持するが、実際に利用するデータのみを組み立てる。
			// 送信側は共有PIDをマルチプレックス上のすべてのサービスにルーティングする。
			selected := dev.CurrentService == tsp<<16|uint(p.Sid)
			for _, m := range next.modules {
				m.ignored = !(selected && component.isBML()) && !m.isLogo()
				if m.ignored {
					m.blocks = nil
					m.files = nil
					m.received = 0
					m.done = false
				} else if !m.done && m.blocks == nil {
					m.blocks = make(map[uint16][]byte)
				}
			}
			budget := 0
			for k, c := range r.streams {
				if k != key {
					for _, m := range c.modules {
						if !m.ignored {
							budget += m.size + int(m.originalSize)
						}
					}
				}
			}
			for _, m := range next.modules {
				if !m.ignored {
					budget += m.size + int(m.originalSize)
				}
			}
			if budget > dsmMemoryLimit || (old == nil && len(r.streams) >= 128) {
				// 拒否されたDIIを正常に受信したものとしてキャッシュしないでください
				delete(r.diiSections, key)
				return fmt.Errorf("DSM memory limit: requested=%d limit=%d streams=%d", budget, dsmMemoryLimit, len(r.streams))
			}
			r.streams[key] = next
			r.dirty = true
		}
	case 0x1003:
		if s[0] != 0x3c {
			return fmt.Errorf("DDB table mismatch")
		}
		c := r.streams[key]
		if c == nil || c.download != dsm32(h[4:]) {
			return nil
		}
		m, data, err := c.block(body)
		if err != nil {
			return err
		}
		if m != nil {
			if m.isLogo() {
				logos, err := parseDSMLogos(data, tsp)
				if err != nil {
					m.retryLogo()
					return err
				}
				for _, logo := range logos {
					if err := r.saveLogo(logo); err != nil {
						m.retryLogo()
						return err
					}
				}
				m.logoSaved = true
			} else if component.isBML() {
				files, err := dsmFiles(data, m.contentType)
				if err != nil {
					return fmt.Errorf("module=%04x version=%d name=%q type=%q bytes=%d prefix=%x: %w", m.id, m.version, m.name, m.contentType, len(data), data[:min(32, len(data))], err)
				}
				m.files = files
				r.dirty = true
			}
		}
		// 受領通知が失われても、再送されてきたDDBに対して再通知できる。
		if len(body) >= 6 && r.ackLogo != nil {
			if saved := c.modules[dsm16(body)]; saved != nil && saved.logoSaved && saved.version == body[2] && time.Since(saved.logoAckAt) >= 5*time.Second {
				receipt := dsmLogoReceipt{p.Sid, p.Onid, p.Tsid, p.Pid, c.download, saved.id, saved.version}
				if r.ackLogo(dev, receipt) {
					saved.logoAckAt = time.Now()
				}
			}
		}
	default:
		return nil
	}

	// スナップショットの再送は、新規に開かれた・あるいは再接続されたブラウザウィンドウに対しても行われます。
	// カルーセルによる継続的なトラフィックにより遅延していた公開データが確実にフラッシュ（送信）されます。
	if receiverDownload || dev.CurrentService&0xffff != uint(p.Sid) || dev.CurrentService>>16 != tsp {
		return nil
	}
	if (r.dirty && time.Since(r.lastPublish) >= 250*time.Millisecond) || time.Since(r.lastPublish) >= 15*time.Second {
		messages := r.snapshot(p)
		if len(messages) == 0 {
			return nil
		}
		tvp := dev.CurrentProgram
		if tvp == nil || tvp.Sid != dev.CurrentService {
			tvp = &TvProgram{Sid: dev.CurrentService}
		}
		if tvp != nil {
			messages = append([]any{map[string]any{
				"type":                "programInfo",
				"originalNetworkId":   p.Onid,
				"transportStreamId":   p.Tsid,
				"serviceId":           p.Sid,
				"networkId":           p.Onid,
				"eventId":             tvp.Eid,
				"eventName":           tvp.Title,
				"startTimeUnixMillis": tvp.Start * 1000,
				"durationSeconds":     tvp.End - tvp.Start,
				"indefiniteDuration":  false}},
				messages...,
			)
		}
		messages = append(messages, map[string]any{"type": "currentTime", "timeUnixMillis": time.Now().UnixMilli()})
		payload, err := json.Marshal(map[string]any{"DeviceID": dev.ID, "Sid": dev.CurrentService, "Session": fmt.Sprint(r.session), "Messages": messages})
		if err != nil {
			return err
		}
		r.notify("bml:", string(payload))
		r.lastPublish = time.Now()
		r.dirty = false
	}
	return nil
}

func sendDSMLogoReceipt(dev *RockDevice, receipt dsmLogoReceipt) bool {
	message, err := json.Marshal(receipt)
	if err != nil {
		return false
	}
	slot.RLock()
	queue, done, state := dev.Com, dev.requestDone, dev.State
	slot.RUnlock()
	if queue == nil || state == StateOFF {
		return false
	}
	select {
	case <-done:
		return false
	default:
	}
	select {
	case queue <- RequestMessage{ID: dev.ID, Command: "DSMModuleReceived", Message: string(message)}:
		return true
	default:
		// ロゴの受領通知で録画制御・受信ループを待たせない。
		return false
	}
}

func (m *dsmModule) retryLogo() {
	m.done, m.logoSaved = false, false
	m.received = 0
	m.blocks = make(map[uint16][]byte)
}

func (m *dsmModule) isLogo() bool {
	name := strings.TrimPrefix(m.name, "CS_")
	return len(name) == 7 && strings.HasPrefix(name, "LOGO-0") && name[6] >= '0' && name[6] <= '5'
}

func parseDII(b []byte, old *dsmCarousel) (*dsmCarousel, error) {
	bad := func() (*dsmCarousel, error) { return nil, fmt.Errorf("malformed DII") }
	if len(b) < 20 {
		return bad()
	}
	bs := int(dsm16(b[4:]))
	if bs == 0 || bs > 4066 {
		return bad()
	}
	pos := 18 + int(dsm16(b[16:]))
	if pos+2 > len(b) {
		return bad()
	}
	count := int(dsm16(b[pos:]))
	pos += 2
	if count > 256 {
		return bad()
	}
	c := &dsmCarousel{download: dsm32(b), modules: make(map[uint16]*dsmModule), dii: append([]byte(nil), b...)}
	for i := 0; i < count; i++ {
		if pos+8 > len(b) {
			return bad()
		}
		m := &dsmModule{id: dsm16(b[pos:]), size: int(dsm32(b[pos+2:])), version: b[pos+6], blockSize: bs, blocks: make(map[uint16][]byte)}
		n := int(b[pos+7])
		pos += 8
		if m.size <= 0 || m.size > dsmModuleLimit || pos+n > len(b) {
			return bad()
		}
		m.descriptor = string(b[pos : pos+n])
		for d := b[pos : pos+n]; len(d) > 0; {
			if len(d) < 2 || int(d[1])+2 > len(d) {
				return bad()
			}
			val := d[2 : 2+int(d[1])]
			switch d[0] {
			case 1:
				m.contentType = strings.TrimRight(string(val), "\x00")
			case 2:
				m.name = strings.TrimRight(string(val), "\x00")
			case 9, 0xc2:
				if len(val) != 5 {
					return bad()
				}
				m.compression = val[0]
				m.compressed = true
				m.originalSize = dsm32(val[1:])
				if (m.compression != 0 && m.compression != 0xff) || m.originalSize > dsmModuleLimit {
					return bad()
				}
			}
			d = d[2+int(d[1]):]
		}
		if old != nil && old.download == c.download {
			if prev := old.modules[m.id]; prev != nil && prev.version == m.version && prev.size == m.size && prev.blockSize == bs && prev.descriptor == m.descriptor {
				m = prev
			}
		}
		if _, ok := c.modules[m.id]; ok {
			return bad()
		}
		c.modules[m.id] = m
		pos += n
	}
	if pos+2 > len(b) {
		return bad()
	}
	n := int(dsm16(b[pos:]))
	pos += 2
	if pos+n != len(b) {
		return bad()
	}
	for d := b[pos:]; len(d) > 0; {
		if len(d) < 2 || int(d[1])+2 > len(d) {
			return bad()
		}
		if d[0] == 0xf0 && d[1] > 0 {
			c.returnToEntry = d[2]&0x80 != 0
		}
		d = d[2+int(d[1]):]
	}
	return c, nil
}

func (c *dsmCarousel) block(b []byte) (*dsmModule, []byte, error) {
	if len(b) < 6 {
		return nil, nil, fmt.Errorf("short DDB")
	}
	m := c.modules[dsm16(b)]
	if m == nil || m.ignored || m.version != b[2] || m.done {
		return nil, nil, nil
	}
	number := dsm16(b[4:])
	offset := int(number) * m.blockSize
	if offset >= m.size {
		return nil, nil, fmt.Errorf("DDB block out of range")
	}
	size := m.blockSize
	if size > m.size-offset {
		size = m.size - offset
	}
	if len(b)-6 != size {
		return nil, nil, fmt.Errorf("DDB block length mismatch")
	}
	if _, ok := m.blocks[number]; ok {
		return nil, nil, nil
	}
	m.blocks[number] = append([]byte(nil), b[6:]...)
	m.received += size
	if m.received != m.size {
		return nil, nil, nil
	}
	data := make([]byte, 0, m.size)
	for i := 0; i < (m.size+m.blockSize-1)/m.blockSize; i++ {
		data = append(data, m.blocks[uint16(i)]...)
	}
	m.blocks = nil
	m.done = true
	if m.compressed {
		zr, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, nil, err
		}
		data, err = io.ReadAll(io.LimitReader(zr, dsmModuleLimit+1))
		zr.Close()
		if err != nil || len(data) > dsmModuleLimit || len(data) != int(m.originalSize) {
			return nil, nil, fmt.Errorf("invalid compressed DSM module")
		}
	}
	return m, data, nil
}

func makeDSMFile(location *string, contentType string, data []byte) (dsmFile, error) {
	media, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return dsmFile{}, err
	}
	parts := strings.SplitN(media, "/", 2)
	if len(parts) != 2 {
		return dsmFile{}, fmt.Errorf("invalid MIME type")
	}
	parameters := []map[string]string{}
	for k, v := range params {
		parameters = append(parameters, map[string]string{"attribute": k, "originalAttribute": k, "value": v})
	}
	f := dsmFile{Location: location, Type: map[string]any{
		"type":            parts[0],
		"originalType":    parts[0],
		"subtype":         parts[1],
		"originalSubtype": parts[1],
		"parameters":      parameters},
		Data: base64.StdEncoding.EncodeToString(data),
	}
	if strings.Contains(media, "bml") || strings.Contains(media, "xml") ||
		(location != nil && (strings.HasSuffix(strings.ToLower(*location), ".bml") ||
			strings.HasSuffix(strings.ToLower(*location), ".xml"))) {
		f.HTML, _ = convertBML(data, params["charset"])
	}
	return f, nil
}

func (c DSMComponent) isBML() bool {
	return c.ComponentTag != 255 && (c.DataComponentId == 7 || c.DataComponentId == 11 || c.DataComponentId == 12 || c.DataComponentId == 13)
}

func dsmFiles(data []byte, contentType string) ([]dsmFile, error) {
	if contentType != "" && !strings.HasPrefix(strings.ToLower(contentType), "multipart/") {
		f, err := makeDSMFile(nil, contentType, data)
		return []dsmFile{f}, err
	}
	reader := bufio.NewReader(bytes.NewReader(data))
	headers, err := textproto.NewReader(reader).ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("invalid MIME module header")
	}
	media, params, err := mime.ParseMediaType(headers.Get("Content-Type"))
	if err != nil {
		return nil, err
	}
	if media != "multipart/mixed" || params["boundary"] == "" {
		return nil, fmt.Errorf("expected ARIB multipart/mixed module")
	}
	mr := multipart.NewReader(reader, params["boundary"])
	files := []dsmFile{}
	partCount := 0
	var lastHeaders []string
	for {
		part, err := mr.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid MIME part or boundary")
		}
		partCount++
		lastHeaders = lastHeaders[:0]
		for key := range part.Header {
			lastHeaders = append(lastHeaders, key)
		}
		sort.Strings(lastHeaders)
		if partCount > 256 {
			return nil, fmt.Errorf("too many DSM resources")
		}
		loc := part.Header.Get("Content-Location")
		if len(loc) > 1024 || strings.ContainsAny(loc, "\x00\r\n") {
			return nil, fmt.Errorf("invalid DSM resource location")
		}
		body, err := io.ReadAll(io.LimitReader(part, dsmModuleLimit+1))
		if err != nil || len(body) > dsmModuleLimit {
			return nil, fmt.Errorf("DSM resource too large")
		}
		// Content-Locationが空であることは、ファイルが存在しないことではなく、空の相対URIであることを意味する。
		// BMLリソースマップが解決できるよう、nullではなく""（空文字列）のままにしておくこと。
		// ヘッダーが欠落していても、アドレス指定可能な他の部分を破棄してはならない。
		if _, present := part.Header["Content-Location"]; !present {
			continue
		}
		f, err := makeDSMFile(&loc, part.Header.Get("Content-Type"), body)
		if err != nil {
			return nil, err
		}
		files = append(files, f)
	}
	if len(files) == 0 && partCount > 0 {
		return nil, fmt.Errorf("no addressable MIME resources: parts=%d lastHeaders=%q", partCount, lastHeaders)
	}
	return files, nil
}

func parseDSMLogos(b []byte, tsp uint) ([]PacketCDT, error) {
	bad := func() ([]PacketCDT, error) { return nil, fmt.Errorf("malformed DSM logo module") }
	if len(b) < 3 || b[0] > 5 {
		return bad()
	}
	logos := []PacketCDT{}
	pos := 3
	for i := 0; i < int(dsm16(b[1:])); i++ {
		if pos+3 > len(b) {
			return bad()
		}
		id := dsm16(b[pos:]) & 511
		count := int(b[pos+2])
		pos += 3
		if pos+count*6+2 > len(b) {
			return bad()
		}
		services := b[pos : pos+count*6]
		pos += count * 6
		size := int(dsm16(b[pos:]))
		pos += 2
		if size == 0 || pos+size > len(b) {
			return bad()
		}
		for j := 0; j < count; j++ {
			onid := dsm16(services[j*6:])
			space := tsp
			// BSとCSのロゴは、エンジニアリング・サービスのカルーセルを共有できます
			switch onid {
			case 4:
				space = uint(TuningSpaceBS)
			case 6, 7:
				space = uint(TuningSpaceCS)
			}
			logos = append(logos, PacketCDT{
				Sid:  uint32(space<<16) | uint32(dsm16(services[j*6+4:])),
				Onid: onid, LogoType: b[0],
				LogoId: id,
				Data:   hex.EncodeToString(b[pos : pos+size]),
			})
		}
		pos += size
	}
	if pos != len(b) {
		return bad()
	}
	return logos, nil
}

func bxmlInfo(info string) map[string]any {
	b, err := hex.DecodeString(info)
	if err != nil || len(b) < 1 {
		return nil
	}
	result := map[string]any{"transmissionFormat": b[0] >> 6, "entryPointFlag": b[0]&0x20 != 0}
	pos := 1
	if b[0]&0x20 != 0 {
		if len(b) < 2 {
			return nil
		}
		pos = 2
		major, minor := uint16(1), uint16(0)
		if b[1]&0x40 == 0 {
			if len(b) < 6 {
				return nil
			}
			major = dsm16(b[2:])
			minor = dsm16(b[4:])
			pos = 6
			if b[1]&0x80 != 0 {
				pos += 4
			}
		}
		result["entryPointInfo"] = map[string]any{
			"autoStartFlag":      b[0]&16 != 0,
			"documentResolution": b[0] & 15,
			"useXML":             b[1]&128 != 0,
			"defaultVersionFlag": b[1]&64 != 0,
			"independentFlag":    b[1]&32 != 0,
			"styleForTVFlag":     b[1]&16 != 0,
			"bmlMajorVersion":    major,
			"bmlMinorVersion":    minor,
		}
	}
	if b[0]>>6 == 0 && pos+2 <= len(b) {
		result["additionalAribCarouselInfo"] = map[string]any{
			"dataEventId":           b[pos] >> 4,
			"eventSectionFlag":      b[pos]&8 != 0,
			"ondemandRetrievalFlag": b[pos+1]&128 != 0,
			"fileStorableFlag":      b[pos+1]&64 != 0,
			"startPriority":         (b[pos+1] >> 5) & 1,
		}
	}
	return result
}

func (r *dsmReceiver) snapshot(p PacketDSM) []any {
	components := []any{}
	messages := []any{}
	for _, comp := range p.Components {
		if !comp.isBML() {
			continue
		}
		info := bxmlInfo(comp.Info)
		if info == nil {
			continue
		}
		components = append(components, map[string]any{
			"pid":             comp.Pid,
			"componentId":     comp.ComponentTag,
			"streamType":      13,
			"dataComponentId": comp.DataComponentId,
			"bxmlInfo":        info},
		)
		c := r.streams[dsmKey{p.Sid, p.Tsid, comp.Pid}]
		if c == nil {
			continue
		}
		ids := []int{}
		for id := range c.modules {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		list := []any{}
		downloaded := []any{}
		for _, id := range ids {
			m := c.modules[uint16(id)]
			list = append(list, map[string]any{"id": m.id, "version": m.version, "size": m.size})
			if len(m.files) > 0 {
				downloaded = append(downloaded, map[string]any{
					"type":        "moduleDownloaded",
					"componentId": comp.ComponentTag,
					"moduleId":    m.id,
					"version":     m.version,
					"dataEventId": c.download >> 28,
					"files":       m.files},
				)
			}
		}
		messages = append(messages, map[string]any{
			"type":              "moduleListUpdated",
			"componentId":       comp.ComponentTag,
			"dataEventId":       c.download >> 28,
			"modules":           list,
			"returnToEntryFlag": c.returnToEntry,
		})
		messages = append(messages, downloaded...)
	}
	if len(components) == 0 {
		return nil
	}
	return append([]any{map[string]any{"type": "pmt", "components": components}}, messages...)
}

// ストリームイベント記述子にはBMLのbeventで使用されるブロードキャスト・トリガーが含まれています。
func parseDSMEvents(s []byte, component uint8) (map[string]any, error) {
	if len(s) < 12 || s[0] != 0x3d {
		return nil, fmt.Errorf("invalid DSM stream-event section")
	}
	events := []any{}
	for d := s[8 : len(s)-4]; len(d) > 0; {
		if len(d) < 2 || int(d[1])+2 > len(d) {
			return nil, fmt.Errorf("truncated DSM stream descriptor")
		}
		v := d[2 : 2+int(d[1])]
		switch d[0] {
		case 0x17:
			if len(v) != 18 {
				return nil, fmt.Errorf("invalid NPT reference")
			}
			npt := int64(dsm32(v[10:]))
			if v[9]&1 != 0 {
				npt -= 1 << 32
			}
			events = append(events, map[string]any{"type": "nptReference", "postDiscontinuityIndicator": v[0]&128 != 0,
				"dsmContentId": v[0] & 127, "STCReference": uint64(v[1]&1)<<32 | uint64(dsm32(v[2:])),
				"NPTReference": npt, "scaleNumerator": int16(dsm16(v[14:])), "scaleDenominator": int16(dsm16(v[16:]))})
		case 0x40:
			if len(v) < 11 {
				return nil, fmt.Errorf("invalid general event")
			}
			if v[2] == 0 || v[2] == 2 {
				private := []int{}
				for _, b := range v[11:] {
					private = append(private, int(b))
				}
				ev := map[string]any{"type": "immediateEvent", "timeMode": v[2], "eventMessageGroupId": dsm16(v) >> 4,
					"eventMessageType": v[8], "eventMessageId": dsm16(v[9:]), "privateDataByte": private}
				if v[2] == 2 {
					ev["type"] = "nptEvent"
					ev["eventMessageNPT"] = uint64(v[3]&1)<<32 | uint64(dsm32(v[4:]))
				}
				events = append(events, ev)
			}
		}
		d = d[2+int(d[1]):]
	}
	return map[string]any{"type": "esEventUpdated", "componentId": component, "dataEventId": s[3] >> 4, "events": events}, nil
}

func (r *dsmReceiver) notifyEvent(dev *RockDevice, tsp uint, p PacketDSM, messages []any) error {
	if dev.CurrentService&0xffff != uint(p.Sid) || dev.CurrentService>>16 != tsp {
		return nil
	}
	payload, err := json.Marshal(map[string]any{"DeviceID": dev.ID, "Sid": dev.CurrentService, "Session": fmt.Sprint(r.session), "Mode": "event", "Messages": messages})
	if err != nil {
		return err
	}
	r.notify("bml:", string(payload))
	return nil
}

// DIIテーブルは複数のセクションにまたがることがあります。完全なモジュールリストのみをコミットしてください
func (r *dsmReceiver) collectDII(key dsmKey, s, h, body []byte) (*dsmCarousel, bool, error) {
	if s[6] > s[7] || s[7] >= 16 {
		return nil, false, fmt.Errorf("invalid DII section range")
	}
	set := r.diiSections[key]
	if set == nil || set.transaction != dsm32(h[4:]) || set.version != s[5] || set.last != s[7] {
		if set == nil && len(r.diiSections) >= 128 {
			return nil, false, fmt.Errorf("DII stream limit")
		}
		set = &dsmDIISections{transaction: dsm32(h[4:]), version: s[5], last: s[7], parts: make(map[byte][]byte)}
		r.diiSections[key] = set
	}
	if bytes.Equal(set.parts[s[6]], body) {
		return nil, false, nil
	}
	set.parts[s[6]] = append([]byte(nil), body...)
	if len(set.parts) != int(set.last)+1 {
		return nil, false, nil
	}
	old := r.streams[key]
	var combined *dsmCarousel
	for i := byte(0); i <= set.last; i++ {
		c, err := parseDII(set.parts[i], old)
		if err != nil {
			return nil, false, err
		}
		if combined == nil {
			combined = c
			continue
		}
		if combined.download != c.download {
			return nil, false, fmt.Errorf("DII download mismatch")
		}
		for id, m := range c.modules {
			if _, exists := combined.modules[id]; exists {
				return nil, false, fmt.Errorf("duplicate DII module")
			}
			combined.modules[id] = m
		}
		if len(combined.modules) > 256 {
			return nil, false, fmt.Errorf("DII module limit")
		}
	}
	return combined, true, nil
}

func (r *dsmReceiver) manifest(dev *RockDevice, tsp uint, p PacketDSM) error {
	if len(p.Components) > 32 {
		return fmt.Errorf("DSM component limit")
	}
	present := map[uint16]bool{}
	for _, c := range p.Components {
		present[c.Pid] = true
	}
	for key := range r.diiSections {
		if key.tsid != p.Tsid || (key.sid == p.Sid && !present[key.pid]) {
			delete(r.diiSections, key)
		}
	}
	for key := range r.streams {
		if key.tsid != p.Tsid || (key.sid == p.Sid && !present[key.pid]) {
			delete(r.streams, key)
			delete(r.diiSections, key)
			r.dirty = true
		}
	}
	if dev.CurrentService&0xffff != uint(p.Sid) || dev.CurrentService>>16 != tsp {
		return nil
	}
	// データコンポーネントが存在しない場合でもPMTを出力する：これにより古い画面が消去される
	messages := r.snapshot(p)
	if len(messages) != 0 {
		messages = messages[:1]
	} else {
		messages = []any{map[string]any{"type": "pmt", "components": []any{}}}
	}
	payload, err := json.Marshal(map[string]any{"DeviceID": dev.ID, "Sid": dev.CurrentService, "Session": fmt.Sprint(r.session), "Mode": "event", "Messages": messages})
	if err != nil {
		return err
	}
	r.notify("bml:", string(payload))
	return nil
}
