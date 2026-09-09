package cli

import (
	"fmt"

	"github.com/iwashitahga/tfc-vault/internal/tfhost"
	"github.com/iwashitahga/tfc-vault/internal/vault"
)

func runEnv(args []string) error {
	fs := newFlagSet("env <profile>", "Print shell export lines for a profile's token.")
	tfeToken := fs.Bool("tfe-token", false, "Also export TFE_TOKEN")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("expected exactly one profile name")
	}

	store, err := vault.Open()
	if err != nil {
		return err
	}
	cred, err := lookup(store, fs.Arg(0))
	if err != nil {
		return err
	}
	name, err := tfhost.EnvName(cred.Host)
	if err != nil {
		return err
	}

	warn("warning: this puts the token into your shell environment and possibly its history; prefer 'tfc-vault exec'")
	fmt.Fprintf(stdout, "export %s=%s\n", name, cred.Token)
	if *tfeToken {
		fmt.Fprintf(stdout, "export TFE_TOKEN=%s\n", cred.Token)
	}
	return nil
}

func runGet(args []string) error {
	fs := newFlagSet("get <profile>", "Print a profile's raw token on stdout.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return fmt.Errorf("expected exactly one profile name")
	}
	store, err := vault.Open()
	if err != nil {
		return err
	}
	cred, err := lookup(store, fs.Arg(0))
	if err != nil {
		return err
	}
	warn("warning: this writes the token to stdout, where terminal scrollback and shell history can keep it; prefer 'tfc-vault exec'")
	fmt.Fprintln(stdout, cred.Token)
	return nil
}
