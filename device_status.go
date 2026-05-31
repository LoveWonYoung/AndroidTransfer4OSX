package main

func deviceStatusHint(status string) string {
	switch status {
	case "unauthorized":
		return "未授权：请解锁手机并确认 USB 调试授权弹窗"
	case "offline":
		return "离线：请重新插拔 USB，或尝试重启 ADB server"
	case "no permissions":
		return "无权限：请检查系统 USB 权限或 ADB 驱动"
	case "device":
		return "可用"
	default:
		if status == "" {
			return "状态未知：请重新刷新设备"
		}
		return "不可用：" + status
	}
}
