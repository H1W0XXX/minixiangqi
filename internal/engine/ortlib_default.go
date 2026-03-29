//go:build !darwin

package engine

import (
	"fmt"
	"path/filepath"
)

func resolveORTSharedLibraryPath(libPath string) (string, error) {
	if libPath == "" {
		return "", fmt.Errorf("empty onnxruntime shared library path")
	}
	absLibPath, err := filepath.Abs(libPath)
	if err != nil {
		return "", err
	}
	return absLibPath, nil
}

func configureORTSearchPath(libDir string) {}
