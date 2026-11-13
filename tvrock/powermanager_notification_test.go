package tvrock

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPowerManagerUpdateNotifiesAllClientsAfterSave(t *testing.T) {
	previousSetting := configure.PowerManagerSetting
	previousHandlers := deviceController.Handler
	t.Cleanup(func() {
		configure.PowerManagerSetting = previousSetting
		deviceController.Handler = previousHandlers
	})

	for _, saveOK := range []bool{true, false} {
		name := "save failure"
		if saveOK {
			name = "save success"
		}
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if saveOK {
				if err := os.Mkdir("conf", 0700); err != nil {
					t.Fatal(err)
				}
			}
			queues := []*DevConQueue{
				{Com: make(chan string, 1), Done: make(chan struct{})},
				{Com: make(chan string, 1), Done: make(chan struct{})},
			}
			deviceController.Handler = map[string]*DevConQueue{"app": queues[0], "rockbar": queues[1]}
			configure.PowerManagerSetting.SleepTimerPause = false
			router := gin.New()
			router.SetHTMLTemplate(template.Must(template.New(templateError500).Parse("save failed")))
			router.PUT("/pms", APIUpdatePowerManagerSetting)
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPut, "/pms", strings.NewReader(`{"sleeptimerpause":true}`))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(response, request)
			wantStatus := http.StatusInternalServerError
			if saveOK {
				wantStatus = http.StatusOK
			}
			if response.Code != wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, wantStatus)
			}
			for _, queue := range queues {
				select {
				case message := <-queue.Com:
					if !saveOK || message != ControllerUpdatePowerManagerSetting {
						t.Fatalf("unexpected notification %q", message)
					}
				default:
					if saveOK {
						t.Fatal("client did not receive power manager notification")
					}
				}
			}
		})
	}
}
