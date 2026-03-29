//go:build darwin

package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const darwinSharedLibraryName = "libonnxruntime.dylib"

func resolveORTSharedLibraryPath(libPath string) (string, error) {
	candidates := make([]string, 0, 3)

	// Prefer project/local dylib when building/running from source.
	candidates = append(candidates, darwinSharedLibraryName)

	// Fall back to dylib next to executable.
	if exe, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), darwinSharedLibraryName))
	}

	// If caller explicitly provides a path, try it as well.
	if libPath != "" && libPath != "onnxruntime.dll" {
		candidates = append(candidates, libPath)
	}

	checked := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, p := range candidates {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		checked = append(checked, abs)
		info, err := os.Stat(abs)
		if err == nil && !info.IsDir() {
			return abs, nil
		}
	}

	return "", fmt.Errorf("cannot find %s, checked: %s", darwinSharedLibraryName, strings.Join(checked, ", "))
}

func configureORTSearchPath(libDir string) {
	prependPathEnv("DYLD_LIBRARY_PATH", libDir)
}
