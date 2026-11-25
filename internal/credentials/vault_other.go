//go:build !linux && !windows

package credentials

// NewVault returns the available operating-system credential store, or nil.
func NewVault() Vault { return nil }
