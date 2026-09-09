// Command tfc-vault stores Terraform API tokens in the macOS keychain and
// releases them to Terraform one command at a time.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iwashitahga/tfc-vault/internal/cli"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "tfc-vault: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	// Terraform invokes a credentials helper under the name
	// terraform-credentials-<name>, passing the protocol verb as the first
	// argument. Detect that and skip the normal command dispatch.
	if strings.HasPrefix(filepath.Base(os.Args[0]), "terraform-credentials-") {
		return cli.Helper(os.Args[1:])
	}
	return cli.Run(os.Args[1:])
}
