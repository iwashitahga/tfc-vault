package cli

import (
	"fmt"

	"github.com/iwashitahga/tfc-vault/internal/vault"
)

func runRemove(args []string) error {
	fs := newFlagSet("remove <profile>", "Delete a profile from the keychain.")
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
	if err := store.Delete(fs.Arg(0)); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Removed profile %q. Revoke the token in the web UI as well if it may have leaked.\n", fs.Arg(0))
	return nil
}
