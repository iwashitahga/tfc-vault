// Package cli implements the tfc-vault command set.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
)

// stdout is where command results are written. Diagnostics go to stderr via
// warn. Tests replace it to capture output.
var stdout io.Writer = os.Stdout

const usage = `tfc-vault keeps Terraform API tokens in the macOS keychain instead of
~/.terraform.d/credentials.tfrc.json, and hands them to Terraform only for the
lifetime of a single command.

Usage:
  tfc-vault <command> [flags] [arguments]

Commands:
  add [host]              Store a token for a host (default app.terraform.io)
  list                    List stored profiles
  remove <profile>        Delete a profile from the keychain
  exec <profile> -- cmd   Run cmd with the token in its environment
  env <profile>           Print shell export lines for the token
  get <profile>           Print the raw token on stdout
  import [path]           Import an existing credentials.tfrc.json
  migrate                 Copy profiles in from another keychain
  lock                    Lock the keychain now
  unlock                  Unlock the keychain from the terminal (ssh only)
  install                 Register tfc-vault as a Terraform credentials helper
  helper <verb> <host>    Terraform credentials helper protocol (internal)

Run 'tfc-vault <command> -h' for the flags of a single command.
`

// Run dispatches a tfc-vault invocation. args excludes the program name.
func Run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("no command given")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "add", "login":
		return runAdd(rest)
	case "list", "ls":
		return runList(rest)
	case "remove", "rm":
		return runRemove(rest)
	case "exec":
		return runExec(rest)
	case "env":
		return runEnv(rest)
	case "get":
		return runGet(rest)
	case "import":
		return runImport(rest)
	case "migrate":
		return runMigrate(rest)
	case "lock":
		return runLock(rest)
	case "unlock":
		return runUnlock(rest)
	case "install":
		return runInstall(rest)
	case "helper":
		return Helper(rest)
	case "version":
		fmt.Println(Version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// Version is set at build time with -ldflags.
var Version = "dev"

func newFlagSet(name, oneLine string) *flag.FlagSet {
	fs := flag.NewFlagSet("tfc-vault "+name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s\n\nUsage: tfc-vault %s\n\nFlags:\n", oneLine, name)
		fs.PrintDefaults()
	}
	return fs
}
