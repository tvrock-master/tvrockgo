package tvrock

import (
	"testing"
	"time"
)

func TestLoginFailureStoreLocksAfterMaxFailures(t *testing.T) {
	store := loginFailureStore{
		items: map[string]loginFailure{},
	}
	now := time.Date(2026, 8, 26, 18, 0, 0, 0, time.Local)
	key := "192.0.2.1"

	for i := 0; i < loginMaxFailures-1; i++ {
		store.recordFailure(key, now)
		if store.isLocked(key, now) {
			t.Fatalf("isLocked() = true after %d failures, want false", i+1)
		}
	}

	store.recordFailure(key, now)
	if !store.isLocked(key, now) {
		t.Fatal("isLocked() = false after max failures, want true")
	}
}

func TestLoginFailureStoreUnlocksAfterLockout(t *testing.T) {
	store := loginFailureStore{
		items: map[string]loginFailure{},
	}
	now := time.Date(2026, 8, 26, 18, 0, 0, 0, time.Local)
	key := "192.0.2.1"

	for i := 0; i < loginMaxFailures; i++ {
		store.recordFailure(key, now)
	}

	if store.isLocked(key, now.Add(loginLockout-time.Second)) == false {
		t.Fatal("isLocked() = false before lockout expires, want true")
	}
	if store.isLocked(key, now.Add(loginLockout+time.Second)) {
		t.Fatal("isLocked() = true after lockout expires, want false")
	}
}

func TestLoginFailureStoreReset(t *testing.T) {
	store := loginFailureStore{
		items: map[string]loginFailure{},
	}
	now := time.Date(2026, 8, 26, 18, 0, 0, 0, time.Local)
	key := "192.0.2.1"

	for i := 0; i < loginMaxFailures; i++ {
		store.recordFailure(key, now)
	}
	store.reset(key)

	if store.isLocked(key, now) {
		t.Fatal("isLocked() = true after reset, want false")
	}
}
