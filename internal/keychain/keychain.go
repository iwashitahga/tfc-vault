//go:build darwin && cgo

// Package keychain covers the few macOS keychain operations the keyring
// library does not expose: asking whether a keychain is locked, and setting
// how long it stays unlocked.
package keychain

/*
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>
#include <stdlib.h>

// The SecKeychain family is deprecated in favour of the data-protection
// keychain, which has no per-file locking. A separate lockable keychain is the
// whole point here, so the old API is the only one that will do.
#pragma clang diagnostic push
#pragma clang diagnostic ignored "-Wdeprecated-declarations"

static OSStatus tfcKeychainStatus(const char *path, SecKeychainStatus *status) {
	SecKeychainRef kc = NULL;
	OSStatus err = SecKeychainOpen(path, &kc);
	if (err != errSecSuccess) {
		return err;
	}
	err = SecKeychainGetStatus(kc, status);
	CFRelease(kc);
	return err;
}

static OSStatus tfcKeychainSetSettings(const char *path, int lockOnSleep, unsigned int interval) {
	SecKeychainRef kc = NULL;
	OSStatus err = SecKeychainOpen(path, &kc);
	if (err != errSecSuccess) {
		return err;
	}
	SecKeychainSettings settings;
	settings.version = SEC_KEYCHAIN_SETTINGS_VERS1;
	settings.lockOnSleep = lockOnSleep ? TRUE : FALSE;
	settings.useLockInterval = interval > 0 ? TRUE : FALSE;
	settings.lockInterval = interval;
	err = SecKeychainSetSettings(kc, &settings);
	CFRelease(kc);
	return err;
}

static OSStatus tfcKeychainUnlock(const char *path, const char *password, unsigned int length) {
	SecKeychainRef kc = NULL;
	OSStatus err = SecKeychainOpen(path, &kc);
	if (err != errSecSuccess) {
		return err;
	}
	err = SecKeychainUnlock(kc, length, password, TRUE);
	CFRelease(kc);
	return err;
}

static OSStatus tfcKeychainLock(const char *path) {
	SecKeychainRef kc = NULL;
	OSStatus err = SecKeychainOpen(path, &kc);
	if (err != errSecSuccess) {
		return err;
	}
	err = SecKeychainLock(kc);
	CFRelease(kc);
	return err;
}

#pragma clang diagnostic pop
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// unlockStateStatus is kSecUnlockStateStatus: the bit that is set while the
// keychain is unlocked.
const unlockStateStatus = 1

// IsLocked reports whether the named keychain is currently locked. The name may
// be a bare keychain file name, which macOS resolves under ~/Library/Keychains.
func IsLocked(name string) (bool, error) {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	var status C.SecKeychainStatus
	if err := check(C.tfcKeychainStatus(cName, &status)); err != nil {
		return false, err
	}
	return uint32(status)&unlockStateStatus == 0, nil
}

// SetSettings configures when the keychain relocks. A zero interval leaves it
// unlocked until the machine sleeps or it is locked explicitly.
func SetSettings(name string, lockOnSleep bool, intervalSeconds uint32) error {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	var onSleep C.int
	if lockOnSleep {
		onSleep = 1
	}
	return check(C.tfcKeychainSetSettings(cName, onSleep, C.uint(intervalSeconds)))
}

// Unlock unlocks the keychain with the given passphrase. It stays unlocked
// until the interval set by SetSettings elapses, the machine sleeps, or Lock is
// called.
func Unlock(name, passphrase string) error {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	cPass := C.CString(passphrase)
	defer C.free(unsafe.Pointer(cPass))

	return check(C.tfcKeychainUnlock(cName, cPass, C.uint(len(passphrase))))
}

// Lock locks the keychain immediately.
func Lock(name string) error {
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))
	return check(C.tfcKeychainLock(cName))
}

func check(status C.OSStatus) error {
	if status == 0 {
		return nil
	}
	return fmt.Errorf("keychain operation failed (OSStatus %d)", int(status))
}
