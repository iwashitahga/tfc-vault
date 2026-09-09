package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/iwashitahga/tfc-vault/internal/tfhost"
	"github.com/iwashitahga/tfc-vault/internal/vault"
)

// lookup accepts either a profile name or, for convenience, a hostname.
func lookup(store *vault.Store, name string) (vault.Credential, error) {
	cred, err := store.Get(name)
	if err == nil {
		return cred, nil
	}
	if !errors.Is(err, vault.ErrNotFound) {
		return vault.Credential{}, err
	}
	if host, hostErr := tfhost.Normalize(name); hostErr == nil {
		if _, cred, err := store.ResolveHost(host); err == nil {
			return cred, nil
		}
	}
	return vault.Credential{}, fmt.Errorf("%w: %s (see: tfc-vault list)", vault.ErrNotFound, name)
}

// credentialEnv returns the environment to run a child process under: the
// current one with the token's variables set and any inherited copies dropped.
//
// TFE_TOKEN is dropped whether or not it is being set, so a token inherited
// from the surrounding shell can never reach the child under a name tfc-vault
// is responsible for.
func credentialEnv(cred vault.Credential, alsoTFE bool) ([]string, string, error) {
	name, err := tfhost.EnvName(cred.Host)
	if err != nil {
		return nil, "", err
	}
	drop := map[string]bool{name: true, "TFE_TOKEN": true}

	env := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if !drop[key] {
			env = append(env, kv)
		}
	}
	env = append(env, name+"="+cred.Token)
	if alsoTFE {
		env = append(env, "TFE_TOKEN="+cred.Token)
	}
	return env, name, nil
}

func runExec(args []string) error {
	fs := newFlagSet("exec <profile> -- <command> [args...]", "Run a command with the token in its environment.")
	tfeToken := fs.Bool("tfe-token", false, "Also set TFE_TOKEN, for the tfe provider and go-tfe tools")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return fmt.Errorf("expected a profile name")
	}
	profile, rest := rest[0], rest[1:]
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		fs.Usage()
		return fmt.Errorf("expected a command to run after the profile")
	}

	store, err := vault.Open()
	if err != nil {
		return err
	}
	cred, err := lookup(store, profile)
	if err != nil {
		return err
	}
	env, _, err := credentialEnv(cred, *tfeToken)
	if err != nil {
		return err
	}

	path, err := exec.LookPath(rest[0])
	if err != nil {
		return err
	}
	// Replace this process so signals, exit status and the terminal all belong
	// to the child, and the token never outlives it.
	if err := syscall.Exec(path, rest, env); err != nil {
		return fmt.Errorf("running %s: %w", path, err)
	}
	return nil
}
