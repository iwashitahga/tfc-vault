// Package tfhost normalises Terraform service hostnames and maps them onto the
// TF_TOKEN_* environment variable names Terraform reads credentials from.
package tfhost

import (
	"fmt"
	"strings"

	"golang.org/x/net/idna"
)

// Default is the hostname of Terraform Cloud (HCP Terraform).
const Default = "app.terraform.io"

// Normalize lower-cases host and converts any internationalised domain name to
// its punycode (ACE) form, matching how Terraform itself canonicalises service
// hostnames.
func Normalize(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("empty hostname")
	}
	if strings.ContainsAny(host, "/:") {
		return "", fmt.Errorf("%q is not a bare hostname: drop the scheme and any port", host)
	}
	ascii, err := idna.Lookup.ToASCII(strings.ToLower(host))
	if err != nil {
		return "", fmt.Errorf("invalid hostname %q: %w", host, err)
	}
	return ascii, nil
}

// EnvName returns the environment variable Terraform reads a bearer token for
// host from. Periods become single underscores and hyphens become double
// underscores, per the TF_TOKEN_ credential convention.
func EnvName(host string) (string, error) {
	ascii, err := Normalize(host)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("TF_TOKEN_")
	for _, r := range ascii {
		switch {
		case r == '.':
			b.WriteByte('_')
		case r == '-':
			b.WriteString("__")
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			return "", fmt.Errorf("hostname %q contains %q, which has no TF_TOKEN_ encoding", host, r)
		}
	}
	return b.String(), nil
}

// TokensURL is the page where a user creates an API token for host.
func TokensURL(host string) string {
	return "https://" + host + "/app/settings/tokens"
}
