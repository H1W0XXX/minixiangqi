package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveModelPath(modelPath string) (string, error) {
	if modelPath == "" {
		return "", fmt.Errorf("empty model path")
	}

	candidates := make([]string, 0, 3)
	candidates = append(candidates, modelPath)

	if !filepath.IsAbs(modelPath) {
		if exe, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exe)
			candidates = append(candidates, filepath.Join(exeDir, modelPath))
			candidates = append(candidates, filepath.Join(exeDir, filepath.Base(modelPath)))
		}
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

	return "", fmt.Errorf("model file not found, checked: %s", strings.Join(checked, ", "))
}
