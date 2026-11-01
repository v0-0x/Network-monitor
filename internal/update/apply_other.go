//go:build !linux && !darwin && !windows

package update

import (
	"context"
	"errors"
)

func cantInstall() string { return "Updates can't be installed on this system." }

func applyUpdate(ctx context.Context, file, dir string) (applied, error) {
	return nil, errors.New(cantInstall())
}

// CleanupOld does nothing here.
func CleanupOld() {}
