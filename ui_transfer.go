package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
)

func (ui *transferUI) StartPush() {
	if !ui.beginTask("有任务正在执行，请稍候") {
		return
	}

	adbInput := strings.TrimSpace(ui.adbPathEntry.Text)
	deviceLabel := ui.deviceSelect.Selected
	remoteDir := strings.TrimSpace(ui.remoteDirEntry.Text)

	ui.state.mu.Lock()
	serial := ui.state.deviceMap[deviceLabel]
	queueSnapshot := append([]queueItem(nil), ui.state.queue...)
	ui.state.mu.Unlock()

	fyne.Do(func() {
		ui.pushBtn.Disable()
	})
	ui.setStatus("正在传输...")

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
		if remoteDir == "" {
			ui.appendLog("目标目录不能为空")
			ui.setStatus("请输入安卓目标目录")
			return
		}
		if len(queueSnapshot) == 0 {
			ui.appendLog("待传输队列为空")
			ui.setStatus("请先添加文件或目录到队列")
			return
		}

		adbExec, err := resolveADBPath(adbInput)
		if err != nil {
			ui.appendLog("ADB 路径错误: " + err.Error())
			ui.setStatus("ADB 路径无效")
			return
		}

		ui.appendLog(fmt.Sprintf("目标设备: %s", serial))
		ui.appendLog(fmt.Sprintf("安卓目录: %s", remoteDir))
		ui.appendLog(fmt.Sprintf("队列条目: %d", len(queueSnapshot)))
		ui.resetQueueStatuses("待传输")

		taskCtx, taskCancel := context.WithCancel(context.Background())
		defer taskCancel()
		ui.setTaskCancel(taskCancel)

		mkdirCtx, mkdirCancel := context.WithTimeout(taskCtx, adbEnsureDirTimeout)
		defer mkdirCancel()
		if out, err := adbEnsureDir(mkdirCtx, adbExec, serial, remoteDir); err != nil {
			ui.appendLog("$ " + adbExec + " -s " + serial + " shell mkdir -p " + remoteDir)
			if strings.TrimSpace(out) != "" {
				ui.appendLog(out)
			}
			if errors.Is(err, context.Canceled) {
				ui.appendLog("已取消传输")
				ui.setStatus("已取消传输")
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				ui.appendLog("创建安卓目录超时")
				ui.setStatus("创建目标目录超时")
				return
			}
			ui.appendLog("创建安卓目录失败: " + err.Error())
			ui.setStatus("创建目标目录失败")
			return
		}

		successCount := 0
		failCount := 0
		var totalTransferredBytes int64
		var totalTransferDuration time.Duration
		for i, item := range queueSnapshot {
			ui.updateQueueItemStatus(i, "校验中")
			info, err := os.Stat(item.LocalPath)
			if err != nil {
				failCount++
				ui.updateQueueItemStatus(i, "失败")
				ui.appendLog(fmt.Sprintf("队列项 %d 跳过，本地路径不可用: %s (%v)", i+1, item.LocalPath, err))
				continue
			}

			itemKind := "文件"
			if info.IsDir() {
				itemKind = "目录"
			}
			itemBytes, sizeErr := estimateTransferBytes(item.LocalPath, info)
			if sizeErr != nil {
				ui.appendLog(fmt.Sprintf("队列项 %d 无法统计大小，将仅显示耗时: %v", i+1, sizeErr))
			}
			sizeText := "未知大小"
			if sizeErr == nil {
				sizeText = formatDataSize(itemBytes)
			}

			ui.updateQueueItemStatus(i, "传输中")
			ui.setStatus(fmt.Sprintf("正在传输 (%d/%d): %s (%s)", i+1, len(queueSnapshot), filepath.Base(item.LocalPath), sizeText))
			ui.appendLog(fmt.Sprintf("开始传输 [%d/%d] %s: %s (大小: %s)", i+1, len(queueSnapshot), itemKind, item.LocalPath, sizeText))
			ui.appendLog("$ " + adbExec + " -s " + serial + " push -p " + item.LocalPath + " " + remoteDir)

			startAt := time.Now()
			progressDone := make(chan struct{})
			progressStateMu := sync.Mutex{}
			progressPercent := -1
			progressSpeed := ""
			progressStepLogged := -1

			go func(itemName string, itemIndex int) {
				ticker := time.NewTicker(1 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-progressDone:
						return
					case <-ticker.C:
						progressStateMu.Lock()
						pct := progressPercent
						speed := progressSpeed
						progressStateMu.Unlock()

						status := fmt.Sprintf("正在传输 (%d/%d): %s, 已用时 %s", itemIndex+1, len(queueSnapshot), itemName, formatDuration(time.Since(startAt)))
						if pct >= 0 {
							status += fmt.Sprintf(", 进度 %d%%", pct)
						}
						if speed != "" {
							status += fmt.Sprintf(", 速度 %s", speed)
						}
						ui.setStatus(status)
					}
				}
			}(filepath.Base(item.LocalPath), i)

			pushCtx, pushCancel := context.WithTimeout(taskCtx, adbPushTimeout)
			out, err := adbPushWithProgress(pushCtx, adbExec, serial, item.LocalPath, remoteDir, func(progress adbPushProgressInfo) {
				progressStateMu.Lock()
				if progress.Percent >= 0 {
					progressPercent = progress.Percent
				}
				if progress.Speed != "" {
					progressSpeed = progress.Speed
				}
				pct := progressPercent
				speed := progressSpeed

				step := -1
				shouldRefreshQueueStatus := false
				if pct >= 0 {
					step = pct / 10
					if step > progressStepLogged {
						progressStepLogged = step
						shouldRefreshQueueStatus = true
					}
				}
				progressStateMu.Unlock()

				status := fmt.Sprintf("正在传输 (%d/%d): %s", i+1, len(queueSnapshot), filepath.Base(item.LocalPath))
				if pct >= 0 {
					status += fmt.Sprintf(", 进度 %d%%", pct)
				}
				if speed != "" {
					status += fmt.Sprintf(", 速度 %s", speed)
				}
				ui.setStatus(status)
				if shouldRefreshQueueStatus && pct >= 0 {
					ui.updateQueueItemStatus(i, fmt.Sprintf("传输中 %d%%", pct))
					ui.appendLog(fmt.Sprintf("队列项 %d 进度: %d%%", i+1, pct))
				}
			})
			elapsed := time.Since(startAt)
			close(progressDone)
			pushCancel()
			if strings.TrimSpace(out) != "" {
				ui.appendLog(out)
			}
			if err != nil {
				if errors.Is(err, context.Canceled) {
					ui.updateQueueItemStatus(i, "已取消")
					ui.appendLog("已取消传输")
					ui.setStatus("已取消传输")
					return
				}
				if errors.Is(err, context.DeadlineExceeded) {
					failCount++
					ui.updateQueueItemStatus(i, "超时")
					ui.appendLog(fmt.Sprintf("队列项 %d 传输超时: %s", i+1, item.LocalPath))
					continue
				}
				failCount++
				ui.updateQueueItemStatus(i, "失败")
				ui.appendLog(fmt.Sprintf("队列项 %d 传输失败: %v", i+1, err))
				continue
			}

			successCount++
			speedText := "未知"
			durationText := formatDuration(elapsed)
			dataText := sizeText
			statusText := "成功"

			if adbSpeed, ok := parseADBPushSpeed(out); ok && adbSpeed.Bytes > 0 && adbSpeed.Duration > 0 {
				speedText = adbSpeed.RawSpeed
				durationText = formatDuration(adbSpeed.Duration)
				dataText = formatDataSize(adbSpeed.Bytes)
				totalTransferredBytes += adbSpeed.Bytes
				totalTransferDuration += adbSpeed.Duration
				statusText = "成功 " + adbSpeed.RawSpeed
			} else if itemBytes > 0 && elapsed > 0 {
				speedText = formatTransferSpeed(itemBytes, elapsed)
				totalTransferredBytes += itemBytes
				totalTransferDuration += elapsed
				statusText = "成功 " + speedText
			}

			ui.updateQueueItemStatus(i, statusText)
			ui.appendLog(fmt.Sprintf("队列项 %d 传输完成: 速度 %s, 数据量 %s, 用时 %s", i+1, speedText, dataText, durationText))
			ui.setStatus(fmt.Sprintf("已完成 (%d/%d): %s, 速度 %s", i+1, len(queueSnapshot), filepath.Base(item.LocalPath), speedText))
		}

		finalStatus := fmt.Sprintf("传输完成：成功 %d，失败 %d", successCount, failCount)
		if totalTransferredBytes > 0 && totalTransferDuration > 0 {
			overallSpeed := formatTransferSpeed(totalTransferredBytes, totalTransferDuration)
			totalText := formatDataSize(totalTransferredBytes)
			finalStatus = fmt.Sprintf("传输完成：成功 %d，失败 %d，总数据量 %s，平均速度 %s", successCount, failCount, totalText, overallSpeed)
			ui.appendLog(finalStatus)
		} else {
			ui.appendLog(finalStatus)
		}
		ui.setStatus(finalStatus)
	}()
}
