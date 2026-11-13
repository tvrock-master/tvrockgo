package tvrock

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDeviceEPGCoalescesAndKeepsReceivedBandAfterTuning(t *testing.T) {
	q := newDeviceEPGQueue("test")
	dev := &RockDevice{CurrentService: 0x10065}
	now := Now().Unix()
	p := TvProgram{Sid: 101, Eid: 1, Start: now + 3600, End: now + 5400, Content: "details", CnibA: 1, CnibB: 2}
	q.receive(dev, []TvProgram{p}, 100)
	p.Content, p.Title, p.CnibA, p.CnibB = "", "updated", 15, 15
	q.receive(dev, []TvProgram{p}, 100)
	dev.CurrentService = 0x20065
	q.receive(dev, []TvProgram{{Sid: 101, Eid: 2, Start: now + 60, End: now + 120}}, 50)
	select {
	case <-q.wake:
	default:
		t.Fatal("near EPG did not wake worker")
	}
	q.flush(true, func(band uint, list []TvProgram) {
		if band != 0x20000 || len(list) != 1 || list[0].Sid != 0x20065 {
			t.Fatal(band, list)
		}
	})
	if len(q.programs) != 1 {
		t.Fatal("future EPG was sent with urgent updates")
	}
	q.flush(false, func(band uint, list []TvProgram) {
		if band != 0x10000 || len(list) != 1 || list[0].Title != "updated" || list[0].Content != "details" || list[0].CnibA != 1 || list[0].CnibB != 2 {
			t.Fatal(band, list)
		}
	})
	if q.bytes != 0 || q.coalesced != 1 {
		t.Fatal(q.bytes, q.coalesced)
	}
}

func TestDeviceEPGBoundedAndReceiveDoesNotWaitForStorage(t *testing.T) {
	q := newDeviceEPGQueue("test")
	dev := &RockDevice{CurrentService: 0x10065}
	now := Now().Unix()
	q.receive(dev, []TvProgram{{Sid: 101, Eid: 1, Start: now + 3600}}, 100)
	started, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() { q.flush(false, func(uint, []TvProgram) { close(started); <-release }); close(finished) }()
	<-started
	t.Cleanup(func() { close(release); <-finished })
	received := make(chan struct{})
	go func() {
		q.receive(dev, []TvProgram{{Sid: 101, Eid: 2, Start: now + 7200, Content: strings.Repeat("x", deviceEPGMaxBytes+1)}}, 100)
		close(received)
	}()
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("database worker blocked receiver")
	}
	if q.dropped != 1 || q.bytes > deviceEPGMaxBytes {
		t.Fatal(q.dropped, q.bytes)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { q.run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("EPG worker ignored cancellation")
	}
}
