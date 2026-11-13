package tvrock

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"
)

const deviceEPGMaxBytes = 4 * 1024 * 1024

// CPFは従来どおり即時処理する。直近30分のEITはworkerを即座に起こし、
// 先のEPGは接続別に集約する。DB待ちをWebSocket受信へ持ち込まない。
// 同じ番組の再通知は最新値にまとめ、上限超過時は未登録の先の番組を捨てる。
type deviceEPGQueue struct {
	sync.Mutex
	id                                                  string
	programs                                            map[[2]uint]TvProgram
	urgent                                              map[[2]uint]bool
	wake                                                chan struct{}
	bytes                                               int
	messages, receivedBytes, events, coalesced, dropped int64
}

func newDeviceEPGQueue(id string) *deviceEPGQueue {
	return &deviceEPGQueue{id: id, programs: map[[2]uint]TvProgram{}, urgent: map[[2]uint]bool{}, wake: make(chan struct{}, 1)}
}

func epgProgramBytes(p TvProgram) int {
	return 128 + len(p.Title) + len(p.Subtitle) + len(p.Content)
}

func (q *deviceEPGQueue) receive(dev *RockDevice, programs []TvProgram, bytes int) {
	near := false
	cutoff := Now().Add(30 * time.Minute).Unix()
	q.Lock()
	q.messages++
	q.receivedBytes += int64(bytes)
	q.events += int64(len(programs))
	for _, p := range programs {
		// 受信時の放送種別を保存し、後の選局変更から独立させる。
		p.Sid = dev.CurrentService&0xffff0000 | p.Sid&0xffff
		key := [2]uint{p.Sid, p.Eid}
		if (dev.CurrentProgram != nil && dev.CurrentProgram.Sid == p.Sid && dev.CurrentProgram.Eid == p.Eid) ||
			(dev.NextProgram != nil && dev.NextProgram.Sid == p.Sid && dev.NextProgram.Eid == p.Eid) {
			if previous, ok := q.programs[key]; ok {
				q.bytes -= epgProgramBytes(previous)
				delete(q.programs, key)
				delete(q.urgent, key)
			}
			continue
		}
		previous, exists := q.programs[key]
		oldBytes := 0
		if exists {
			oldBytes = epgProgramBytes(previous)
			if p.Content == "" {
				p.Content = previous.Content
			}
			if p.CnibA == 15 && p.CnibB == 15 {
				p.CnibA, p.CnibB = previous.CnibA, previous.CnibB
			}
		}
		size := epgProgramBytes(p)
		if p.Start <= cutoff {
			// 直近情報のために必要なら先の番組を退避対象から外す。
			for other, program := range q.programs {
				if (exists || len(q.programs) < 2048) && q.bytes-oldBytes+size <= deviceEPGMaxBytes {
					break
				}
				if other != key && !q.urgent[other] {
					q.bytes -= epgProgramBytes(program)
					delete(q.programs, other)
					delete(q.urgent, other)
					q.dropped++
				}
			}
		}
		if (!exists && len(q.programs) >= 2048) || q.bytes-oldBytes+size > deviceEPGMaxBytes {
			q.dropped++
			continue
		}
		if exists {
			q.coalesced++
		}
		q.programs[key] = p
		q.urgent[key] = p.Start <= cutoff
		near = near || q.urgent[key]
		q.bytes += size - oldBytes
	}
	q.Unlock()
	if near {
		select {
		case q.wake <- struct{}{}:
		default:
		}
	}
}

func (q *deviceEPGQueue) flush(urgentOnly bool, apply func(uint, []TvProgram)) {
	q.Lock()
	programs := make([]TvProgram, 0, len(q.programs))
	for key, program := range q.programs {
		if urgentOnly && !q.urgent[key] {
			continue
		}
		programs = append(programs, program)
		q.bytes -= epgProgramBytes(program)
		delete(q.programs, key)
		delete(q.urgent, key)
	}
	q.Unlock()
	grouped := map[uint][]TvProgram{}
	for _, p := range programs {
		band := p.Sid & 0xffff0000
		grouped[band] = append(grouped[band], p)
	}
	for band, list := range grouped {
		sort.Slice(list, func(i, j int) bool { return list[i].Start < list[j].Start })
		apply(band, list)
	}
}

func (q *deviceEPGQueue) run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	defer func() {
		q.Lock()
		defer q.Unlock()
		log.Printf("info: EPG受信統計 device:%s messages:%d bytes:%d events:%d coalesced:%d dropped:%d pending:%d", q.id, q.messages, q.receivedBytes, q.events, q.coalesced, q.dropped, len(q.programs))
	}()
	for {
		urgentOnly := false
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-q.wake:
			urgentOnly = true
		}
		started := time.Now()
		q.flush(urgentOnly, func(band uint, list []TvProgram) {
			dev := RockDevice{ID: q.id, CurrentService: band}
			dev.UpdateTVProgram(list)
		})
		if elapsed := time.Since(started); elapsed >= time.Second {
			log.Printf("warn: EPG保存処理の遅延 device:%s elapsed_ms:%d", q.id, elapsed.Milliseconds())
		}
	}
}
