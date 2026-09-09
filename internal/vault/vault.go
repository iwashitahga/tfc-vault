// Package vault stores Terraform API tokens in the macOS keychain, keyed by
// profile name. Nothing is ever written to disk in plaintext.
package vault

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/99designs/keyring"
	"github.com/iwashitahga/tfc-vault/internal/keychain"
)

// ServiceName is the keychain service every tfc-vault item is filed under.
const ServiceName = "tfc-vault"

// DefaultKeychain is the dedicated keychain tokens live in. Keeping them out of
// the login keychain is what makes locking meaningful: the login keychain stays
// unlocked for your whole session, and anything running as you can read it.
const DefaultKeychain = "tfc-vault"

// LoginKeychain selects the login keychain instead, for anyone who would rather
// not manage a second passphrase.
const LoginKeychain = "login"

// DefaultLockInterval is how long the keychain stays unlocked after it is used.
const DefaultLockInterval = 15 * 60

// ErrNotFound is returned when no profile with the requested name exists.
var ErrNotFound = errors.New("no such profile")

// ErrLocked is returned when the keychain has to be unlocked before the request
// can be answered.
var ErrLocked = errors.New("keychain is locked")

// Name returns the keychain tfc-vault will use. TFC_VAULT_KEYCHAIN overrides it;
// set it to "login" for the login keychain.
func Name() string {
	if name := os.Getenv("TFC_VAULT_KEYCHAIN"); name != "" {
		return name
	}
	return DefaultKeychain
}

// Credential is one Terraform API token together with the service host it
// authenticates against.
type Credential struct {
	Host  string `json:"host"`
	Token string `json:"token"`
}

// Store is a handle on a keyring holding tfc-vault credentials.
type Store struct {
	ring keyring.Keyring
	// keychain is empty for a store that is not backed by a dedicated,
	// lockable keychain, which is the case for the login keychain and in tests.
	keychain string
}

// NewStore wraps an already-open keyring. Tests use it to supply an in-memory
// ring in place of the real keychain.
func NewStore(ring keyring.Keyring) *Store {
	return &Store{ring: ring}
}

type options struct {
	passphrase func(prompt string) (string, error)
}

// Option adjusts how the keychain is opened.
type Option func(*options)

// WithPassphrasePrompt supplies the passphrase used when the dedicated keychain
// has to be created. Without it, creation fails rather than falling back to a
// window the caller did not ask for.
func WithPassphrasePrompt(fn func(prompt string) (string, error)) Option {
	return func(o *options) { o.passphrase = fn }
}

// Open connects to the keychain tfc-vault stores tokens in. Reading from a
// locked keychain makes macOS ask for its passphrase.
func Open(opts ...Option) (*Store, error) {
	return OpenNamed(Name(), opts...)
}

// OpenNamed opens a specific keychain by name, so one store can be migrated
// into another.
func OpenNamed(name string, opts ...Option) (*Store, error) {
	var o options
	for _, apply := range opts {
		apply(&o)
	}

	cfg := keyring.Config{
		ServiceName:                    ServiceName,
		AllowedBackends:                []keyring.BackendType{keyring.KeychainBackend},
		KeychainTrustApplication:       true,
		KeychainSynchronizable:         false,
		KeychainAccessibleWhenUnlocked: true,
	}

	store := &Store{}
	if name != LoginKeychain {
		cfg.KeychainName = name
		store.keychain = name
		if o.passphrase != nil {
			cfg.KeychainPasswordFunc = o.passphrase
		} else {
			cfg.KeychainPasswordFunc = func(string) (string, error) {
				return "", fmt.Errorf("the %s keychain does not exist yet; create it with 'tfc-vault add'", name)
			}
		}
	}

	ring, err := keyring.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("opening keychain: %w", err)
	}
	store.ring = ring
	return store, nil
}

// file is the keychain name macOS resolves under ~/Library/Keychains.
func (s *Store) file() string {
	return s.keychain + ".keychain"
}

// Exists reports whether the dedicated keychain has been created yet.
func (s *Store) Exists() bool {
	if s.keychain == "" {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	for _, suffix := range []string{".keychain-db", ".keychain"} {
		if _, err := os.Stat(filepath.Join(home, "Library", "Keychains", s.keychain+suffix)); err == nil {
			return true
		}
	}
	return false
}

// Locked reports whether the request would need the keychain unlocked first.
func (s *Store) Locked() bool {
	if s.keychain == "" || !s.Exists() {
		return false
	}
	locked, err := keychain.IsLocked(s.file())
	return err == nil && locked
}

// Unlock unlocks the keychain with the given passphrase.
func (s *Store) Unlock(passphrase string) error {
	if s.keychain == "" {
		return fmt.Errorf("the login keychain is not managed by tfc-vault")
	}
	if !s.Exists() {
		return fmt.Errorf("the %s keychain does not exist yet", s.keychain)
	}
	return keychain.Unlock(s.file(), passphrase)
}

// KeychainName reports which keychain this store is backed by.
func (s *Store) KeychainName() string {
	if s.keychain == "" {
		return LoginKeychain
	}
	return s.keychain
}

// Lock relocks the keychain, so the next read asks for the passphrase again.
func (s *Store) Lock() error {
	if s.keychain == "" {
		return fmt.Errorf("the login keychain is not locked by tfc-vault")
	}
	if !s.Exists() {
		return fmt.Errorf("the %s keychain does not exist yet", s.keychain)
	}
	return keychain.Lock(s.file())
}

// applyLockSettings makes a freshly created keychain relock on sleep and after
// an idle interval, instead of staying open for the rest of the session.
func (s *Store) applyLockSettings() error {
	if s.keychain == "" {
		return nil
	}
	return keychain.SetSettings(s.file(), true, DefaultLockInterval)
}

// Put writes cred under the given profile name, replacing any existing entry.
func (s *Store) Put(profile string, cred Credential) error {
	data, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("encoding credential for profile %s: %w", profile, err)
	}
	// A dedicated keychain is created lazily by the first write, and only then
	// can its relock behaviour be set.
	fresh := !s.Exists()

	err = s.ring.Set(keyring.Item{
		Key:         profile,
		Data:        data,
		Label:       fmt.Sprintf("tfc-vault (%s)", profile),
		Description: "Terraform API token",
	})
	if err != nil {
		return fmt.Errorf("storing profile %s in the keychain: %w", profile, err)
	}
	if fresh {
		if err := s.applyLockSettings(); err != nil {
			return fmt.Errorf("setting the auto-lock policy on the %s keychain: %w", s.keychain, err)
		}
	}
	return nil
}

