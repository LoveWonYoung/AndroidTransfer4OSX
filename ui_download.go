package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (ui *transferUI) StartPull() {
	if !ui.beginTask("有任务正在执行，请稍候") {
		return
	}

	adbInput := strings.TrimSpace(ui.adbPathEntry.Text)
	deviceLabel := ui.deviceSelect.Selected
	remotePath := strings.TrimSpace(ui.pullRemoteEntry.Text)
	localDir := strings.TrimSpace(ui.pullLocalEntry.Text)

	ui.state.mu.Lock()
	serial := ui.state.deviceMap[deviceLabel]
	ui.state.mu.Unlock()

	ui.setStatus("正在下载...")

	go func() {
		defer func() {
			ui.endTask()
			ui.updatePushEnabled()
		}()

		if serial == "" {
			ui.appendLog("未选择可用设备")
			ui.setStatus("请选择设备")
			return
		}
		if remotePath == "" {
			ui.appendLog("安卓路径不能为空")
			ui.setStatus("请输入安卓路径")
			return
		}
		if localDir == "" {
			ui.appendLog("电脑保存目录不能为空")
			ui.setStatus("请选择电脑保存目录")
			return
		}

		localInfo, err := os.Stat(localDir)
		if err != nil {
			ui.appendLog("电脑保存目录不可用: " + err.Error())
			ui.setStatus("保存目录无效")
			return
		}
		if !localInfo.IsDir() {
			ui.appendLog("电脑保存路径不是目录: " + localDir)
			ui.setStatus("保存路径不是目录")
			return
		}

		adbExec, err := resolveADBPath(adbInput)
		if err != nil {
			ui.appendLog("ADB 路径错误: " + err.Error())
			ui.setStatus("ADB 路径无效")
			return
		}

		ui.savePreferenceString(prefKeyPullRemotePath, remotePath)
		ui.savePreferenceString(prefKeyPullLocalDir, localDir)
		ui.appendLog(fmt.Sprintf("目标设备: %s", serial))
		ui.appendLog(fmt.Sprintf("安卓路径: %s", remotePath))
		ui.appendLog(fmt.Sprintf("保存到电脑: %s", localDir))
		ui.appendLog("$ " + adbExec + " -s " + serial + " pull " + remotePath + " " + localDir)

		taskCtx, taskCancel := context.WithCancel(context.Background())
		defer taskCancel()
		ui.setTaskCancel(taskCancel)

		startAt := time.Now()
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()
			itemName := filepath.Base(remotePath)
			if itemName == "." || itemName == string(filepath.Separator) || strings.TrimSpace(itemName) == "" {
				itemName = remotePath
			}
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					ui.setStatus(fmt.Sprintf("正在下载: %s, 已用时 %s", itemName, formatDuration(time.Since(startAt))))
				}
			}
		}()

		pullCtx, pullCancel := context.WithTimeout(taskCtx, adbPullTimeout)
		out, err := adbPull(pullCtx, adbExec, serial, remotePath, localDir)
		elapsed := time.Since(startAt)
		close(done)
		pullCancel()

		if strings.TrimSpace(out) != "" {
			ui.appendLog(out)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				ui.appendLog("已取消下载")
				ui.setStatus("已取消下载")
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				ui.appendLog("下载超时: " + remotePath)
				ui.setStatus("下载超时")
				return
			}
			ui.appendLog("下载失败: " + err.Error())
			ui.setStatus("下载失败")
			return
		}

		speedText := "未知"
		dataText := "未知"
		durationText := formatDuration(elapsed)
		if adbSpeed, ok := parseADBPushSpeed(out); ok && adbSpeed.Bytes > 0 && adbSpeed.Duration > 0 {
			speedText = adbSpeed.RawSpeed
			dataText = formatDataSize(adbSpeed.Bytes)
			durationText = formatDuration(adbSpeed.Duration)
		}

		finalStatus := fmt.Sprintf("下载完成：数据量 %s，速度 %s，用时 %s", dataText, speedText, durationText)
		ui.appendLog(finalStatus)
		ui.setStatus(finalStatus)
	}()
}
