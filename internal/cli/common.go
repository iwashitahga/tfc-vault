package cli

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"
)

// readToken takes a token from the terminal without echoing it, or from stdin
// when the command is being piped to.
func readToken(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, prompt)
		raw, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(raw)), nil
	}
	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// controllingTTY returns the terminal the user is at, which is not stdin when
// the token is being piped in. Passphrases and confirmations are read there, so
// `... | tfc-vault add` can still ask a question.
func controllingTTY() (*os.File, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		if term.IsTerminal(int(os.Stdin.Fd())) {
			return os.Stdin, nil
		}
		return nil, fmt.Errorf("no terminal available to ask on")
	}
	return f, nil
}

// confirm asks a yes/no question. It answers no when stdin is not a terminal,
// so a scripted run never takes a destructive branch by accident.
func confirm(prompt string) bool {
	tty, err := controllingTTY()
	if err != nil {
		return false
	}
	if tty != os.Stdin {
		defer tty.Close()
	}
	fmt.Fprintf(os.Stderr, "%s [y/N]: ", prompt)
	line, err := bufio.NewReader(tty).ReadString('\n')
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// readNewPassphrase asks twice for the passphrase protecting a keychain that is
// about to be created. It insists on a terminal: a passphrase read from a pipe
// would end up in whatever produced it.
func readNewPassphrase(name string) (string, error) {
	if _, err := controllingTTY(); err != nil {
		return "", fmt.Errorf("the %s keychain does not exist yet and creating it needs a terminal to read its passphrase", name)
	}
	warn("Creating the %s keychain. Choose a passphrase to protect it.", name)
	warn("macOS will ask for this passphrase when Terraform needs a token.")

	first, err := readSecret(fmt.Sprintf("Passphrase for the %s keychain: ", name))
	if err != nil {
		return "", err
	}
	if first == "" {
		return "", fmt.Errorf("an empty passphrase would leave the keychain unprotected")
	}
	second, err := readSecret("Repeat the passphrase: ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", fmt.Errorf("the two passphrases differ")
	}
	return first, nil
}

// readSecret reads one line from the controlling terminal without echoing it.
func readSecret(prompt string) (string, error) {
	tty, err := controllingTTY()
	if err != nil {
		return "", err
	}
	if tty != os.Stdin {
		defer tty.Close()
	}
	fmt.Fprint(os.Stderr, prompt)
	raw, err := term.ReadPassword(int(tty.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// verifyToken asks the host whether the token is accepted. Only an explicit
// rejection fails: an endpoint that is missing or unreachable is not evidence
// that the token is bad.
func verifyToken(host, token string) error {
	req, err := http.NewRequest(http.MethodGet, "https://"+host+"/api/v2/account/details", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/vnd.api+json")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("could not reach %s: %w", host, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%s rejected the token (HTTP %d)", host, resp.StatusCode)
	}
	return nil
}

// openBrowser best-effort opens a URL on macOS.
func openBrowser(url string) {
	if err := exec.Command("open", url).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "open %s in your browser to create a token\n", url)
	}
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
