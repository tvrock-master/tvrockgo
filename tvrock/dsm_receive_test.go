package tvrock

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func dsmWire(tid byte, id uint16, body []byte, section, last byte) []byte {
	s := []byte{tid, 0xb0, 0, 0, 0, 0xc1, section, last, 0x11, 3, byte(id >> 8), byte(id), 0x10, 0, 0, 1, 0xff, 0, byte(len(body) >> 8), byte(len(body))}
	s = append(s, body...)
	binary.BigEndian.PutUint16(s[1:], 0xb000|uint16(len(s)+1))
	c := dsmCRC(s)
	return binary.BigEndian.AppendUint32(s, c)
}
func TestDSMReceivePipelineAndLateSnapshot(t *testing.T) {
	r := newDSMReceiver()
	var sent []map[string]any
	r.notify = func(command, body string) {
		if command != "bml:" {
			t.Fatal(command)
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatal(err)
		}
		sent = append(sent, v)
	}
	dev := &RockDevice{ID: "test", CurrentService: uint(TuningSpaceBS)<<16 | 101}
	packet := PacketDSM{Sid: 101, Tsid: 1, Onid: 4, Pid: 256, Components: []DSMComponent{{Pid: 256, ComponentTag: 64, DataComponentId: 7, Info: "3340f880"}}}
	data := dsmTestEntity()
	dii := dsmWire(0x3b, 0x1002, dsmTestDII(data, nil, 1), 0, 0)
	packet.Data = hex.EncodeToString(dii)
	if err := r.receive(dev, uint(TuningSpaceBS), packet); err != nil {
		t.Fatal(err)
	}
	for n := (len(data)+31)/32 - 1; n >= 0; n-- {
		packet.Data = hex.EncodeToString(dsmWire(0x3c, 0x1003, dsmTestBlock(data, n, 1), byte(n), 0))
		r.lastPublish = time.Time{}
		if err := r.receive(dev, uint(TuningSpaceBS), packet); err != nil {
			t.Fatal(err)
		}
	}
	if len(sent) == 0 {
		t.Fatal("nothing published")
	}
	last := sent[len(sent)-1]
	if last["DeviceID"] != "test" || last["Sid"] != float64(dev.CurrentService) {
		t.Fatal(last)
	}
	messages := last["Messages"].([]any)
	found := false
	for _, msg := range messages {
		v := msg.(map[string]any)
		if v["type"] == "moduleDownloaded" {
			found = true
			f := v["files"].([]any)[0].(map[string]any)
			if f["html"] == "" {
				t.Fatal("missing server HTML")
			}
		}
	}
	if !found {
		t.Fatal("complete module not published")
	}
	count := len(sent)
	packet.Data = hex.EncodeToString(dii)
	r.lastPublish = time.Now().Add(-16 * time.Second)
	if err := r.receive(dev, uint(TuningSpaceBS), packet); err != nil {
		t.Fatal(err)
	}
	if len(sent) != count+1 {
		t.Fatal("late browser bootstrap missing")
	}
	packet.Kind = "pmt"
	packet.Components = nil
	if err := r.receive(dev, uint(TuningSpaceBS), packet); err != nil {
		t.Fatal(err)
	}
	if len(r.streams) != 0 || len(r.diiSections) != 0 {
		t.Fatal("removed stream retained")
	}
	last = sent[len(sent)-1]
	components := last["Messages"].([]any)[0].(map[string]any)["components"].([]any)
	if len(components) != 0 {
		t.Fatal("removed data broadcast not cleared")
	}
}
func TestDSMMultiSectionDIICommitsAtomically(t *testing.T) {
	r := newDSMReceiver()
	r.notify = func(string, string) {}
	p := PacketDSM{Sid: 101, Tsid: 1, Pid: 256, Components: []DSMComponent{{Pid: 256}}}
	first := dsmTestDII([]byte("one"), nil, 1)
	second := dsmTestDII([]byte("two"), nil, 1)
	second[21] = 1
	p.Data = hex.EncodeToString(dsmWire(0x3b, 0x1002, second, 1, 1))
	if err := r.receive(&RockDevice{}, 1, p); err != nil {
		t.Fatal(err)
	}
	if len(r.streams) != 0 {
		t.Fatal("incomplete DII published")
	}
	p.Data = hex.EncodeToString(dsmWire(0x3b, 0x1002, first, 0, 1))
	if err := r.receive(&RockDevice{}, 1, p); err != nil {
		t.Fatal(err)
	}
	c := r.streams[dsmKey{101, 1, 256}]
	if c == nil || len(c.modules) != 2 {
		t.Fatal("DII sections not merged")
	}
}
