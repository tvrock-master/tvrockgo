package tvrock

import (
	"context"
	"sync"
)

// Admission（受け入れ）とshutdown（停止）はロックを共有しているため、Waitと新規のAddが競合することはありません。
// ストアを使用する処理は、それらのストアがクローズされる前に完了する必要があります。
type workerGroup struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	stopping bool
}

func (g *workerGroup) enter() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopping {
		return false
	}
	g.wg.Add(1)
	return true
}

func (g *workerGroup) start(fn func()) bool {
	if !g.enter() {
		return false
	}
	go func() { defer g.wg.Done(); fn() }()
	return true
}

func (g *workerGroup) stop() {
	g.mu.Lock()
	g.stopping = true
	g.mu.Unlock()
}

func (g *workerGroup) wait() { g.wg.Wait() }

var serviceWorkers workerGroup
var shutdownOnce sync.Once
var globalCancel context.CancelFunc

func serviceContext() context.Context {
	if globalContext == nil {
		return context.Background()
	}
	return globalContext
}
