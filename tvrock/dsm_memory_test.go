package tvrock

import (
	"encoding/binary"
	"encoding/hex"
	"strings"
	"testing"
)

func dsmMemoryPacket(sid uint16, size int) PacketDSM {
	body := dsmTestDII([]byte{1}, nil, 1)
	binary.BigEndian.PutUint32(body[22:], uint32(size))
	return PacketDSM{Sid: sid, Tsid: 1, Pid: 256,
		Components: []DSMComponent{{Pid: 256, ComponentTag: 64, DataComponentId: 12}},
		Data:       hex.EncodeToString(dsmWire(0x3b, 0x1002, body, 0, 0))}
}

func TestDSMUnselectedServicesDoNotReservePayloadMemory(t *testing.T) {
	r := newDSMReceiver()
	r.notify = func(string, string) {}
	dev := &RockDevice{CurrentService: 1<<16 | 101}
	for sid := uint16(101); sid < 121; sid++ {
		p := dsmMemoryPacket(sid, dsmModuleLimit)
		if err := r.receive(dev, 1, p); err != nil {
			t.Fatal(err)
		}
		m := r.streams[dsmKey{sid, 1, 256}].modules[0]
		if m.ignored != (sid != 101) {
			t.Fatalf("sid=%d ignored=%v", sid, m.ignored)
		}
		p.Data = hex.EncodeToString(dsmWire(0x3c, 0x1003, dsmTestBlock(make([]byte, 32), 0, 1), 0, 0))
		if err := r.receive(dev, 1, p); err != nil {
			t.Fatal(err)
		}
		if sid != 101 && (m.received != 0 || len(m.blocks) != 0) {
			t.Fatal("unselected payload retained")
		}
	}
	// Switching services on the same TS must reprocess identical DII data.
	dev.CurrentService = 1<<16 | 102
	if err := r.receive(dev, 1, dsmMemoryPacket(102, dsmModuleLimit)); err != nil {
		t.Fatal(err)
	}
	if len(r.streams) != 1 || r.streams[dsmKey{102, 1, 256}].modules[0].ignored {
		t.Fatal("service switch retained stale selection")
	}
}

func TestDSMMemoryRejectionRetriesIdenticalDII(t *testing.T) {
	r := newDSMReceiver()
	r.notify = func(string, string) {}
	dev := &RockDevice{CurrentService: 1<<16 | 101}
	r.service = dev.CurrentService
	occupied := dsmKey{101, 1, 257}
	r.streams[occupied] = &dsmCarousel{modules: map[uint16]*dsmModule{0: {size: dsmMemoryLimit}}}
	p := dsmMemoryPacket(101, 32)
	p.Components = append(p.Components, DSMComponent{Pid: 257})
	key := dsmKey{101, 1, 256}
	if err := r.receive(dev, 1, p); err == nil || !strings.Contains(err.Error(), "requested=") {
		t.Fatalf("expected budget diagnostic, got %v", err)
	}
	if r.streams[key] != nil || r.diiSections[key] != nil {
		t.Fatal("rejected DII was committed")
	}
	delete(r.streams, occupied)
	if err := r.receive(dev, 1, p); err != nil {
		t.Fatal(err)
	}
	if r.streams[key] == nil {
		t.Fatal("identical DII not retried after memory became available")
	}
}

func TestDSMIncompleteDIIClearedOnTransportChange(t *testing.T) {
	r := newDSMReceiver()
	r.notify = func(string, string) {}
	p := dsmMemoryPacket(101, 32)
	p.Data = hex.EncodeToString(dsmWire(0x3b, 0x1002, dsmTestDII([]byte{1}, nil, 1), 0, 1))
	if err := r.receive(&RockDevice{}, 1, p); err != nil {
		t.Fatal(err)
	}
	p = dsmMemoryPacket(101, 32)
	p.Tsid = 2
	if err := r.receive(&RockDevice{}, 1, p); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.diiSections[dsmKey{101, 1, 256}]; ok {
		t.Fatal("incomplete old DII retained")
	}
}
