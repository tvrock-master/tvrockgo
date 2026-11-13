package tvrock

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Keep section length and CRC valid while changing only the DSM message length.
func dsmDownloadLengthWire(tid byte, id uint16, body []byte, length uint16) []byte {
	s := dsmWire(tid, id, body, 0, 0)
	binary.BigEndian.PutUint16(s[18:], length)
	binary.BigEndian.PutUint32(s[len(s)-4:], dsmCRC(s[:len(s)-4]))
	return s
}

func TestDSMReceiverDownloadComponentsAreIgnored(t *testing.T) {
	for _, message := range []struct {
		name string
		tid  byte
		id   uint16
		body []byte
	}{
		{"DII", 0x3b, 0x1002, dsmTestDII([]byte("receiver firmware"), nil, 1)},
		{"DDB", 0x3c, 0x1003, dsmTestBlock([]byte("receiver firmware"), 0, 1)},
	} {
		for _, length := range []uint16{0, uint16(len(message.body)), 0xffff} {
			t.Run(fmt.Sprintf("%s/length=%d", message.name, length), func(t *testing.T) {
				r := newDSMReceiver()
				r.notify = func(string, string) { t.Fatal("receiver download published") }
				r.saveLogo = func(PacketCDT) error { t.Fatal("receiver download saved as logo"); return nil }
				p := PacketDSM{Sid: 929, Tsid: 16625}
				for pid := uint16(1905); pid <= 1914; pid++ {
					p.Components = append(p.Components, DSMComponent{Pid: pid, ComponentTag: uint8(pid - 1792), DataComponentId: 9})
				}
				dev := &RockDevice{CurrentService: uint(TuningSpaceBS)<<16 | 929}
				for _, pid := range []uint16{0x771, 0x778} {
					p.Pid = pid
					p.Data = hex.EncodeToString(dsmDownloadLengthWire(message.tid, message.id, message.body, length))
					for repeat := 0; repeat < 16; repeat++ {
						if err := r.receive(dev, uint(TuningSpaceBS), p); err != nil {
							t.Fatal(err)
						}
					}
				}
				if len(r.streams) != 0 || len(r.diiSections) != 0 {
					t.Fatal("receiver download retained carousel state")
				}
			})
		}
	}
}

