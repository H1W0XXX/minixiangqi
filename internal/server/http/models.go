package httpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"minixiangqi/internal/engine"
)

type ModelOption struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Backend string `json:"backend"`
}

type ModelsResponse struct {
	CurrentKey string        `json:"current_key"`
	Models     []ModelOption `json:"models"`
}

type modelCatalog struct {
	mu         sync.RWMutex
	currentKey string
	libPath    string
	nnueSource string
	options    []ModelOption
	cache      map[string]*engine.Engine
}

func newModelCatalog() *modelCatalog {
	return &modelCatalog{
		cache: make(map[string]*engine.Engine),
	}
}

func (c *modelCatalog) Configure(currentBackend, modelPath, libPath, nnuePath, nnueSource string, current *engine.Engine) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.libPath = libPath
	c.nnueSource = nnueSource
	c.refreshLocked(modelPath, nnuePath)
	c.currentKey = c.resolveCurrentKeyLocked(currentBackend, modelPath, nnuePath)
	if current != nil && c.currentKey != "" {
		c.cache[c.currentKey] = current
	}
}

func (c *modelCatalog) Snapshot() ModelsResponse {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.refreshLocked("", "")
	if c.currentKey == "" && len(c.options) > 0 {
		c.currentKey = c.options[0].Key
	}
	out := make([]ModelOption, len(c.options))
	copy(out, c.options)
	return ModelsResponse{
		CurrentKey: c.currentKey,
		Models:     out,
	}
}

func (c *modelCatalog) CloneEngineForKey(key string) (*engine.Engine, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.refreshLocked("", "")
	if key == "" {
		key = c.currentKey
	}
	if key == "" {
		return engine.NewEngine(), "", nil
	}
	for _, opt := range c.options {
		if opt.Key != key {
			continue
		}
		if cached := c.cache[key]; cached != nil {
			return cached.CloneForGame(), opt.Label, nil
		}
		created := engine.NewEngine()
		switch opt.Backend {
		case string(engine.BackendONNX):
			modelPath := strings.TrimPrefix(key, "onnx|")
			if err := created.InitNN(modelPath, c.libPath); err != nil {
				return nil, "", err
			}
		case string(engine.BackendNNUE):
			modelPath := strings.TrimPrefix(key, "nnue|")
			if err := created.InitNNUE(modelPath, c.nnueSource); err != nil {
				return nil, "", err
			}
		case string(engine.BackendNone):
		default:
			return nil, "", fmt.Errorf("unsupported backend: %s", opt.Backend)
		}
		c.cache[key] = created
		return created.CloneForGame(), opt.Label, nil
	}
	return nil, "", fmt.Errorf("model not found")
}

func (c *modelCatalog) refreshLocked(preferredONNX, preferredNNUE string) {
	type seenModel struct {
		key string
		opt ModelOption
	}
	seen := make(map[string]seenModel)

	add := func(backend, path string) {
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if st, err := os.Stat(abs); err != nil || st.IsDir() {
			return
		}
		key := backend + "|" + abs
		if _, ok := seen[key]; ok {
			return
		}
		label := filepath.Base(abs)
		if backend == string(engine.BackendNNUE) {
			label += " [NNUE]"
		} else {
			label += " [KataGo ONNX]"
		}
		seen[key] = seenModel{
			key: key,
			opt: ModelOption{
				Key:     key,
				Label:   label,
				Backend: backend,
			},
		}
	}

	if preferredONNX != "" {
		add(string(engine.BackendONNX), preferredONNX)
	}
	if preferredNNUE != "" {
		add(string(engine.BackendNNUE), preferredNNUE)
	}

	for _, dir := range scanModelDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := strings.ToLower(entry.Name())
			full := filepath.Join(dir, entry.Name())
			switch {
			case strings.HasSuffix(name, ".onnx"):
				add(string(engine.BackendONNX), full)
			case strings.HasSuffix(name, ".nnue"):
				add(string(engine.BackendNNUE), full)
			}
		}
	}

	options := []ModelOption{
		{Key: "none", Label: "不使用模型", Backend: string(engine.BackendNone)},
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(seen[keys[i]].opt.Label) < strings.ToLower(seen[keys[j]].opt.Label)
	})
	for _, key := range keys {
		options = append(options, seen[key].opt)
	}
	c.options = options
}

func (c *modelCatalog) resolveCurrentKeyLocked(currentBackend, modelPath, nnuePath string) string {
	switch currentBackend {
	case string(engine.BackendONNX):
		if abs, err := filepath.Abs(modelPath); err == nil && abs != "" {
			return "onnx|" + abs
		}
	case string(engine.BackendNNUE):
		if abs, err := filepath.Abs(nnuePath); err == nil && abs != "" {
			return "nnue|" + abs
		}
	}
	return "none"
}

func scanModelDirs() []string {
	seen := make(map[string]struct{})
	dirs := make([]string, 0, 2)
	add := func(dir string) {
		if dir == "" {
			return
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return
		}
		if _, ok := seen[abs]; ok {
			return
		}
		if st, err := os.Stat(abs); err == nil && st.IsDir() {
			seen[abs] = struct{}{}
			dirs = append(dirs, abs)
		}
	}

	if wd, err := os.Getwd(); err == nil {
		add(wd)
	}
	if exe, err := os.Executable(); err == nil {
		add(filepath.Dir(exe))
	}
	return dirs
}
