package tvrock

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func dsmTestDII(data []byte, desc []byte, version byte) []byte {
	b := make([]byte, 20)
	binary.BigEndian.PutUint32(b, 0x10000001)
	binary.BigEndian.PutUint16(b[4:], 32)
	b[19] = 1
	b = append(b, 0, 0, 0, 0, 0, 0, version, byte(len(desc)))
	binary.BigEndian.PutUint32(b[22:], uint32(len(data)))
	b = append(b, desc...)
	return append(b, 0, 0)
}
func dsmTestBlock(data []byte, n int, version byte) []byte {
	end := min((n+1)*32, len(data))
	b := []byte{0, 0, version, 0, byte(n >> 8), byte(n)}
	return append(b, data[n*32:end]...)
}
func dsmTestEntity() []byte {
	return []byte("Content-Type: multipart/mixed; boundary=tvrock\r\n\r\n--tvrock\r\nContent-Type: text/X-arib-bml; charset=UTF-8\r\nContent-Location: startup.bml\r\n\r\n" + `<?xml version="1.0" encoding="UTF-8"?><bml><head><title>DSM test</title><style>body{width:960px;height:540px;background-color:#10243a;color:#ffffff}p{position:absolute;left:48px;top:60px;width:850px;height:64px;font-size:32px}#controls{top:160px}a{font-size:28px;nav-index:0;color:#ffffff;background-color:#245a86}</style><script type="text/ecmascript"><![CDATA[function change(){document.getElementById("message").firstChild.data="REMOTE OK";}]]></script></head><body><p id="message">DSM-CC BML TEST</p><p id="controls"><a id="action" onclick="change()">PRESS ENTER</a></p></body></bml>` + "\r\n--tvrock--\r\n")
}
func TestDSMOutOfOrderDuplicateAndVersion(t *testing.T) {
	data := dsmTestEntity()
	c, err := parseDII(dsmTestDII(data, nil, 3), nil)
	if err != nil {
		t.Fatal(err)
	}
	count := (len(data) + 31) / 32
	for n := count - 1; n >= 0; n-- {
		m, b, err := c.block(dsmTestBlock(data, n, 3))
		if err != nil {
			t.Fatal(err)
		}
		if n > 0 && m != nil {
			t.Fatal("premature publication")
		}
		if n == 0 && (m == nil || !bytes.Equal(b, data)) {
			t.Fatal("module assembly mismatch")
		}
		duplicate, _, err := c.block(dsmTestBlock(data, n, 3))
		if err != nil || duplicate != nil {
			t.Fatal("duplicate publication")
		}
	}
	same, err := parseDII(dsmTestDII(data, nil, 3), c)
	if err != nil || same.modules[0] != c.modules[0] {
		t.Fatal("repeat DII lost state")
	}
	newer, err := parseDII(dsmTestDII(data, nil, 4), c)
	if err != nil || newer.modules[0].done {
		t.Fatal("version change retained stale module")
	}
	m, _, err := newer.block(dsmTestBlock(data, 0, 3))
	if err != nil || m != nil || newer.modules[0].received != 0 {
		t.Fatal("old DDB accepted")
	}
	if _, _, err := newer.block([]byte{0, 0, 4, 0, 255, 255}); err == nil {
		t.Fatal("out of range accepted")
	}
}
func TestDSMCompressedMultipartAndHTML(t *testing.T) {
	data := dsmTestEntity()
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	zw.Write(data)
	zw.Close()
	desc := []byte{0xc2, 5, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(desc[3:], uint32(len(data)))
	c, err := parseDII(dsmTestDII(buf.Bytes(), desc, 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	var restored []byte
	for n := 0; n < (buf.Len()+31)/32; n++ {
		_, b, err := c.block(dsmTestBlock(buf.Bytes(), n, 1))
		if err != nil {
			t.Fatal(err)
		}
		if b != nil {
			restored = b
		}
	}
	if !bytes.Equal(data, restored) {
		t.Fatal("zlib decode mismatch")
	}
	files, err := dsmFiles(restored, "")
	if err != nil || len(files) != 1 {
		t.Fatalf("multipart: %v", err)
	}
	if !strings.Contains(files[0].HTML, "DSM-CC BML TEST") || strings.Contains(files[0].HTML, "<script") || strings.Contains(files[0].HTML, "onclick=") {
		t.Fatal("HTML conversion/sanitizing failed")
	}
	raw, _ := base64.StdEncoding.DecodeString(files[0].Data)
	if !bytes.Contains(raw, []byte("function change")) {
		t.Fatal("interactive BML was stripped")
	}
}
func TestDSMLogoUsesCDTStorage(t *testing.T) {
	var pngData bytes.Buffer
	png.Encode(&pngData, image.NewNRGBA(image.Rect(0, 0, 2, 1)))
	b := []byte{5, 0, 1, 0, 7, 2, 0, 4, 0, 1, 0, 101, 0, 6, 0, 2, 0, 201, 0, 0}
	binary.BigEndian.PutUint16(b[len(b)-2:], uint16(pngData.Len()))
	b = append(b, pngData.Bytes()...)
	logos, err := parseDSMLogos(b, uint(TuningSpaceBS))
	if err != nil || len(logos) != 2 {
		t.Fatalf("logos %v", err)
	}
	dir := t.TempDir()
	for _, logo := range logos {
		if err := saveCDT(dir, logo); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, fmtDSMLogoName(logo.Sid))
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
	}
	if logos[1].Sid>>16 != uint32(TuningSpaceCS) {
		t.Fatal("cross-network service routing")
	}
	for n := 0; n < len(b); n++ {
		if _, err := parseDSMLogos(b[:n], 1); err == nil {
			t.Fatalf("truncated logo accepted %d", n)
		}
	}
}
func fmtDSMLogoName(sid uint32) string { b, _ := json.Marshal(sid); return string(b) + "_5" }
func TestDSMCharsetAndInertHTML(t *testing.T) {
	b := append([]byte(`<?xml version="1.0" encoding="Shift_JIS"?><bml><body><p>`), 0x93, 0xfa, 0x96, 0x7b)
	b = append(b, []byte(`</p><script>parent.alert(1)</script><img src="https://example.invalid/" onerror="alert(1)"/></body></bml>`)...)
	mimeHTML, err := convertBML(b, "Shift_JIS")
	if err != nil || !strings.Contains(mimeHTML, "日本") {
		t.Fatalf("MIME + XML charset: %v, %s", err, mimeHTML)
	}
	out, err := convertBML(b, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "日本") || strings.Contains(out, "<script") || strings.Contains(out, "onerror=") || strings.Contains(out, ` src="https:`) {
		t.Fatal(out)
	}
}
func TestDSMEventsAndMalformedInput(t *testing.T) {
	s := []byte{0x3d, 0, 0, 0x10, 0, 0xc1, 0, 0, 0x40, 12, 0, 0x10, 0, 0, 0, 0, 0, 0, 1, 0, 2, 0x41, 0, 0, 0, 0}
	ev, err := parseDSMEvents(s, 0x40)
	if err != nil {
		t.Fatal(err)
	}
	if len(ev["events"].([]any)) != 1 {
		t.Fatal(ev)
	}
	if _, err := parseDSMEvents(s[:15], 64); err == nil {
		t.Fatal("truncated event accepted")
	}
	data := dsmTestDII(dsmTestEntity(), nil, 1)
	for n := 0; n < len(data); n++ {
		if _, err := parseDII(data[:n], nil); err == nil {
			t.Fatalf("truncated DII accepted %d", n)
		}
	}
	r := newDSMReceiver()
	dev := &RockDevice{}
	for n := 0; n < 128; n++ {
		p := PacketDSM{Sid: 101, Pid: 256, Components: []DSMComponent{{Pid: 256}}, Data: hex.EncodeToString(make([]byte, n))}
		if err := r.receive(dev, 1, p); err == nil {
			t.Fatal("invalid section accepted")
		}
	}
}
func TestDSMSnapshotFixture(t *testing.T) {
	data := dsmTestEntity()
	c, err := parseDII(dsmTestDII(data, nil, 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	c.modules[0].files, err = dsmFiles(data, "")
	if err != nil {
		t.Fatal(err)
	}
	r := newDSMReceiver()
	r.streams[dsmKey{101, 1, 256}] = c
	p := PacketDSM{Sid: 101, Tsid: 1, Onid: 4, Pid: 256, Components: []DSMComponent{{Pid: 256, ComponentTag: 64, DataComponentId: 7, Info: "3340f880"}}}
	messages := r.snapshot(p)
	messages = append([]any{map[string]any{"type": "programInfo", "originalNetworkId": 4, "transportStreamId": 1, "serviceId": 101, "networkId": 4, "eventId": 1, "eventName": "DSM-CC TEST", "startTimeUnixMillis": 0, "durationSeconds": 3600, "indefiniteDuration": false}}, messages...)
	if len(messages) != 4 {
		t.Fatal(messages)
	}
	if path := os.Getenv("DSM_FIXTURE"); path != "" {
		b, err := json.Marshal(map[string]any{"DeviceID": "test", "Sid": 131173, "Session": "test", "Messages": messages})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
}
