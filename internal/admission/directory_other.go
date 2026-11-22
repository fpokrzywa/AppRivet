//go:build !linux && !darwin

package admission

import "os"

// No portable directory-entry flush is claimed on Windows. File Sync still runs.
func syncStoreDirectory(*os.Root) error { return nil }
