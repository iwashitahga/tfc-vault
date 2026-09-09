//go:build !cgo

package vault

// Reaching the macOS keychain needs cgo. Built with CGO_ENABLED=0 the keyring
// library still compiles, but every operation fails at run time with an
// unhelpful "backend not available", so refuse at build time instead.
func init() {
	tfc_vault_requires_CGO_ENABLED_1()
}
