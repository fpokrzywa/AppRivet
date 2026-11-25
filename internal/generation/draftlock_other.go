//go:build !darwin && !linux && !windows

package generation

import (
	"errors"
	"os"
)

func lockDraft(*os.File) error {
	return errors.New("draft locking requires macOS, Linux or Windows")
}
