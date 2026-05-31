package main

import "testing"

func TestDeviceStatusHint(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{status: "device", want: "可用"},
		{status: "unauthorized", want: "未授权：请解锁手机并确认 USB 调试授权弹窗"},
		{status: "offline", want: "离线：请重新插拔 USB，或尝试重启 ADB server"},
		{status: "", want: "状态未知：请重新刷新设备"},
		{status: "recovery", want: "不可用：recovery"},
	}

	for _, tt := range tests {
		if got := deviceStatusHint(tt.status); got != tt.want {
			t.Fatalf("deviceStatusHint(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}
