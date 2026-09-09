package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iwashitahga/tfc-vault/internal/tfhost"
	"github.com/iwashitahga/tfc-vault/internal/vault"
)

// DefaultCredentialsFile is where terraform login writes tokens in plaintext.
func DefaultCredentialsFile() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".terraform.d", "credentials.tfrc.json"), nil
}

type credentialsFile struct {
	Credentials map[string]struct {
		Token string `json:"token"`
	} `json:"credentials"`
}

func runImport(args []string) error {
	fs := newFlagSet("import [path]", "Move tokens out of a plaintext credentials.tfrc.json into the keychain.")
	purge := fs.Bool("purge", false, "Overwrite and delete the plaintext file once every token is imported")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		fs.Usage()
		return fmt.Errorf("expected at most one path")
	}

	path := fs.Arg(0)
	if path == "" {
		var err error
		if path, err = DefaultCredentialsFile(); err != nil {
			return err
		}
	}
	return importCredentials(path, *purge)
}

// importCredentials copies every token in a credentials.tfrc.json into the
// keychain, optionally destroying the file afterwards.
func importCredentials(path string, purge bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var parsed credentialsFile
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(parsed.Credentials) == 0 {
		return fmt.Errorf("%s holds no credentials", path)
	}

	store, err := vault.Open(vault.WithPassphrasePrompt(func(string) (string, error) {
		return readNewPassphrase(vault.Name())
	}))
	if err != nil {
		return err
	}
	// Map iteration order is unspecified, so hosts are reported in whatever
	// order they come out; nothing here depends on the order.
	for host, entry := range parsed.Credentials {
		normalized, err := tfhost.Normalize(host)
		if err != nil {
			return err
		}
		if entry.Token == "" {
			return fmt.Errorf("no token for %s in %s", host, path)
		}
		if err := store.Put(normalized, vault.Credential{Host: normalized, Token: entry.Token}); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Imported %s as profile %q.\n", normalized, normalized)
	}

	if purge {
		if err := shred(path); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Deleted %s.\n", path)
	} else {
		warn("%s still holds these tokens in plaintext; re-run with -purge to delete it", path)
	}
	warn("these tokens have been on disk, so rotate them in the web UI when convenient")
	return nil
}

// shred overwrites a file's bytes before unlinking it. On a copy-on-write or
// flash-backed filesystem this cannot guarantee the old blocks are gone, and it
// does nothing about copies in backups, so callers still tell the user to
// rotate the token. It does keep the secret out of the most obvious places.
func shred(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("opening %s to overwrite it: %w", path, err)
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return fmt.Errorf("sizing %s: %w", path, err)
	}
	if _, err := f.Write(make([]byte, info.Size())); err != nil {
		f.Close()
		return fmt.Errorf("overwriting %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("flushing %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", path, err)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("deleting %s: %w", path, err)
	}
	return nil
}
