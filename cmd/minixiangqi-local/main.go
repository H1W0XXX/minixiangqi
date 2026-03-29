package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	httpserver "minixiangqi/internal/server/http"
)

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func resolveExistingDir(dir string) string {
	if dir == "" {
		return dir
	}
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		if abs, e := filepath.Abs(dir); e == nil {
			return abs
		}
		return dir
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	exe, err := os.Executable()
	if err != nil {
		return dir
	}
	exeDir := filepath.Dir(exe)
	candidates := []string{
		filepath.Join(exeDir, dir),
		filepath.Join(exeDir, filepath.Base(dir)),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c
		}
	}
	return dir
}

func main() {
	addr := flag.String("addr", "0.0.0.0:2888", "listen address")
	webDir := flag.String("web", "./web", "directory with static web files")
	webMobileDir := flag.String("web-mobile", "./web_mobile", "directory with mobile web files")
	backend := flag.String("backend", "onnx", "search backend: onnx|nnue|none")
	modelPath := flag.String("model", "minixiangqi.onnx", "path to ONNX model file")
	libPath := flag.String("lib", "onnxruntime.dll", "path to onnxruntime shared library")
	enableNN := flag.Bool("enable-nn", true, "enable selected neural backend at startup")
	nnuePath := flag.String("nnue", "minixiangqi.nnue", "path to NNUE file")
	nnueSource := flag.String("nnue-source", "third_lib\\variant-nnue-pytorch", "path to local NNUE training source/workdir")
	flag.Parse()

	mux := http.NewServeMux()
	*webDir = resolveExistingDir(*webDir)
	*webMobileDir = resolveExistingDir(*webMobileDir)

	h := httpserver.NewHandler()
	if *enableNN {
		switch strings.ToLower(strings.TrimSpace(*backend)) {
		case "onnx":
			if *modelPath != "" {
				log.Printf("initializing ONNX model %s", *modelPath)
				if err := h.Engine().InitNN(*modelPath, *libPath); err != nil {
					log.Fatalf("failed to initialize ONNX: %v", err)
				}
			}
		case "nnue":
			if err := h.Engine().InitNNUE(*nnuePath, *nnueSource); err != nil {
				log.Fatalf("failed to initialize NNUE: %v", err)
			}
			log.Printf("NNUE enabled, model=%s source=%s", *nnuePath, *nnueSource)
		case "none":
			log.Printf("neural backend disabled")
		default:
			log.Fatalf("unknown backend: %s", *backend)
		}
	} else {
		log.Printf("NN disabled at startup")
	}
	h.ConfigureModels(*modelPath, *libPath, *nnuePath, *nnueSource)

	mux.Handle("/api/", h)
	httpserver.RegisterStaticRoutes(mux, *webDir, *webMobileDir)

	log.Printf("listening on %s, backend=%s", *addr, h.Engine().BackendName())
	go func() {
		time.Sleep(250 * time.Millisecond)
		url := "http://127.0.0.1:2888"
		if strings.Contains(*addr, ":") {
			parts := strings.Split(*addr, ":")
			url = "http://127.0.0.1:" + parts[len(parts)-1]
		}
		openBrowser(url)
	}()

	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}
