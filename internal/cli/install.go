package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// helperName is the file name Terraform looks for when the CLI config names
// "tfc-vault" as its credentials helper.
const helperName = "terraform-credentials-tfc-vault"

const helperBlock = `credentials_helper "tfc-vault" {
  args = []
}
`

func runInstall(args []string) error {
	fs := newFlagSet("install", "Register tfc-vault as Terraform's credentials helper.")
	uninstall := fs.Bool("uninstall", false, "Remove the helper link instead")
	migrate := fs.Bool("migrate", false, "Import and delete an existing credentials.tfrc.json without asking")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		fs.Usage()
		return fmt.Errorf("install takes no arguments")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	pluginDir := filepath.Join(home, ".terraform.d", "plugins")
	linkPath := filepath.Join(pluginDir, helperName)

	if *uninstall {
		if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		fmt.Fprintf(stdout, "Removed %s.\n", linkPath)
		fmt.Fprintf(stdout, "Delete the credentials_helper block from %s by hand to finish.\n", cliConfigPath(home))
		return nil
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}
	if err := linkHelper(exe, linkPath); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Linked %s -> %s\n", linkPath, exe)

	if err := writeCLIConfig(cliConfigPath(home)); err != nil {
		return err
	}
	return migratePlaintext(*migrate)
}

// linkHelper points linkPath at exe. The symlink is built under a temporary
// name and renamed into place so the helper is never briefly missing, which
// would otherwise leave a window for another process to claim the path.
func linkHelper(exe, linkPath string) error {
	tmp := linkPath + ".tmp"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(exe, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, linkPath); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func cliConfigPath(home string) string {
	if p := os.Getenv("TF_CLI_CONFIG_FILE"); p != "" {
		return p
	}
	return filepath.Join(home, ".terraformrc")
}

func writeCLIConfig(path string) error {
	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(helperBlock), 0o600); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "Wrote %s.\n", path)
		return nil
	}
	if err != nil {
		return err
	}

	if strings.Contains(string(existing), "credentials_helper") {
		if strings.Contains(string(existing), `credentials_helper "tfc-vault"`) {
			fmt.Fprintf(stdout, "%s already selects tfc-vault.\n", path)
			return nil
		}
		warn("%s already declares a different credentials_helper; leaving it alone", path)
		warn("replace that block with:\n\n%s", helperBlock)
		return nil
	}

	if err := os.WriteFile(path+".bak", existing, 0o600); err != nil {
		return err
	}
	body := string(existing)
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	body += "\n" + helperBlock
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Appended the helper block to %s (previous contents saved as %s.bak).\n", path, path)
	return nil
}

// migratePlaintext deals with a leftover credentials.tfrc.json. Terraform reads
// that file before it consults a credentials helper, and does not call the
// helper at all for a host the file already covers, so the helper stays inert
// until the file is gone.
func migratePlaintext(force bool) error {
	path, err := DefaultCredentialsFile()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		fmt.Fprintln(stdout, "Done. Terraform will now ask tfc-vault for tokens.")
		return nil
	}

	warn("")
	warn("%s still exists. Terraform reads it before the helper, so the", path)
	warn("helper stays unused until those tokens move into the keychain.")

	if !force && !confirm("Import them and delete the file?") {
		warn("Run 'tfc-vault import -purge' when you are ready.")
		return nil
	}
	if err := importCredentials(path, true); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "Done. Terraform will now ask tfc-vault for tokens.")
	return nil
}
