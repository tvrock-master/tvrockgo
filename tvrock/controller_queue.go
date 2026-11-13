package tvrock

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
)

// UI通知の遅延を、録画アプリの受信処理や他のクライアントへ伝播させない。
// 字幕は別の小さいキューで扱い、通常通知を優先する。
type DevConQueue struct {
	Com       chan string   `json:"-"`
	Captions  chan string   `json:"-"`
	Done      chan struct{} `json:"-"`
	once      sync.Once
	failOnce  sync.Once
	overflow  chan struct{}
	dropped   atomic.Uint64
	coalesced atomic.Uint64
	sent      atomic.Uint64
	bytes     atomic.Uint64
}

type controllerQueueStats struct {
	Pending         int    `json:"pending"`
	PendingCaptions int    `json:"pendingCaptions"`
	DroppedCaptions uint64 `json:"droppedCaptions"`
	Coalesced       uint64 `json:"coalesced"`
	Sent            uint64 `json:"sent"`
	PayloadBytes    uint64 `json:"payloadBytes"`
	Overloaded      bool   `json:"overloaded"`
}

func (p *DevConQueue) failure() <-chan struct{} {
	p.once.Do(func() { p.overflow = make(chan struct{}) })
	return p.overflow
}

func (p *DevConQueue) overloaded() bool {
	select {
	case <-p.failure():
		return true
	default:
		return false
	}
}

func controllerCaption(message string) bool {
	if strings.HasPrefix(message, ControllerSendCaption) {
		return true
	}
	if strings.HasPrefix(message, "ply:") {
		var value struct {
			Action string `json:"action"`
		}
		return json.Unmarshal([]byte(message[4:]), &value) == nil && value.Action == "caption"
	}
	return false
}

func (p *DevConQueue) send(message string) {
	failure := p.failure()
	select {
	case <-p.Done:
		return
	case <-failure:
		return
	default:
	}
	queue := p.Com
	caption := controllerCaption(message)
	if caption && p.Captions != nil {
		queue = p.Captions
	}
	select {
	case queue <- message:
	default:
		if caption {
			p.dropped.Add(1)
		} else {
			// 制御・検索結果を黙って欠落させず、このクライアントを切断する。
			p.failOnce.Do(func() { close(p.overflow) })
		}
	}
}

// 同じバッチの状態通知をまとめる。検索結果や再生の操作応答は順序を保つ。
func coalesceControllerMessages(messages []string) []string {
	result := make([]string, 0, len(messages))
	positions := map[string]int{}
	for _, message := range messages {
		key := ""
		if len(message) >= 4 {
			switch message[:4] {
			case ControllerSendDevice:
				key = message // 最新のデバイス情報は送信時に取得する。
			case ControllerUpdateTimeTable, ControllerUpdateStation, ControllerUpdateSchedule,
				ControllerUpdateRecorderSetting, ControllerUpdatePowerManagerSetting, ControllerUpdateKeywordReserve,
				ControllerSendStorageInfo, "pbs:":
				key = message[:4]
			}
		}
		if key == "" {
			// 操作応答を跨いで状態を入れ替えない。
			clear(positions)
		} else if index, ok := positions[key]; ok {
			result[index] = message
			continue
		} else {
			positions[key] = len(result)
		}
		result = append(result, message)
	}
	return result
}

func (p *DevConQueue) receive(ctx context.Context) ([]string, bool) {
	failure := p.failure()
	select {
	case <-ctx.Done():
		return nil, false
	case <-p.Done:
		return nil, false
	case <-failure:
		return nil, false
	default:
	}

	var message string
	// 字幕が溜まっていても、通常通知を先に取り出す。
	select {
	case message = <-p.Com:
	default:
		select {
		case <-ctx.Done():
			return nil, false
		case <-p.Done:
			return nil, false
		case <-failure:
			return nil, false
		case message = <-p.Com:
		case message = <-p.Captions:
		}
	}

	messages := []string{message}
	for len(messages) < 256 {
		select {
		case message = <-p.Com:
			messages = append(messages, message)
		default:
			return p.coalesce(messages), true
		}
	}
	return p.coalesce(messages), true
}

func (p *DevConQueue) coalesce(messages []string) []string {
	result := coalesceControllerMessages(messages)
	p.coalesced.Add(uint64(len(messages) - len(result)))
	return result
}

func (p *DevConQueue) stats() controllerQueueStats {
	return controllerQueueStats{len(p.Com), len(p.Captions), p.dropped.Load(), p.coalesced.Load(), p.sent.Load(), p.bytes.Load(), p.overloaded()}
}
