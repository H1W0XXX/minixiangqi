//go:build !windows

package engine

import "os"

func setNativeEnv(key, value string) {
	_ = os.Setenv(key, value)
}
