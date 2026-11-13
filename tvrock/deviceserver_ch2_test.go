package tvrock

import (
	"bytes"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestCH2AndSDTRefreshServicesWithoutChangingRecordingState(t *testing.T) {
	for _, tc := range []struct {
		name     string
		tuner    TunerType
		tsp      TuningSpace
		existing bool
		hasCH2   bool
	}{
		{"terrestrial-empty", TunerTypeTS, TuningSpaceTS, false, false},
		{"satellite-empty", TunerTypeBSCS, TuningSpaceBS, false, false},
		{"terrestrial-existing", TunerTypeTS, TuningSpaceTS, true, false},
		{"satellite-existing", TunerTypeBSCS, TuningSpaceBS, true, false},
		{"unknown-tuner-terrestrial", TunerType(255), TuningSpaceTS, false, false},
		{"unknown-tuner-satellite", TunerType(255), TuningSpaceBS, false, false},
		{"terrestrial-with-CH2", TunerTypeTS, TuningSpaceTS, false, true},
		{"satellite-with-CH2", TunerTypeBSCS, TuningSpaceBS, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			db.SetMaxOpenConns(1)
			if err := CreateTable(db); err != nil {
				t.Fatal(err)
			}
			oldCache, oldDB := station.Cache, station.db
			t.Cleanup(func() { station.Cache, station.db = oldCache, oldDB })
			const existingSID = uint(1<<16 | 999)
			station.Cache = map[uint]*TvStation{}
			if tc.existing {
				station.Cache[existingSID] = &TvStation{Sid: existingSID, Tsp: 1, Name: "既存局", Ch: 14, View: 1}
			}
			station.db = &RockDB{Data: []*sql.DB{db}}
			d := shutdownTestDevice(t)
			d.TunerType = tc.tuner
			oldDevices := configure.Devices
			configure.Devices = []RockDevice{*d}
			t.Cleanup(func() { configure.Devices = oldDevices })
			var logs bytes.Buffer
			oldLog := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(oldLog) })
			r := gin.New()
			r.GET("/dev", SockDeviceTerminal)
			server := httptest.NewServer(r)
			defer server.Close()
			h := http.Header{"Authorization": {"Bearer " + d.RegistryCode}, "User-Agent": {"TvRockDTV/test"}}
			c, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/dev", h)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				_ = c.Close()
				serviceWorkers.wg.Wait()
				for _, want := range []string{"error: .ch2ファイルが見つかりません", "warn: 不定選局状態"} {
					if !tc.hasCH2 && !strings.Contains(logs.String(), want) {
						t.Errorf("missing log %q", want)
					}
					if tc.hasCH2 && strings.Contains(logs.String(), want) {
						t.Errorf("unexpected missing CH2 log %q", want)
					}
				}
			}()
			_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
			var message RequestMessage
			if err := c.ReadJSON(&message); err != nil || message.Command != "LoopBack" {
				t.Fatalf("initial command: %+v, %v", message, err)
			}
			ch2 := "CH2:file not found"
			if tc.hasCH2 {
				space := "UHF"
				if tc.tsp == TuningSpaceBS {
					space = "BS"
				}
				// 視聴アプリの初期一覧で子が無効でも、後のSDTから発見して追加する。
				ch2 = fmt.Sprintf("CH2:; 視聴アプリ チャンネル設定ファイル|;#SPACE(0,%s)|親局,0,14,0,0,101,0,1234,1|子局,0,14,0,0,102,0,1234,0", space)
			}
			if err := c.WriteMessage(websocket.TextMessage, []byte(ch2)); err != nil {
				t.Fatal(err)
			}
			if err := c.ReadJSON(&message); err != nil || message.Command != "Ready" {
				t.Fatalf("CH2 did not complete initialization: %+v, %v", message, err)
			}
			want := fmt.Sprintf("%d", tc.tuner)
			if tc.existing {
				want += ",999"
			}
			if tc.hasCH2 {
				want += ",101"
			}
			if message.Message != want {
				t.Fatalf("Ready = %q, want %q", message.Message, want)
			}
			if !slot.IsReady(d.ID) {
				t.Fatal("device is not ready")
			}
			if err := c.WriteMessage(websocket.TextMessage, []byte("STA:3")); err != nil {
				t.Fatal(err)
			}
			if tc.tuner == TunerType(255) {
				// 空・未知ONID・他TS・種別混在では放送種別を確定しない。
				for _, payload := range []string{
					`SDT:[]`,
					`SDT:[{"TableId":66,"Onid":0,"Sid":401,"ServiceType":1}]`,
					`SDT:[{"TableId":70,"Onid":4,"Sid":402,"ServiceType":1}]`,
					`SDT:[{"TableId":66,"Onid":4,"Sid":403,"ServiceType":1},{"TableId":70,"Onid":4,"Sid":404,"ServiceType":1}]`,
					`SDT:[{"TableId":66,"Onid":4,"Sid":405,"ServiceType":1},{"TableId":66,"Onid":32736,"Sid":406,"ServiceType":1}]`,
				} {
					if err := c.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
						t.Fatal(err)
					}
				}
			}
			onid := uint(32736)
			if tc.tsp == TuningSpaceBS {
				onid = 4
			}
			// STC到着前に、既定のチューナー種別またはSDTのONIDから局を登録する。
			payload := fmt.Sprintf(`SDT:[{"TableId":66,"Onid":%d,"Sid":101,"Tsid":1234,"ServiceType":1,"OneSeg":0,"ServiceName":"SDTテスト局"},{"TableId":66,"Onid":%d,"Sid":102,"Tsid":1234,"ServiceType":1,"OneSeg":0,"ServiceName":"SDTサブ局"}]`, onid, onid)
			if err := c.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
				t.Fatal(err)
			}
			if err := c.ReadJSON(&message); err != nil || message.Command != "Ready" {
				t.Fatalf("updated service list: %+v, %v", message, err)
			}
			if !strings.Contains(","+message.Message+",", ",101,") {
				t.Fatalf("SDT service missing from Ready: %q", message.Message)
			}
			if !strings.Contains(","+message.Message+",", ",102,") {
				t.Fatalf("second SDT service missing: %q", message.Message)
			}
			slot.RLock()
			state := d.State
			slot.RUnlock()
			if state != StateRecording {
				t.Fatalf("service update changed recording state: %v", state)
			}
			wantSID := uint(tc.tsp)<<16 | 101
			deadline := time.Now().Add(5 * time.Second)
			for {
				loaded, err := station.GetStations(CurrentSlot)
				if err != nil {
					t.Fatal(err)
				}
				if got := loaded[wantSID]; got != nil && got.Name == "SDTテスト局" && got.Tsid == 1234 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("SDT station was not persisted")
				}
				time.Sleep(time.Millisecond)
			}
			if got, ok := station.Get(existingSID); tc.existing && (!ok || got.Name != "既存局" || got.Ch != 14) {
				t.Fatal("existing station was changed")
			}
			wantStations := 2
			if tc.existing {
				wantStations++
			}
			if len(station.GetStationList()) != wantStations {
				t.Fatal("invalid SDT registered unexpected stations")
			}
			// Ready受信後にDLLがSDTを再送しても、同じ一覧を返信し続けない。
			if err := c.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
				t.Fatal(err)
			}
			if err := c.WriteMessage(websocket.TextMessage, []byte("STA:2")); err != nil {
				t.Fatal(err)
			}
			for {
				slot.RLock()
				state = d.State
				slot.RUnlock()
				if state == StateReady {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("record stop was not acknowledged")
				}
				time.Sleep(time.Millisecond)
			}
			// DEVICE画面の終了操作と同じランチャー経路でPowerOffを送る。
			launcher := &Launcher{}
			launcher.Exec(&LaunchMessage{DeviceID: d.ID, RegistryID: d.RegistryCode, BootType: DeviceBootOFF})
			if err := c.ReadJSON(&message); err != nil || message.Command != "PowerOff" {
				t.Fatalf("shutdown command: %+v, %v", message, err)
			}
		})
	}
}

