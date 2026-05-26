package server

import (
	"testing"
	"time"
)

func TestShareManager_CreateAndResolve(t *testing.T) {
	m := NewShareManager()
	defer m.Shutdown()

	share, err := m.Create("work", ShareModeViewer, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if share.Token == "" {
		t.Fatal("empty token")
	}
	if share.Mode != ShareModeViewer {
		t.Fatalf("mode = %q, want viewer", share.Mode)
	}

	got, ok := m.Resolve(share.Token)
	if !ok || got != share {
		t.Fatal("Resolve did not return the created share")
	}
}

func TestShareManager_InvalidMode(t *testing.T) {
	m := NewShareManager()
	defer m.Shutdown()
	if _, err := m.Create("s", "spectator", time.Minute); err == nil {
		t.Fatal("expected invalid-mode error")
	}
}

func TestShareManager_RejectsNonPositiveTTL(t *testing.T) {
	m := NewShareManager()
	defer m.Shutdown()
	for _, ttl := range []time.Duration{0, -time.Second} {
		if _, err := m.Create("s", ShareModeViewer, ttl); err == nil {
			t.Fatalf("ttl=%v: expected error", ttl)
		}
	}
}

func TestShareManager_RejectsExcessiveTTL(t *testing.T) {
	m := NewShareManager()
	defer m.Shutdown()
	if _, err := m.Create("s", ShareModeViewer, 31*24*time.Hour); err == nil {
		t.Fatal("expected ttl-too-long error")
	}
}

func TestShareManager_ExpiredShareNotResolved(t *testing.T) {
	m := NewShareManager()
	defer m.Shutdown()

	// Create a share with a tiny TTL, then wait past it.
	share, err := m.Create("s", ShareModeViewer, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	if _, ok := m.Resolve(share.Token); ok {
		t.Fatal("expected expired share to fail Resolve")
	}
	// And it should be evicted from the map after the failed lookup.
	for _, s := range m.List() {
		if s.Token == share.Token {
			t.Fatal("expected expired share to be evicted from List")
		}
	}
}

func TestShareManager_Revoke(t *testing.T) {
	m := NewShareManager()
	defer m.Shutdown()
	share, _ := m.Create("s", ShareModeWriter, time.Minute)
	if !m.Revoke(share.Token) {
		t.Fatal("first Revoke should succeed")
	}
	if m.Revoke(share.Token) {
		t.Fatal("second Revoke should return false")
	}
	if _, ok := m.Resolve(share.Token); ok {
		t.Fatal("revoked token should not Resolve")
	}
}

func TestShareManager_RevokeForSession(t *testing.T) {
	m := NewShareManager()
	defer m.Shutdown()
	a, _ := m.Create("alpha", ShareModeViewer, time.Minute)
	b, _ := m.Create("alpha", ShareModeWriter, time.Minute)
	c, _ := m.Create("beta", ShareModeViewer, time.Minute)

	m.RevokeForSession("alpha")

	if _, ok := m.Resolve(a.Token); ok {
		t.Fatal("alpha viewer share should be revoked")
	}
	if _, ok := m.Resolve(b.Token); ok {
		t.Fatal("alpha writer share should be revoked")
	}
	if _, ok := m.Resolve(c.Token); !ok {
		t.Fatal("beta share should survive RevokeForSession('alpha')")
	}
}
