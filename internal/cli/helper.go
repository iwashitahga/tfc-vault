package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/iwashitahga/tfc-vault/internal/tfhost"
	"github.com/iwashitahga/tfc-vault/internal/vault"
)

// Helper implements Terraform's credentials helper protocol. Terraform runs the
// binary as `terraform-credentials-tfc-vault <verb> <hostname>` and reads a
// JSON object from stdout; an empty object means "no credentials for this host".
func Helper(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: tfc-vault helper <get|store|forget> <hostname>")
	}
	verb, host := args[0], args[1]
	host, err := tfhost.Normalize(host)
	if err != nil {
		return err
	}

	store, err := vault.Open()
	if err != nil {
		return err
	}

	switch verb {
	case "get":
		return helperGet(store, host)
	case "store":
		return helperStore(store, host, os.Stdin)
	case "forget":
		return helperForget(store, host)
	default:
		return fmt.Errorf("unsupported credentials helper verb %q", verb)
	}
}

func helperGet(store *vault.Store, host string) error {
	var cred vault.Credential
	var err error

	profile := os.Getenv("TFC_VAULT_PROFILE")
	if profile != "" {
		cred, err = store.Get(profile)
	} else {
		_, cred, err = store.ResolveHost(host)
	}
	if errors.Is(err, vault.ErrNotFound) {
		// Terraform falls back to its other credential sources on an empty object.
		fmt.Fprintln(stdout, "{}")
		return nil
	}
	if err != nil {
		return err
	}
	// A pinned profile that belongs to another host would hand Terraform a
	// token for the wrong service, so refuse rather than answer.
	if cred.Host != host {
		return fmt.Errorf("TFC_VAULT_PROFILE names profile %s, which holds a token for %s, not %s", profile, cred.Host, host)
	}
	return json.NewEncoder(stdout).Encode(map[string]string{"token": cred.Token})
}

func helperStore(store *vault.Store, host string, r io.Reader) error {
	body, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return err
	}
	var in struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &in); err != nil {
		return fmt.Errorf("reading credentials from stdin: %w", err)
	}
	if in.Token == "" {
		return fmt.Errorf("no token in the credentials object")
	}
	profile := os.Getenv("TFC_VAULT_PROFILE")
	if profile == "" {
		profile = host
	}
	// Never overwrite a profile that belongs to a different host.
	if existing, err := store.Get(profile); err == nil && existing.Host != host {
		return fmt.Errorf("profile %s already holds a token for %s; refusing to overwrite it with one for %s", profile, existing.Host, host)
	} else if err != nil && !errors.Is(err, vault.ErrNotFound) {
		return err
	}
	return store.Put(profile, vault.Credential{Host: host, Token: in.Token})
}

func helperForget(store *vault.Store, host string) error {
	profile := os.Getenv("TFC_VAULT_PROFILE")
	if profile == "" {
		name, _, err := store.ResolveHost(host)
		if errors.Is(err, vault.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		profile = name
	}
	if err := store.Delete(profile); err != nil && !errors.Is(err, vault.ErrNotFound) {
		return err
	}
	return nil
}
