package tvrock

import (
	"bufio"
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TvRockDTV側のexamples/device_lifecycle_probeを指定した場合のみ、
// 本物のRustクライアントとGoのデバイス受信処理をループバックで接続する。
func TestDeviceLifecycleWithRustClient(t *testing.T) {
	probe := os.Getenv("TVROCK_DTV_LIFECYCLE_PROBE")
	if probe == "" {
		t.Skip("TVROCK_DTV_LIFECYCLE_PROBE is not configured")
	}
	for _, mode := range []string{"reconnect shutdown", "next recording"} {
		t.Run(mode, func(t *testing.T) {
			d := shutdownTestDevice(t)
			router := gin.New()
			router.GET("/dev", SockDeviceTerminal)
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, probe, server.URL, d.RegistryCode)
			configureBackgroundCommand(cmd)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cancel(); _ = cmd.Wait() })
			lines := make(chan string, 32)
			go func() {
				defer close(lines)
				scanner := bufio.NewScanner(stdout)
				for scanner.Scan() {
					lines <- scanner.Text()
				}
			}()
			await := func(want string) {
				t.Helper()
				for {
					select {
					case line, ok := <-lines:
						if !ok {
							t.Fatalf("probe ended before %q", want)
						}
						if strings.HasPrefix(line, want) {
							return
						}
						if strings.HasPrefix(line, "event 19 ") {
							t.Fatalf("unexpected PowerOff before %q", want)
						}
					case <-ctx.Done():
						t.Fatalf("probe timeout before %q", want)
					}
				}
			}
			await("ready 1")
			if mode == "reconnect shutdown" {
				// Rustクライアントの実際の再接続処理を使用する。
				if slot.Send(&RequestMessage{ID: d.ID, Command: "exit"}) != 1 {
					t.Fatal("disconnect command not queued")
				}
				limit := time.Now().Add(3 * time.Second)
				for {
					slot.RLock()
					active := d.connectionActive
					slot.RUnlock()
					if !active {
						break
					}
					if time.Now().After(limit) {
						t.Fatal("old connection did not finish")
					}
					time.Sleep(time.Millisecond)
				}
				r := pendingTestShutdown(t, d)
				r.run() // 切断中に終了要求。次の接続でPowerOffが届く必要がある。
				await("ready 2")
			} else {
				r := pendingTestShutdown(t, d)
				// 古いPowerOffがキューに入った後に後続予約へ制御権を渡す。
				scheduler.deviceMu.Lock()
				r.run()
				scheduler.CurrentRecording[d.ID] = &RecordingSchedule{ID: 990002, DeviceID: d.ID}
				(&RecordingJob{DeviceID: d.ID}).ShutdownCancel()
				scheduler.deviceMu.Unlock()
				for _, command := range []RequestMessage{
					{ID: d.ID, Command: "StreamChange", Message: "131173"},
					{ID: d.ID, Command: "RecordStart", Message: "simulated-recording.ts"},
					{ID: d.ID, Command: "RecordStop"},
				} {
					if slot.Send(&command) != 1 {
						t.Fatal("recording command not queued")
					}
				}
				await("event 20 ")
				await("event 16 ")
				await("event 17 ")
				limit := time.Now().Add(3 * time.Second)
				for {
					slot.RLock()
					state := d.State
					slot.RUnlock()
					if state == StateReady {
						break
					}
					if time.Now().After(limit) {
						t.Fatal("record stop was not acknowledged")
					}
					time.Sleep(time.Millisecond)
				}
				scheduler.Lock()
				delete(scheduler.CurrentRecording, d.ID)
				scheduler.Unlock()
				r = pendingTestShutdown(t, d)
				r.run()
			}
			await("event 19 ")
			await("poweroff-stopped")
			if err := cmd.Wait(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
