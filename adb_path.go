package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	darwinPlatformToolsDir  = "platform_tools_darwin"
	windowsPlatformToolsDir = "platform_tools_windows"
)

func defaultADBInput() string {
	if path, ok := bundledADBPath(runtime.GOOS); ok {
		return path
	}
	if path, err := exec.LookPath(adbExecutableName()); err == nil {
		return path
	}
	if dir := platformToolsDirForGOOS(runtime.GOOS); dir != "" {
		return filepath.Join(dir, adbExecutableName())
	}
	return adbExecutableName()
}

func adbExecutableName() string {
	return adbExecutableNameForGOOS(runtime.GOOS)
}

func adbExecutableNameForGOOS(goos string) string {
	if goos == "windows" {
		return "adb.exe"
	}
	return "adb"
}

func platformToolsDirForGOOS(goos string) string {
	switch goos {
	case "darwin":
		return darwinPlatformToolsDir
	case "windows":
		return windowsPlatformToolsDir
	default:
		return ""
	}
}

func bundledADBPath(goos string) (string, bool) {
	dir := platformToolsDirForGOOS(goos)
	if dir == "" {
		return "", false
	}
	adbName := adbExecutableNameForGOOS(goos)
	candidates := []string{
		filepath.Join(dir, adbName),
	}

	if exePath, err := os.Executable(); err == nil {
		candidates = append([]string{
			filepath.Join(filepath.Dir(exePath), dir, adbName),
		}, candidates...)
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}
