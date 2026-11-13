package tvrock

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSocketWriteTimesOutWhenClientStopsReading(t *testing.T) {
	finished := make(chan error, 1)
	serverSide, clientSide := net.Pipe()
	listener := &socketTestListener{Conn: serverSide, closed: make(chan struct{})}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			finished <- err
			return
		}
		defer c.Close()
		finished <- writeSocketMessage(c, []byte(strings.Repeat("x", 1<<20)))
	}))
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	address, _ := url.Parse("ws://127.0.0.1/dev")
	c, _, err := websocket.NewClient(clientSide, address, nil, 1024, 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	select {
	case err := <-finished:
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatalf("expected write timeout, got %v", err)
		}
	case <-time.After(8 * time.Second):
		t.Fatal("stalled client held writer beyond deadline")
	}
}

// net.PipeにはOSの受信バッファがなく、読み取らない相手を確実に再現できる。
type socketTestListener struct {
	net.Conn
	accepted bool
	closed   chan struct{}
	once     sync.Once
}

func (l *socketTestListener) Accept() (net.Conn, error) {
	if !l.accepted {
		l.accepted = true
		return l.Conn, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}
func (l *socketTestListener) Close() error {
	l.once.Do(func() { close(l.closed); _ = l.Conn.Close() })
	return nil
}
func (l *socketTestListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1}
}
