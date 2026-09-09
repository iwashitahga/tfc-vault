package cli

import (
	"fmt"

	"github.com/iwashitahga/tfc-vault/internal/vault"
)

func runLock(args []string) error {
	fs := newFlagSet("lock", "Lock the tfc-vault keychain now, so the next read asks for the passphrase.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return fmt.Errorf("lock takes no arguments")
	}
	store, err := vault.Open()
	if err != nil {
		return err
	}
	if err := store.Lock(); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Locked the %s keychain.\n", store.KeychainName())
	return nil
}

// runUnlock exists for sessions with no window server, such as ssh, where the
// system passphrase dialog cannot be drawn. Everywhere else, letting macOS ask
// is better: the dialog is drawn by a separate system process, so tfc-vault
// never holds the passphrase, and it says which application is asking.
func runUnlock(args []string) error {
	fs := newFlagSet("unlock", "Unlock the keychain by typing the passphrase here, for sessions with no macOS dialog.")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return fmt.Errorf("unlock takes no arguments")
	}
	store, err := vault.Open()
	if err != nil {
		return err
	}
	if !store.Locked() {
		fmt.Fprintf(stdout, "The %s keychain is already unlocked.\n", store.KeychainName())
		return nil
	}
	warn("note: this reads the passphrase into tfc-vault. Where macOS can draw its own")
	warn("dialog, running the command you actually want is safer than unlocking here.")
	passphrase, err := readSecret(fmt.Sprintf("Passphrase for the %s keychain: ", store.KeychainName()))
	if err != nil {
		return err
	}
	if err := store.Unlock(passphrase); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Unlocked the %s keychain.\n", store.KeychainName())
	return nil
}

func runMigrate(args []string) error {
	fs := newFlagSet("migrate", "Copy profiles out of another keychain into the one tfc-vault uses now.")
	from := fs.String("from", vault.LoginKeychain, "Keychain to copy from")
	remove := fs.Bool("remove", false, "Delete each profile from the source keychain once it has been copied")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return fmt.Errorf("migrate takes no arguments")
	}
	if *from == vault.Name() {
		return fmt.Errorf("tfc-vault already uses the %s keychain", *from)
	}

	source, err := vault.OpenNamed(*from)
	if err != nil {
		return err
	}
	names, err := source.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return fmt.Errorf("the %s keychain holds no tfc-vault profiles", *from)
	}

	destination, err := vault.Open(vault.WithPassphrasePrompt(func(string) (string, error) {
		return readNewPassphrase(vault.Name())
	}))
	if err != nil {
		return err
	}

	for _, name := range names {
		cred, err := source.Get(name)
		if err != nil {
			return err
		}
		if err := destination.Put(name, cred); err != nil {
			return err
		}
		if *remove {
			if err := source.Delete(name); err != nil {
				return err
			}
		}
		fmt.Fprintf(stdout, "Moved profile %q from %s to %s.\n", name, *from, vault.Name())
	}
	if !*remove {
		warn("the profiles are still in the %s keychain; re-run with -remove to delete them there", *from)
	}
	return nil
}
