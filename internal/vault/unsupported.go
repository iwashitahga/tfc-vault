//go:build !darwin

package vault

// tfc-vault has only a macOS keychain backend. Fail the build rather than ship
// a binary that cannot store anything.
func init() {
	tfc_vault_only_supports_macOS()
}