// Get reads the credential stored under profile.
func (s *Store) Get(profile string) (Credential, error) {
	if err := s.requireKeychain(); err != nil {
		return Credential{}, err
	}
	item, err := s.ring.Get(profile)
	if errors.Is(err, keyring.ErrKeyNotFound) {
		// A locked keychain answers a query with no results, which would
		// otherwise be indistinguishable from having no such profile.
		if s.Locked() {
			return Credential{}, s.lockedError()
		}
		return Credential{}, fmt.Errorf("%w: %s", ErrNotFound, profile)
	}
	if err != nil {
		return Credential{}, fmt.Errorf("reading profile %s from the keychain: %w", profile, err)
	}
	var cred Credential
	if err := json.Unmarshal(item.Data, &cred); err != nil {
		return Credential{}, fmt.Errorf("profile %s holds malformed data: %w", profile, err)
	}
	if cred.Host == "" {
		cred.Host = profile
	}
	return cred, nil
}

// Delete removes a profile. Backends disagree on whether removing an absent
// key is an error, so the name is checked against the key list first.
func (s *Store) Delete(profile string) error {
	names, err := s.List()
	if err != nil {
		return err
	}
	if !slices.Contains(names, profile) {
		return fmt.Errorf("%w: %s", ErrNotFound, profile)
	}
	if err := s.ring.Remove(profile); err != nil {
		if errors.Is(err, keyring.ErrKeyNotFound) {
			return fmt.Errorf("%w: %s", ErrNotFound, profile)
		}
		return fmt.Errorf("removing profile %s from the keychain: %w", profile, err)
	}
	return nil
}

// List returns every profile name, sorted. It reads no secrets, only keys.
func (s *Store) List() ([]string, error) {
	if err := s.requireKeychain(); err != nil {
		return nil, err
	}
	names, err := s.ring.Keys()
	if err != nil {
		return nil, fmt.Errorf("listing keychain profiles: %w", err)
	}
	if len(names) == 0 && s.Locked() {
		return nil, s.lockedError()
	}
	sort.Strings(names)
	return names, nil
}

// requireKeychain refuses to treat a keychain that was never created as an
// empty one, which would look exactly like having no profiles.
func (s *Store) requireKeychain() error {
	if s.keychain == "" || s.Exists() {
		return nil
	}
	return fmt.Errorf("the %s keychain does not exist yet: store a token with 'tfc-vault add', or move existing ones with 'tfc-vault migrate -from login'", s.keychain)
}

func (s *Store) lockedError() error {
	return fmt.Errorf("%w: unlock it with 'tfc-vault unlock', or answer the macOS passphrase prompt (%s)", ErrLocked, s.keychain)
}

// ResolveHost finds the profile holding a token for host. A profile named
// exactly after the host wins; otherwise the single profile pointing at that
// host is used. An ambiguous match is an error rather than a guess.
//
// Searching by host has to decrypt every profile. A profile that cannot be
// read is not silently skipped: if the search ends without a match, the first
// such failure is reported instead of a bare "not found", so a corrupt entry or
// a keychain error is distinguishable from a genuinely absent token.
func (s *Store) ResolveHost(host string) (string, Credential, error) {
	cred, err := s.Get(host)
	if err == nil {
		return host, cred, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return "", Credential{}, err
	}

	names, err := s.List()
	if err != nil {
		return "", Credential{}, err
	}

	var matches []string
	var match Credential
	var firstErr error
	for _, name := range names {
		cred, err := s.Get(name)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if cred.Host == host {
			matches = append(matches, name)
			match = cred
		}
	}

	switch {
	case len(matches) == 1:
		return matches[0], match, nil
	case len(matches) > 1:
		return "", Credential{}, fmt.Errorf("host %s has several profiles (%v); pick one with TFC_VAULT_PROFILE", host, matches)
	case firstErr != nil:
		return "", Credential{}, fmt.Errorf("searching for a profile for host %s: %w", host, firstErr)
	default:
		return "", Credential{}, fmt.Errorf("%w for host %s", ErrNotFound, host)
	}
}
