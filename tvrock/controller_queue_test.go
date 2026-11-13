package tvrock

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestControllerCaptionFloodDoesNotDelayControlOrOtherClients(t *testing.T) {
	slow := &DevConQueue{Com: make(chan string, 1), Captions: make(chan string, 2), Done: make(chan struct{})}
	fast := &DevConQueue{Com: make(chan string, 2), Captions: make(chan string, 2), Done: make(chan struct{})}
	controller := DeviceController{Handler: map[string]*DevConQueue{"slow": slow, "fast": fast}}
	for i := 0; i < 10000; i++ {
		slow.send("cap:subtitle")
	}
	if slow.dropped.Load() != 9998 || slow.overloaded() {
		t.Fatal(slow.stats())
	}
	slow.send("res:first") // 通常通知も詰まったクライアントを用意する。
	finished := make(chan struct{})
	go func() { controller.Notify(ControllerUpdateSchedule, ""); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("slow client blocked a publisher")
	}
	if !slow.overloaded() {
		t.Fatal("important notification silently lost")
	}
	if got := <-fast.Com; got != ControllerUpdateSchedule {
		t.Fatal(got)
	}
	if _, ok := slow.receive(context.Background()); ok {
		t.Fatal("overflowed connection remained usable")
	}
}

func TestControllerPrefersControlAndCoalescesStateWithoutLosingResponses(t *testing.T) {
	q := &DevConQueue{Com: make(chan string, 32), Captions: make(chan string, 2), Done: make(chan struct{})}
	q.send(`ply:{"action":"caption"}`)
	input := []string{"dev:rec1", "dev:rec1", "usc:", "usc:", "sti:old", "sti:new", "res:first", "usc:", "res:second", `ply:{"action":"close"}`}
	for _, message := range input {
		q.send(message)
	}
	got, ok := q.receive(context.Background())
	want := []string{"dev:rec1", "usc:", "sti:new", "res:first", "usc:", "res:second", `ply:{"action":"close"}`}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if q.coalesced.Load() != 3 {
		t.Fatal(q.stats())
	}
	got, ok = q.receive(context.Background())
	if !ok || len(got) != 1 || !controllerCaption(got[0]) {
		t.Fatal(got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok := q.receive(ctx); ok {
		t.Fatal("cancelled receiver accepted data")
	}
}

func TestControllerConcurrentOverflowNeverBlocks(t *testing.T) {
	q := &DevConQueue{Com: make(chan string, 1), Done: make(chan struct{})}
	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			for range 100 {
				q.send("res:result")
			}
		})
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("concurrent publishers blocked")
	}
	if !q.overloaded() {
		t.Fatal("missing overflow signal")
	}
}
