package main

import "testing"

func TestADBExecutableNameForGOOS(t *testing.T) {
	tests := []struct {
		goos string
		want string
	}{
		{goos: "windows", want: "adb.exe"},
		{goos: "darwin", want: "adb"},
		{goos: "linux", want: "adb"},
	}

	for _, tt := range tests {
		if got := adbExecutableNameForGOOS(tt.goos); got != tt.want {
			t.Fatalf("adbExecutableNameForGOOS(%q) = %q, want %q", tt.goos, got, tt.want)
		}
	}
}

func TestPlatformToolsDirForGOOS(t *testing.T) {
	tests := []struct {
		goos string
		want string
	}{
		{goos: "darwin", want: darwinPlatformToolsDir},
		{goos: "windows", want: windowsPlatformToolsDir},
		{goos: "linux", want: ""},
	}

	for _, tt := range tests {
		if got := platformToolsDirForGOOS(tt.goos); got != tt.want {
			t.Fatalf("platformToolsDirForGOOS(%q) = %q, want %q", tt.goos, got, tt.want)
		}
	}
}
