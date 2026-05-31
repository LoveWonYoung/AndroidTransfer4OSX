package main

import (
	"reflect"
	"testing"
	"time"
)

func TestParseADBDevices(t *testing.T) {
	out := `List of devices attached
ABC123 device product:foo model:Pixel_8 device:bar
OFFLINE offline product:foo model:Old_Phone device:baz
badline
`

	got := parseADBDevices(out)
	want := []deviceInfo{
		{Serial: "ABC123", Status: "device", Model: "Pixel_8", Raw: "ABC123 device product:foo model:Pixel_8 device:bar"},
		{Serial: "OFFLINE", Status: "offline", Model: "Old_Phone", Raw: "OFFLINE offline product:foo model:Old_Phone device:baz"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseADBDevices() = %#v, want %#v", got, want)
	}
}

func TestParseStorageSuggestions(t *testing.T) {
	got := parseStorageSuggestions("self emulated 1234-5678 /storage/9999-AAAA 1234-5678")
	want := []string{
		"/sdcard/Download",
		"/storage/1234-5678/Download/mymac",
		"/storage/9999-AAAA/Download/mymac",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseStorageSuggestions() = %#v, want %#v", got, want)
	}
}

func TestParseADBPushSpeed(t *testing.T) {
	got, ok := parseADBPushSpeed("file: 12.5 MB/s (2048 bytes in 0.500s)")
	if !ok {
		t.Fatal("parseADBPushSpeed() did not match")
	}
	if got.RawSpeed != "12.5 MB/s" || got.Bytes != 2048 || got.Duration != 500*time.Millisecond {
		t.Fatalf("parseADBPushSpeed() = %#v", got)
	}
}

func TestParseADBPushProgressLine(t *testing.T) {
	got, ok := parseADBPushProgressLine("[ 75%] 4.2 MB/s")
	if !ok {
		t.Fatal("parseADBPushProgressLine() did not match")
	}
	if got.Percent != 75 || got.Speed != "4.2 MB/s" || got.RawLine != "[ 75%] 4.2 MB/s" {
		t.Fatalf("parseADBPushProgressLine() = %#v", got)
	}
}
