package tvrock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSearchServiceSelection(t *testing.T) {
	previous := station.Cache
	station.Cache = map[uint]*TvStation{
		101: {Sid: 101, View: 1}, 102: {Sid: 102, Parent: 101, View: 1},
		201: {Sid: 201, Parent: 201, View: 1}, 301: {Sid: 301, View: 0},
	}
	t.Cleanup(func() { station.Cache = previous })
	for _, tt := range []struct{ requested, want []uint }{
		{nil, []uint{101, 201}}, {[]uint{101}, []uint{101}},
		{[]uint{102}, []uint{102}}, {[]uint{301}, []uint{301}}, {[]uint{999}, []uint{}},
	} {
		got := searchServiceIDs(tt.requested)
		slices.Sort(got)
		if !slices.Equal(got, tt.want) {
			t.Fatalf("requested %v: got %v, want %v", tt.requested, got, tt.want)
		}
	}
}

func TestSearchUnknownServiceDoesNotSearchAllStations(t *testing.T) {
	previous := station.Cache
	station.Cache = map[uint]*TvStation{101: {Sid: 101, View: 1}}
	t.Cleanup(func() { station.Cache = previous })
	opt := NewSearch()
	opt.Sid, opt.Title = []uint{999}, "test"
	finished := false
	err := opt.Search("unknown-service", func([]*TvProgram) { t.Error("unexpected search results") }, func(task *SearchTask) {
		finished = true
		if len(task.result) != 0 {
			t.Fatal("unknown selection searched all services")
		}
	})
	if err != nil || !finished {
		t.Fatalf("empty selection did not finish immediately: %v", err)
	}
}

func TestSearchResponseRequestIDAndLegacyProtocol(t *testing.T) {
	previous := deviceController.Handler
	t.Cleanup(func() { deviceController.Handler = previous })
	for _, requestID := range []string{"", "generation-2"} {
		queue := &DevConQueue{Com: make(chan string, 10), Done: make(chan struct{})}
		deviceController.Handler = map[string]*DevConQueue{"search-test": queue}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/search", strings.NewReader(`{"uid":"search-test","requestId":"`+requestID+`"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		APIDoSearch(c) // 空の検索語なので非同期のDB検索は起動しない。
		if w.Code != http.StatusOK {
			t.Fatalf("status %d", w.Code)
		}
		for i := 0; i < 3; i++ {
			select {
			case message := <-queue.Com:
				payload := strings.TrimPrefix(strings.TrimPrefix(message, ControllerResetSearch), ControllerSendSearch)
				if requestID == "" && i == 0 {
					if payload != "" {
						t.Fatal("legacy reset payload changed")
					}
					continue
				}
				var object map[string]json.RawMessage
				if err := json.Unmarshal([]byte(payload), &object); err != nil {
					t.Fatal(err)
				}
				if requestID != "" {
					var got string
					_ = json.Unmarshal(object["requestId"], &got)
					if got != requestID || object["data"] == nil {
						t.Fatalf("untagged response: %s", message)
					}
				} else if object["requestId"] != nil {
					t.Fatal("legacy response was wrapped")
				}
			default:
				t.Fatal("search response missing")
			}
		}
	}
}
