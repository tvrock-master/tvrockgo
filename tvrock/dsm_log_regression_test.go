package tvrock

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDSMEngineeringModulesDoNotEnterBMLParser(t *testing.T) {
	for _, id := range []uint16{0, 0xffff} {
		r := newDSMReceiver()
		r.notify = func(string, string) { t.Fatal("engineering data published as BML") }
		p := PacketDSM{Sid: 101, Tsid: 1, Pid: 256, Components: []DSMComponent{{Pid: 256, ComponentTag: 255, DataComponentId: id}}}
		dev := &RockDevice{CurrentService: uint(TuningSpaceBS)<<16 | 101}
		// Binary module prefix taken from the error log, not a MIME entity.
		data := []byte{0x4c, 2, 0x3d, 0x18, 0xf0, 2, 0x7f, 0x30, 0x7f, 0x30, 0x18, 0x7f, 0x30, 0xf1, 0, 0, 0, 0x22}
		p.Data = hex.EncodeToString(dsmWire(0x3b, 0x1002, dsmTestDII(data, nil, 1), 0, 0))
		if err := r.receive(dev, uint(TuningSpaceBS), p); err != nil {
			t.Fatal(err)
		}
		p.Data = hex.EncodeToString(dsmWire(0x3c, 0x1003, dsmTestBlock(data, 0, 1), 0, 0))
		if err := r.receive(dev, uint(TuningSpaceBS), p); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDSMLogoAcknowledgesOnlyAfterSavingAndRetriesFailures(t *testing.T) {
	r := newDSMReceiver()
	r.notify = func(string, string) {}
	failSave := true
	r.saveLogo = func(PacketCDT) error {
		if failSave {
			return errors.New("test disk error")
		}
		return nil
	}
	var receipts []dsmLogoReceipt
	r.ackLogo = func(_ *RockDevice, receipt dsmLogoReceipt) bool { receipts = append(receipts, receipt); return true }
	data := []byte{5, 0, 1, 0, 7, 1, 0, 4, 0, 1, 0, 101, 0, 3, 1, 2, 3}
	name := []byte("LOGO-05")
	desc := append([]byte{2, byte(len(name))}, name...)
	p := PacketDSM{Sid: 101, Onid: 4, Tsid: 1, Pid: 256, Components: []DSMComponent{{Pid: 256, ComponentTag: 255}}}
	dev := &RockDevice{}
	for _, version := range []byte{1, 2} {
		p.Data = hex.EncodeToString(dsmWire(0x3b, 0x1002, dsmTestDII(data, desc, version), 0, 0))
		if err := r.receive(dev, uint(TuningSpaceBS), p); err != nil {
			t.Fatal(err)
		}
		p.Data = hex.EncodeToString(dsmWire(0x3c, 0x1003, dsmTestBlock(data, 0, version), 0, 0))
		if version == 1 {
			if err := r.receive(dev, uint(TuningSpaceBS), p); err == nil {
				t.Fatal("save failure ignored")
			}
			if len(receipts) != 0 {
				t.Fatal("unsaved logo acknowledged")
			}
			failSave = false
		}
		if err := r.receive(dev, uint(TuningSpaceBS), p); err != nil {
			t.Fatal(err)
		}
		if len(receipts) != int(version) || receipts[len(receipts)-1].Version != version || receipts[len(receipts)-1].ModuleID != 0 {
			t.Fatalf("%+v", receipts)
		}
	}
	// 受領通知のキューが満杯でもブロックしない。
	dev.State = StateReady
	dev.Com = make(chan RequestMessage, 1)
	dev.Com <- RequestMessage{Command: "RecordStop"}
	if sendDSMLogoReceipt(dev, receipts[0]) {
		t.Fatal("receipt replaced recording command")
	}
	if (<-dev.Com).Command != "RecordStop" {
		t.Fatal("recording command lost")
	}
}

func TestDSMEngineeringLogoStillSaved(t *testing.T) {
	r := newDSMReceiver()
	r.notify = func(string, string) {}
	count := 0
	r.saveLogo = func(p PacketCDT) error {
		count++
		if p.LogoId != 7 || p.Data != "010203" {
			t.Fatal(p)
		}
		return nil
	}
	data := []byte{5, 0, 1, 0, 7, 1, 0, 4, 0, 1, 0, 101, 0, 3, 1, 2, 3}
	name := []byte("LOGO-05")
	desc := append([]byte{2, byte(len(name))}, name...)
	p := PacketDSM{Sid: 101, Tsid: 1, Pid: 256, Components: []DSMComponent{{Pid: 256, ComponentTag: 255}}}
	p.Data = hex.EncodeToString(dsmWire(0x3b, 0x1002, dsmTestDII(data, desc, 1), 0, 0))
	if err := r.receive(&RockDevice{}, uint(TuningSpaceBS), p); err != nil {
		t.Fatal(err)
	}
	p.Data = hex.EncodeToString(dsmWire(0x3c, 0x1003, dsmTestBlock(data, 0, 1), 0, 0))
	if err := r.receive(&RockDevice{}, uint(TuningSpaceBS), p); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("logos saved %d", count)
	}
}

func TestDSMEmptyLocationAndMissingPart(t *testing.T) {
	// An empty relative URI is distinct from an absent header.
	data := bytes.Replace(dsmTestEntity(), []byte("Content-Location: startup.bml"), []byte("Content-Location:"), 1)
	f, err := dsmFiles(data, "")
	if err != nil || len(f) != 1 || f[0].Location == nil || *f[0].Location != "" {
		t.Fatalf("empty location: %+v %v", f, err)
	}
	// An unaddressable part must not make a following startup.bml disappear.
	prefix := "--tvrock\r\nContent-Type: application/octet-stream\r\n\r\nunused\r\n"
	data = bytes.Replace(dsmTestEntity(), []byte("--tvrock\r\n"), []byte(prefix+"--tvrock\r\n"), 1)
	f, err = dsmFiles(data, "")
	if err != nil || len(f) != 1 || *f[0].Location != "startup.bml" || f[0].HTML == "" {
		t.Fatalf("missing part: %+v %v", f, err)
	}
	data = bytes.Replace(dsmTestEntity(), []byte("Content-Location: startup.bml\r\n"), nil, 1)
	if _, err = dsmFiles(data, ""); err == nil || !strings.Contains(err.Error(), "no addressable") {
		t.Fatal("unusable module must remain diagnosable", err)
	}
}

func TestDSMMalformedMIMEErrorIsBounded(t *testing.T) {
	data := append([]byte("malformed\x00"), bytes.Repeat([]byte{1}, 10000)...)
	_, err := dsmFiles(data, "")
	if err == nil || len(err.Error()) > 100 || strings.ContainsRune(err.Error(), 0) {
		t.Fatalf("unsafe diagnostic %q", err)
	}
}

func TestSDTUnknownOneSegIsAccepted(t *testing.T) {
	var p []PacketSDT
	if err := json.Unmarshal([]byte(`[{"Sid":101,"OneSeg":-1},{"Sid":102,"OneSeg":0},{"Sid":103,"OneSeg":1}]`), &p); err != nil {
		t.Fatal(err)
	}
	if p[0].OneSeg != -1 || p[1].OneSeg != 0 || p[2].OneSeg != 1 {
		t.Fatal(p)
	}
}
