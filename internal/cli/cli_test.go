package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/99designs/keyring"
	"github.com/iwashitahga/tfc-vault/internal/vault"
)

func captureStdout(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	previous := stdout
	stdout = buf
	t.Cleanup(func() { stdout = previous })
	return buf
}

func testStore(t *testing.T, creds map[string]vault.Credential) *vault.Store {
	t.Helper()
	store := vault.NewStore(keyring.NewArrayKeyring(nil))
	for name, cred := range creds {
		if err := store.Put(name, cred); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}
	return store
}

func envValue(env []string, key string) (string, bool) {
	for _, kv := range env {
		if name, value, ok := strings.Cut(kv, "="); ok && name == key {
			return value, true
		}
	}
	return "", false
}

func TestCredentialEnvSetsHostVariable(t *testing.T) {
	t.Setenv("TFC_VAULT_UNRELATED", "keep-me")
	env, name, err := credentialEnv(vault.Credential{Host: "app.terraform.io", Token: "tok"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if name != "TF_TOKEN_app_terraform_io" {
		t.Fatalf("variable name is %q", name)
	}
	if got, ok := envValue(env, name); !ok || got != "tok" {
		t.Fatalf("%s = %q (present: %v)", name, got, ok)
	}
	if got, ok := envValue(env, "TFC_VAULT_UNRELATED"); !ok || got != "keep-me" {
		t.Fatal("unrelated variables must be passed through")
	}
}

// A token inherited from the surrounding shell must not survive into the child
// under a name tfc-vault owns, or the child could use the wrong credential.
func TestCredentialEnvDropsInheritedTokens(t *testing.T) {
	t.Setenv("TF_TOKEN_app_terraform_io", "stale")
	t.Setenv("TFE_TOKEN", "stale")

	env, _, err := credentialEnv(vault.Credential{Host: "app.terraform.io", Token: "fresh"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := envValue(env, "TF_TOKEN_app_terraform_io"); got != "fresh" {
		t.Fatalf("host variable is %q, want the freshly read token", got)
	}
	if _, ok := envValue(env, "TFE_TOKEN"); ok {
		t.Fatal("TFE_TOKEN should be dropped when -tfe-token is not given")
	}
	count := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "TF_TOKEN_app_terraform_io=") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("host variable appears %d times", count)
	}
}

func TestCredentialEnvOptionallySetsTFEToken(t *testing.T) {
	env, _, err := credentialEnv(vault.Credential{Host: "app.terraform.io", Token: "tok"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := envValue(env, "TFE_TOKEN"); !ok || got != "tok" {
		t.Fatalf("TFE_TOKEN = %q (present: %v)", got, ok)
	}
}

func TestCredentialEnvRejectsUnencodableHost(t *testing.T) {
	if _, _, err := credentialEnv(vault.Credential{Host: "not a host", Token: "tok"}, false); err == nil {
		t.Fatal("expected an error for a host with no TF_TOKEN_ encoding")
	}
}

func TestHelperGetReturnsToken(t *testing.T) {
	buf := captureStdout(t)
	store := testStore(t, map[string]vault.Credential{
		"app.terraform.io": {Host: "app.terraform.io", Token: "tok"},
	})
	if err := helperGet(store, "app.terraform.io"); err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("helper output is not JSON: %q", buf.String())
	}
	if out["token"] != "tok" {
		t.Fatalf("helper returned %v", out)
	}
}

// Terraform reads an empty object as "no credentials here", so a missing
// profile must not be an error exit.
func TestHelperGetReturnsEmptyObjectWhenMissing(t *testing.T) {
	buf := captureStdout(t)
	store := testStore(t, nil)
	if err := helperGet(store, "app.terraform.io"); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "{}" {
		t.Fatalf("helper printed %q", buf.String())
	}
}

func TestHelperGetRejectsPinnedProfileForAnotherHost(t *testing.T) {
	captureStdout(t)
	t.Setenv("TFC_VAULT_PROFILE", "elsewhere")
	store := testStore(t, map[string]vault.Credential{
		"elsewhere": {Host: "tfe.example.com", Token: "tok"},
	})
	err := helperGet(store, "app.terraform.io")
	if err == nil {
		t.Fatal("expected a refusal to hand over another host's token")
	}
	if !strings.Contains(err.Error(), "tfe.example.com") {
		t.Fatalf("error should name the mismatched host, got %v", err)
	}
}

func TestHelperStoreAndForget(t *testing.T) {
	captureStdout(t)
	store := testStore(t, nil)

	in := strings.NewReader(`{"token":"written"}`)
	if err := helperStore(store, "app.terraform.io", in); err != nil {
		t.Fatal(err)
	}
	cred, err := store.Get("app.terraform.io")
	if err != nil {
		t.Fatal(err)
	}
	if cred.Token != "written" || cred.Host != "app.terraform.io" {
		t.Fatalf("stored %+v", cred)
	}

	if err := helperForget(store, "app.terraform.io"); err != nil {
		t.Fatal(err)
	}
	if names, err := store.List(); err != nil || len(names) != 0 {
		t.Fatalf("profiles after forget: %v (err %v)", names, err)
	}
}

func TestHelperStoreRefusesToOverwriteAnotherHost(t *testing.T) {
	captureStdout(t)
	t.Setenv("TFC_VAULT_PROFILE", "work")
	store := testStore(t, map[string]vault.Credential{
		"work": {Host: "tfe.example.com", Token: "original"},
	})
	err := helperStore(store, "app.terraform.io", strings.NewReader(`{"token":"attacker"}`))
	if err == nil {
		t.Fatal("expected a refusal")
	}
	cred, getErr := store.Get("work")
	if getErr != nil {
		t.Fatal(getErr)
	}
	if cred.Token != "original" {
		t.Fatal("the existing profile was overwritten")
	}
}

func TestHelperStoreRejectsMalformedInput(t *testing.T) {
	captureStdout(t)
	store := testStore(t, nil)
	if err := helperStore(store, "app.terraform.io", strings.NewReader("not json")); err == nil {
		t.Fatal("expected an error for malformed stdin")
	}
	if err := helperStore(store, "app.terraform.io", strings.NewReader(`{}`)); err == nil {
		t.Fatal("expected an error for an object with no token")
	}
}

func TestHelperForgetIsQuietWhenNothingStored(t *testing.T) {
	captureStdout(t)
	if err := helperForget(testStore(t, nil), "app.terraform.io"); err != nil {
		t.Fatalf("forgetting an absent host should succeed, got %v", err)
	}
}

func TestWriteCLIConfigCreatesFile(t *testing.T) {
	captureStdout(t)
	path := filepath.Join(t.TempDir(), ".terraformrc")
	if err := writeCLIConfig(path); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `credentials_helper "tfc-vault"`) {
		t.Fatalf("wrote %q", body)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode is %v", info.Mode().Perm())
	}
}

func TestWriteCLIConfigAppendsAndBacksUp(t *testing.T) {
	captureStdout(t)
	path := filepath.Join(t.TempDir(), ".terraformrc")
	if err := os.WriteFile(path, []byte(`plugin_cache_dir = "/tmp/x"`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeCLIConfig(path); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "plugin_cache_dir") || !strings.Contains(string(body), "credentials_helper") {
		t.Fatalf("append lost content: %q", body)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(backup), "credentials_helper") {
		t.Fatalf("backup should hold the original, got %q", backup)
	}
}

func TestWriteCLIConfigLeavesForeignHelperAlone(t *testing.T) {
	captureStdout(t)
	path := filepath.Join(t.TempDir(), ".terraformrc")
	original := "credentials_helper \"something-else\" {\n  args = []\n}\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeCLIConfig(path); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != original {
		t.Fatalf("file was modified: %q", body)
	}
}

func TestWriteCLIConfigIsIdempotent(t *testing.T) {
	captureStdout(t)
	path := filepath.Join(t.TempDir(), ".terraformrc")
	if err := writeCLIConfig(path); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeCLIConfig(path); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("second run changed the file:\n%q\n%q", first, second)
	}
}

func TestLinkHelperReplacesExistingLink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "terraform-credentials-tfc-vault")
	first := filepath.Join(dir, "first")
	second := filepath.Join(dir, "second")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := linkHelper(first, link); err != nil {
		t.Fatal(err)
	}
	if err := linkHelper(second, link); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if target != second {
		t.Fatalf("link points at %s", target)
	}
	if _, err := os.Stat(link + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("the temporary link was left behind")
	}
}

func TestShredRemovesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.tfrc.json")
	if err := os.WriteFile(path, []byte(`{"credentials":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := shred(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still present: %v", err)
	}
}
