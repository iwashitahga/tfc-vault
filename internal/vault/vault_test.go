package vault

import (
	"errors"
	"strings"
	"testing"

	"github.com/99designs/keyring"
)

func newTestStore(t *testing.T, creds map[string]Credential) *Store {
	t.Helper()
	store := NewStore(keyring.NewArrayKeyring(nil))
	for name, cred := range creds {
		if err := store.Put(name, cred); err != nil {
			t.Fatalf("seeding profile %s: %v", name, err)
		}
	}
	return store
}

func TestPutGetRoundTrip(t *testing.T) {
	store := newTestStore(t, map[string]Credential{
		"work": {Host: "app.terraform.io", Token: "tok"},
	})
	got, err := store.Get("work")
	if err != nil {
		t.Fatal(err)
	}
	if got.Host != "app.terraform.io" || got.Token != "tok" {
		t.Fatalf("Get returned %+v", got)
	}
}

func TestGetMissing(t *testing.T) {
	store := newTestStore(t, nil)
	if _, err := store.Get("absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestDeleteMissing(t *testing.T) {
	store := newTestStore(t, nil)
	if err := store.Delete("absent"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestListIsSorted(t *testing.T) {
	store := newTestStore(t, map[string]Credential{
		"zulu":  {Host: "a.example.com", Token: "1"},
		"alpha": {Host: "b.example.com", Token: "2"},
	})
	names, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "zulu" {
		t.Fatalf("List returned %v", names)
	}
}

func TestResolveHostPrefersProfileNamedAfterHost(t *testing.T) {
	store := newTestStore(t, map[string]Credential{
		"app.terraform.io": {Host: "app.terraform.io", Token: "exact"},
		"personal":         {Host: "app.terraform.io", Token: "other"},
	})
	name, cred, err := store.ResolveHost("app.terraform.io")
	if err != nil {
		t.Fatal(err)
	}
	if name != "app.terraform.io" || cred.Token != "exact" {
		t.Fatalf("resolved to %s / %s", name, cred.Token)
	}
}

func TestResolveHostFindsSingleNamedProfile(t *testing.T) {
	store := newTestStore(t, map[string]Credential{
		"personal":  {Host: "app.terraform.io", Token: "tok"},
		"elsewhere": {Host: "tfe.example.com", Token: "nope"},
	})
	name, cred, err := store.ResolveHost("app.terraform.io")
	if err != nil {
		t.Fatal(err)
	}
	if name != "personal" || cred.Token != "tok" {
		t.Fatalf("resolved to %s / %s", name, cred.Token)
	}
}

func TestResolveHostRejectsAmbiguity(t *testing.T) {
	store := newTestStore(t, map[string]Credential{
		"work":     {Host: "app.terraform.io", Token: "1"},
		"personal": {Host: "app.terraform.io", Token: "2"},
	})
	_, _, err := store.ResolveHost("app.terraform.io")
	if err == nil {
		t.Fatal("expected an error for two profiles on one host")
	}
	if !strings.Contains(err.Error(), "TFC_VAULT_PROFILE") {
		t.Fatalf("error should point at TFC_VAULT_PROFILE, got %v", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatal("ambiguity must not be reported as not-found")
	}
}

func TestResolveHostNotFound(t *testing.T) {
	store := newTestStore(t, map[string]Credential{
		"elsewhere": {Host: "tfe.example.com", Token: "nope"},
	})
	if _, _, err := store.ResolveHost("app.terraform.io"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// A profile that cannot be decoded must not be reported as a plain "not found",
// or a denied keychain prompt would look like a missing token.
func TestResolveHostReportsUnreadableProfile(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	if err := ring.Set(keyring.Item{Key: "broken", Data: []byte("not json")}); err != nil {
		t.Fatal(err)
	}
	store := NewStore(ring)

	_, _, err := store.ResolveHost("app.terraform.io")
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatalf("unreadable profile reported as not-found: %v", err)
	}
	if !strings.Contains(err.Error(), "broken") {
		t.Fatalf("error should name the unreadable profile, got %v", err)
	}
}

// An entry written before the host field existed falls back to its key.
func TestGetDefaultsHostToProfileName(t *testing.T) {
	ring := keyring.NewArrayKeyring(nil)
	if err := ring.Set(keyring.Item{Key: "app.terraform.io", Data: []byte(`{"token":"legacy"}`)}); err != nil {
		t.Fatal(err)
	}
	cred, err := NewStore(ring).Get("app.terraform.io")
	if err != nil {
		t.Fatal(err)
	}
	if cred.Host != "app.terraform.io" {
		t.Fatalf("host defaulted to %q", cred.Host)
	}
}

// A store built on an in-memory ring stands in for the login keychain: there is
// no separate file for tfc-vault to lock.
func TestStoreWithoutDedicatedKeychain(t *testing.T) {
	store := newTestStore(t, nil)
	if store.KeychainName() != LoginKeychain {
		t.Fatalf("KeychainName = %q", store.KeychainName())
	}
	if store.Locked() {
		t.Fatal("a store with no dedicated keychain is never locked")
	}
	if !store.Exists() {
		t.Fatal("a store with no dedicated keychain always exists")
	}
	if err := store.Lock(); err == nil {
		t.Fatal("locking should be refused when there is no dedicated keychain")
	}
}

func TestNameHonoursEnvironment(t *testing.T) {
	t.Setenv("TFC_VAULT_KEYCHAIN", "")
	if Name() != DefaultKeychain {
		t.Fatalf("default name is %q", Name())
	}
	t.Setenv("TFC_VAULT_KEYCHAIN", LoginKeychain)
	if Name() != LoginKeychain {
		t.Fatalf("override ignored, got %q", Name())
	}
}
