// Package credentials stores AppRivet provider keys in the current user's OS vault.
package credentials

import (
	"errors"
	"regexp"
	"strings"
)

// Vault keeps each provider's key in a separate credential. Get returns an
// empty key and no error when the credential does not exist.
type Vault interface {
	Get(provider string) (string, error)
	Save(provider, key string) error
	Remove(provider string) error
}

const maxKeyBytes = 512

const (
	AppStoreConnect = "app-store-connect"
	GooglePlay      = "google-play"
)

var (
	keyPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]{8,512}$`)
	errProvider = errors.New("unsupported API key provider")
	errKey      = errors.New("invalid provider API key")
)

func ValidProvider(provider string) bool {
	return provider == "openai" || provider == "anthropic"
}

func ValidKey(key string) bool {
	return keyPattern.MatchString(key)
}

func validSlot(slot string) bool {
	return ValidProvider(slot) || slot == AppStoreConnect || slot == GooglePlay
}

func slotSize(slot string) int {
	if slot == AppStoreConnect || slot == GooglePlay {
		return 2560 // CRED_MAX_CREDENTIAL_BLOB_SIZE on Windows.
	}
	return maxKeyBytes
}

func validStoredValue(slot, value string) bool {
	if ValidProvider(slot) {
		return ValidKey(value)
	}
	return validSlot(slot) && value != "" && len(value) <= slotSize(slot) && !strings.ContainsAny(value, "\x00\r\n")
}
