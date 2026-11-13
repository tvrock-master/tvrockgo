package tvrock

import (
	"encoding/hex"
	"testing"

	"golang.org/x/crypto/bcrypt"
	yaml "gopkg.in/yaml.v2"
)

func TestBuildHashGeneratesSessionSecretKey(t *testing.T) {
	conf := TvRockConf{
		SessionSecretKey: "",
		Devices: []RockDevice{
			{ID: "REC01"},
		},
	}

	if !conf.BuildHash() {
		t.Fatal("BuildHash() = false, want true")
	}

	if len(conf.SessionSecretKey) != 64 {
		t.Fatalf("len(SessionSecretKey) = %d, want 64", len(conf.SessionSecretKey))
	}
	if _, err := hex.DecodeString(conf.SessionSecretKey); err != nil {
		t.Fatalf("SessionSecretKey is not hex: %v", err)
	}
	if conf.Devices[0].RegistryCode == "" {
		t.Fatal("RegistryCode was not generated")
	}
}

func TestBuildHashKeepsExistingSessionSecretKey(t *testing.T) {
	conf := TvRockConf{
		SessionSecretKey: "already-configured-secret",
	}

	if conf.BuildHash() {
		t.Fatal("BuildHash() = true, want false")
	}
	if conf.SessionSecretKey != "already-configured-secret" {
		t.Fatal("SessionSecretKey was overwritten")
	}
}

func TestBuildHashGeneratesUserAccountPasswordHash(t *testing.T) {
	conf := TvRockConf{
		SessionSecretKey: "already-configured-secret",
		UserAccount: map[string]string{
			"user1": "password1",
		},
	}

	if !conf.BuildHash() {
		t.Fatal("BuildHash() = false, want true")
	}

	hash := conf.UserAccount["user1"]
	if hash == "password1" {
		t.Fatal("password was not hashed")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("password1")); err != nil {
		t.Fatalf("hashed password does not match: %v", err)
	}
}

func TestBuildHashKeepsExistingUserAccountPasswordHash(t *testing.T) {
	passwordHash := generatePasswordHash("password1")
	conf := TvRockConf{
		SessionSecretKey: "already-configured-secret",
		UserAccount: map[string]string{
			"user1": passwordHash,
		},
	}

	if conf.BuildHash() {
		t.Fatal("BuildHash() = true, want false")
	}
	if conf.UserAccount["user1"] != passwordHash {
		t.Fatal("password hash was overwritten")
	}
}

func TestAuthenticateUserWithPasswordHash(t *testing.T) {
	oldUsers := configure.UserAccount
	defer func() {
		configure.UserAccount = oldUsers
	}()

	configure.UserAccount = map[string]string{
		"user1": generatePasswordHash("password1"),
	}

	if !authenticateUser("user1", "password1") {
		t.Fatal("authenticateUser() = false, want true")
	}
	if authenticateUser("user1", "wrong-password") {
		t.Fatal("authenticateUser() = true, want false")
	}
}

func TestNormalizeFeaturesDefault(t *testing.T) {
	conf := TvRockConf{}
	normalizeFeatures(&conf)

	if len(conf.Features) != 4 {
		t.Fatalf("expected 4 default features, got %d", len(conf.Features))
	}
	expectedNames := []string{"new", "end", "first", "free"}
	for i, name := range expectedNames {
		if conf.Features[i].Name != name {
			t.Errorf("expected feature[%d].Name = %s, got %s", i, name, conf.Features[i].Name)
		}
		if conf.Features[i].Pattern == "" {
			t.Errorf("expected feature[%d].Pattern to be non-empty", i)
		}
	}
}

func TestNormalizeFeaturesCustom(t *testing.T) {
	custom := []FeaturePattern{
		{Name: "special", Pattern: `.*SP.*`},
	}
	conf := TvRockConf{
		Features: custom,
	}
	normalizeFeatures(&conf)

	if len(conf.Features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(conf.Features))
	}
	if conf.Features[0].Name != "special" {
		t.Errorf("expected feature name 'special', got %s", conf.Features[0].Name)
	}
}

func TestFeaturesYAMLUnmarshal(t *testing.T) {
	yamlData := `
features:
  - name: new
    pattern: '.*(🈟|\[新\])|（新）|\(新\)|＜新＞.*'
  - name: end
    pattern: '.*(🈡|\[終\])|（終）|\(終\)|＜終＞.*'
`
	var conf TvRockConf
	if err := yaml.Unmarshal([]byte(yamlData), &conf); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	if len(conf.Features) != 2 {
		t.Fatalf("expected 2 features, got %d", len(conf.Features))
	}
	if conf.Features[0].Name != "new" {
		t.Errorf("expected feature[0].Name = new, got %s", conf.Features[0].Name)
	}
	if conf.Features[1].Name != "end" {
		t.Errorf("expected feature[1].Name = end, got %s", conf.Features[1].Name)
	}
}