func TestFetchTimeoutUsesServiceBandOrTunerFallback(t *testing.T) {
	previous := configure.FetchProgram
	t.Cleanup(func() { configure.FetchProgram = previous })
	configure.FetchProgram.TimeoutTS = 11
	configure.FetchProgram.TimeoutBS = 22
	configure.FetchProgram.TimeoutCS = 33
	configure.FetchProgram.TimeoutOS = 44
	for _, tc := range []struct {
		name  string
		tuner TunerType
		tsp   TuningSpace
		want  int
	}{
		{"terrestrial", TunerTypeBSCS, TuningSpaceTS, 11},
		{"BS", TunerTypeTS, TuningSpaceBS, 22},
		{"CS", TunerTypeTS, TuningSpaceCS, 33},
		{"oneseg", TunerTypeTS, TuningSpaceOS, 44},
		{"4K", TunerTypeTS, TuningSpace4K, 33},
		{"terrestrial-fallback", TunerTypeTS, TuningSpaceNone, 11},
		{"satellite-fallback", TunerTypeBSCS, TuningSpaceNone, 33},
		{"4K-fallback", TunerType4K, TuningSpaceNone, 33},
		{"unknown", TunerType(255), TuningSpaceNone, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := getFetchTimeOutSec(tc.tuner, tc.tsp); got != tc.want {
				t.Fatalf("timeout = %d, want %d", got, tc.want)
			}
		})
	}
}
