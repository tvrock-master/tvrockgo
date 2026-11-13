package tvrock

import (
	"path/filepath"
	"testing"
)

func TestRegistryCodeSurvivesConfigReloadAndDeviceCreation(t *testing.T) {
	conf := TvRockConf{
		SessionSecretKey: "already-configured-secret",
		Devices:          []RockDevice{{ID: "REC01"}},
	}
	if !conf.BuildHash() {
		t.Fatal("missing RegistryCode was not generated")
	}
	want := conf.Devices[0].RegistryCode
	path := filepath.Join(t.TempDir(), "tvrock.yml")
	if err := conf.Save(path); err != nil {
		t.Fatal(err)
	}
	var loaded TvRockConf
	if err := loaded.Load(path); err != nil {
		t.Fatal(err)
	}
	if loaded.BuildHash() {
		t.Fatal("BuildHash changed the saved configuration")
	}
	if loaded.Devices[0].RegistryCode != want {
		t.Fatal("saved RegistryCode was overwritten")
	}
	for _, master := range []string{"", "MASTER01"} {
		t.Run("master="+master, func(t *testing.T) {
			dev := newRockDevice("REC01", master, loaded.Devices[0])
			if dev.RegistryCode != want {
				t.Fatalf("RegistryCode = %q, want saved value %q", dev.RegistryCode, want)
			}
			if dev.Master != master {
				t.Fatalf("Master = %q, want %q", dev.Master, master)
			}
			s := Slot{Device: map[string]*RockDevice{dev.ID: dev}}
			if s.IsBootingByRID(want) {
				t.Fatal("saved RegistryCode incorrectly treated as already booting")
			}
			if found, ok := s.GetByRID(want); !ok || found != dev {
				t.Fatal("device cannot be found using saved RegistryCode")
			}
		})
	}
}
