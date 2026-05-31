package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

func estimateTransferBytes(path string, info os.FileInfo) (int64, error) {
	if info == nil {
		return 0, fmt.Errorf("文件信息为空: %s", path)
	}
	if !info.IsDir() {
		if info.Mode().IsRegular() {
			return info.Size(), nil
		}
		return 0, nil
	}

	var total int64
	err := filepath.WalkDir(path, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil || entry.IsDir() {
			return nil
		}

		entryInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if entryInfo.Mode().IsRegular() {
			total += entryInfo.Size()
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

func formatTransferSpeed(bytes int64, duration time.Duration) string {
	if bytes <= 0 || duration <= 0 {
		return "未知"
	}
	perSecond := float64(bytes) / duration.Seconds()
	return fmt.Sprintf("%s/s", formatFloatSize(perSecond))
}

func formatDataSize(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	return formatFloatSize(float64(bytes))
}

func formatFloatSize(bytes float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := bytes
	unitIndex := 0
	for value >= 1024 && unitIndex < len(units)-1 {
		value /= 1024
		unitIndex++
	}
	if unitIndex == 0 {
		return fmt.Sprintf("%.0f %s", value, units[unitIndex])
	}
	return fmt.Sprintf("%.2f %s", value, units[unitIndex])
}

func formatDuration(duration time.Duration) string {
	if duration <= 0 {
		return "0s"
	}
	if duration >= time.Second {
		return duration.Round(100 * time.Millisecond).String()
	}
	return duration.Round(10 * time.Millisecond).String()
}
