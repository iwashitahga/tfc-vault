package cli

import (
	"fmt"

	"github.com/iwashitahga/tfc-vault/internal/tfhost"
	"github.com/iwashitahga/tfc-vault/internal/vault"
)

func runAdd(args []string) error {
	fs := newFlagSet("add [host]", "Store a Terraform API token in the keychain.")
	profile := fs.String("profile", "", "Name to file the token under (default: the hostname)")
	open := fs.Bool("open", false, "Open the host's token page in a browser first")
	noVerify := fs.Bool("no-verify", false, "Skip checking the token against the host")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() > 1 {
		fs.Usage()
		return fmt.Errorf("expected at most one hostname")
	}
	host := tfhost.Default
	if fs.NArg() == 1 {
		host = fs.Arg(0)
	}
	host, err := tfhost.Normalize(host)
	if err != nil {
		return err
	}
	if _, err := tfhost.EnvName(host); err != nil {
		return err
	}

	name := *profile
	if name == "" {
		name = host
	}

	if *open {
		openBrowser(tfhost.TokensURL(host))
	}

	token, err := readToken(fmt.Sprintf("Token for %s: ", host))
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("no token given")
	}

	if !*noVerify {
		if err := verifyToken(host, token); err != nil {
			return fmt.Errorf("%w (pass -no-verify to store it anyway)", err)
		}
	}

	store, err := vault.Open(vault.WithPassphrasePrompt(func(string) (string, error) {
		return readNewPassphrase(vault.Name())
	}))
	if err != nil {
		return err
	}
	if err := store.Put(name, vault.Credential{Host: host, Token: token}); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Stored token for %s as profile %q.\n", host, name)
	return nil
}