func TestDSMReceiverDownloadCommonLogos(t *testing.T) {
	for _, tag := range []uint8{0x79, 0x7a} {
		for _, name := range []string{"LOGO-05", "CS_LOGO-05"} {
			t.Run(fmt.Sprintf("tag=%02x/%s", tag, name), func(t *testing.T) {
				var imageData bytes.Buffer
				if err := png.Encode(&imageData, image.NewNRGBA(image.Rect(0, 0, 2, 1))); err != nil {
					t.Fatal(err)
				}
				// The engineering service carries logos for both BS and CS.
				data := []byte{5, 0, 1, 0, 7, 2, 0, 4, 0, 1, 0, 101, 0, 6, 0, 2, 0, 201, 0, 0}
				binary.BigEndian.PutUint16(data[len(data)-2:], uint16(imageData.Len()))
				data = append(data, imageData.Bytes()...)
				desc := append([]byte{2, byte(len(name))}, []byte(name)...)
				r := newDSMReceiver()
				r.notify = func(string, string) { t.Fatal("receiver download published as BML") }
				dir := t.TempDir()
				count := 0
				r.saveLogo = func(p PacketCDT) error { count++; return saveCDT(dir, p) }
				p := PacketDSM{Sid: 929, Tsid: 16625, Pid: 0x779, Components: []DSMComponent{{Pid: 0x779, ComponentTag: tag, DataComponentId: 9}}}
				// It must work even while a different service is selected.
				dev := &RockDevice{CurrentService: uint(TuningSpaceBS)<<16 | 101}
				p.Data = hex.EncodeToString(dsmWire(0x3b, 0x1002, dsmTestDII(data, desc, 1), 0, 0))
				if err := r.receive(dev, uint(TuningSpaceBS), p); err != nil {
					t.Fatal(err)
				}
				for n := (len(data) - 1) / 32; n >= 0; n-- {
					p.Data = hex.EncodeToString(dsmWire(0x3c, 0x1003, dsmTestBlock(data, n, 1), 0, 0))
					for repeat := 0; repeat < 2; repeat++ {
						if err := r.receive(dev, uint(TuningSpaceBS), p); err != nil {
							t.Fatal(err)
						}
					}
				}
				if count != 2 {
					t.Fatalf("saved %d logos, want 2", count)
				}
				for _, sid := range []uint32{uint32(TuningSpaceBS)<<16 | 101, uint32(TuningSpaceCS)<<16 | 201} {
					stored, err := os.ReadFile(filepath.Join(dir, fmtDSMLogoName(sid)))
					if err != nil {
						t.Fatal(err)
					}
					xorCDT(stored)
					if _, err := png.Decode(bytes.NewReader(stored)); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
}

func TestDSMReceiverDownloadCommonNonLogosIgnored(t *testing.T) {
	for _, tag := range []uint8{0x79, 0x7a} {
		for _, name := range []string{"firmware", "startup.bml", "LOGO-06", "LOGO-05-firmware", "CS_LOGO-00-extra", ""} {
			t.Run(fmt.Sprintf("tag=%02x/name=%s", tag, name), func(t *testing.T) {
				r := newDSMReceiver()
				r.notify = func(string, string) { t.Fatal("non-logo published") }
				r.saveLogo = func(PacketCDT) error { t.Fatal("non-logo saved"); return nil }
				p := PacketDSM{Sid: 929, Tsid: 16625, Pid: 0x779, Components: []DSMComponent{{Pid: 0x779, ComponentTag: tag, DataComponentId: 9}}}
				dev := &RockDevice{CurrentService: uint(TuningSpaceBS)<<16 | 929}
				data := []byte("not a logo or MIME entity")
				desc := append([]byte{2, byte(len(name))}, []byte(name)...)
				p.Data = hex.EncodeToString(dsmWire(0x3b, 0x1002, dsmTestDII(data, desc, 1), 0, 0))
				if err := r.receive(dev, uint(TuningSpaceBS), p); err != nil {
					t.Fatal(err)
				}
				p.Data = hex.EncodeToString(dsmWire(0x3c, 0x1003, dsmTestBlock(data, 0, 1), 0, 0))
				if err := r.receive(dev, uint(TuningSpaceBS), p); err != nil {
					t.Fatal(err)
				}
				m := r.streams[dsmKey{p.Sid, p.Tsid, p.Pid}].modules[0]
				if !m.ignored || m.blocks != nil || m.received != 0 || m.done || len(m.files) != 0 {
					t.Fatal("non-logo payload retained")
				}
			})
		}
	}
}

func TestDSMMessageLengthValidationRemainsEnabled(t *testing.T) {
	for _, componentID := range []uint16{0, 7, 11, 12, 13} {
		r := newDSMReceiver()
		p := PacketDSM{Sid: 101, Tsid: 1, Pid: 256, Components: []DSMComponent{{Pid: 256, ComponentTag: 64, DataComponentId: componentID}}}
		p.Data = hex.EncodeToString(dsmDownloadLengthWire(0x3b, 0x1002, dsmTestDII([]byte("data"), nil, 1), 0))
		if err := r.receive(&RockDevice{}, 1, p); err == nil || !strings.Contains(err.Error(), "invalid DSM message length") {
			t.Fatalf("component=%d: expected message length error, got %v", componentID, err)
		}
		if len(r.streams) != 0 || len(r.diiSections) != 0 {
			t.Fatal("invalid message retained carousel state")
		}
	}
}

func TestDSMReceiverDownloadCommonValidation(t *testing.T) {
	for _, tag := range []uint8{0x79, 0x7a} {
		r := newDSMReceiver()
		r.notify = func(string, string) { t.Fatal("receiver event published") }
		p := PacketDSM{Sid: 929, Tsid: 16625, Pid: 0x779, Components: []DSMComponent{{Pid: 0x779, ComponentTag: tag, DataComponentId: 9}}}
		s := dsmWire(0x3b, 0x1002, dsmTestDII([]byte("logo"), nil, 1), 0, 0)
		s[len(s)-1] ^= 1
		p.Data = hex.EncodeToString(s)
		if err := r.receive(&RockDevice{}, uint(TuningSpaceBS), p); err == nil || err.Error() != "invalid DSM section/CRC" {
			t.Fatalf("tag=%02x: invalid CRC accepted: %v", tag, err)
		}
		p.Data = hex.EncodeToString(dsmDownloadLengthWire(0x3b, 0x1002, dsmTestDII([]byte("logo"), nil, 1), 0))
		if err := r.receive(&RockDevice{}, uint(TuningSpaceBS), p); err == nil || !strings.Contains(err.Error(), "invalid DSM message length") {
			t.Fatalf("tag=%02x: invalid length accepted: %v", tag, err)
		}
		// Valid section/CRC, but no event body: it must be ignored before event parsing.
		p.Data = hex.EncodeToString(dsmWire(0x3d, 0, nil, 0, 0))
		if err := r.receive(&RockDevice{}, uint(TuningSpaceBS), p); err != nil {
			t.Fatal(err)
		}
		if len(r.streams) != 0 || len(r.diiSections) != 0 {
			t.Fatal("invalid download retained state")
		}
	}
}

func TestDSMReceiverDownloadSectionValidation(t *testing.T) {
	r := newDSMReceiver()
	p := PacketDSM{Sid: 929, Tsid: 16625, Pid: 0x779, Components: []DSMComponent{{Pid: 0x779, DataComponentId: 9}}}
	s := dsmWire(0x3c, 0x1003, dsmTestBlock([]byte("firmware"), 0, 1), 0, 0)
	s[len(s)-1] ^= 1
	p.Data = hex.EncodeToString(s)
	if err := r.receive(&RockDevice{}, 1, p); err == nil || err.Error() != "invalid DSM section/CRC" {
		t.Fatalf("invalid receiver download CRC accepted: %v", err)
	}
	// A download component elsewhere in the PMT must not bypass BML validation.
	p.Pid = 0x780
	p.Components = append(p.Components, DSMComponent{Pid: p.Pid, ComponentTag: 64, DataComponentId: 7})
	p.Data = hex.EncodeToString(dsmDownloadLengthWire(0x3b, 0x1002, dsmTestDII([]byte("BML"), nil, 1), 0))
	if err := r.receive(&RockDevice{}, 1, p); err == nil || !strings.Contains(err.Error(), "invalid DSM message length") {
		t.Fatalf("BML in mixed PMT bypassed message length validation: %v", err)
	}
}
