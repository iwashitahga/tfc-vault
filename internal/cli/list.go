package cli

import (
	"fmt"
	"text/tabwriter"

	"github.com/iwashitahga/tfc-vault/internal/tfhost"
	"github.com/iwashitahga/tfc-vault/internal/vault"
)

func runList(args []string) error {
	fs := newFlagSet("list", "List the profiles held in the keychain.")
	verbose := fs.Bool("v", false, "Also show each profile's host and env var (reads the secrets)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return fmt.Errorf("list takes no arguments")
	}

	store, err := vault.Open()
	if err != nil {
		return err
	}
	names, err := store.List()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		fmt.Fprintln(stdout, "No profiles stored. Add one with: tfc-vault add")
		return nil
	}

	if !*verbose {
		for _, name := range names {
			fmt.Fprintln(stdout, name)
		}
		return nil
	}

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PROFILE\tHOST\tENV")
	for _, name := range names {
		cred, err := store.Get(name)
		if err != nil {
			fmt.Fprintf(tw, "%s\t(unreadable)\t\n", name)
			continue
		}
		env, err := tfhost.EnvName(cred.Host)
		if err != nil {
			env = "(none)"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", name, cred.Host, env)
	}
	return tw.Flush()
}
