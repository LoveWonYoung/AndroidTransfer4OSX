package main

import (
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
)

const (
	defaultRemoteDir = "/sdcard/Download"

	prefKeyADBPath        = "adb_path"
	prefKeyRemoteDir      = "remote_dir"
	prefKeyPullRemotePath = "pull_remote_path"
	prefKeyPullLocalDir   = "pull_local_dir"
	prefKeyRecentDirs     = "recent_remote_dirs"
	prefKeyWindowWidth    = "window_width"
	prefKeyWindowHeight   = "window_height"
	maxRecentRemoteDirs   = 8
	minSavedWindowWidth   = 900
	minSavedWindowHeight  = 600
	defaultWindowWidth    = 1600
	defaultWindowHeight   = 900
)

func defaultPullLocalDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "."
	}
	return filepath.Join(home, "Downloads")
}

func appPreferences() fyne.Preferences {
	app := fyne.CurrentApp()
	if app == nil {
		return nil
	}
	return app.Preferences()
}

func savedWindowSize() fyne.Size {
	fallback := fyne.NewSize(defaultWindowWidth, defaultWindowHeight)
	prefs := appPreferences()
	if prefs == nil {
		return fallback
	}

	width := prefs.FloatWithFallback(prefKeyWindowWidth, float64(fallback.Width))
	height := prefs.FloatWithFallback(prefKeyWindowHeight, float64(fallback.Height))
	if width < minSavedWindowWidth || height < minSavedWindowHeight {
		return fallback
	}
	return fyne.NewSize(float32(width), float32(height))
}

func (ui *transferUI) savedPreferenceString(key, fallback string) string {
	prefs := appPreferences()
	if prefs == nil {
		return fallback
	}
	return prefs.StringWithFallback(key, fallback)
}

func (ui *transferUI) savePreferenceString(key, value string) {
	prefs := appPreferences()
	if prefs == nil {
		return
	}

	value = strings.TrimSpace(value)
	if value == "" {
		prefs.RemoveValue(key)
		return
	}
	prefs.SetString(key, value)
}

func (ui *transferUI) saveRecentRemoteDir(value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}

	prefs := appPreferences()
	if prefs == nil {
		return
	}

	next := []string{value}
	for _, existing := range prefs.StringList(prefKeyRecentDirs) {
		existing = strings.TrimSpace(existing)
		if existing == "" || existing == value {
			continue
		}
		next = append(next, existing)
		if len(next) >= maxRecentRemoteDirs {
			break
		}
	}
	prefs.SetStringList(prefKeyRecentDirs, next)
}

func (ui *transferUI) recentRemoteDirs() []string {
	prefs := appPreferences()
	if prefs == nil {
		return nil
	}
	return prefs.StringList(prefKeyRecentDirs)
}

func mergeDirOptions(options, recent []string) []string {
	seen := make(map[string]struct{}, len(options)+len(recent))
	merged := make([]string, 0, len(options)+len(recent))
	for _, group := range [][]string{options, recent} {
		for _, value := range group {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			merged = append(merged, value)
		}
	}
	return merged
}

func (ui *transferUI) saveWindowState() {
	prefs := appPreferences()
	if prefs == nil || ui.window == nil || ui.window.Canvas() == nil {
		return
	}

	size := ui.window.Canvas().Size()
	if size.Width < minSavedWindowWidth || size.Height < minSavedWindowHeight {
		return
	}
	prefs.SetFloat(prefKeyWindowWidth, float64(size.Width))
	prefs.SetFloat(prefKeyWindowHeight, float64(size.Height))
}
