package main

import (
	"context"
	"strings"

	"fyne.io/fyne/v2"
)

func (ui *transferUI) appendLog(msg string) {
	normalized := strings.TrimRight(msg, "\n")
	if strings.TrimSpace(normalized) == "" {
		return
	}

	newLines := strings.Split(normalized, "\n")
	ui.state.mu.Lock()
	ui.state.logLines = append(ui.state.logLines, newLines...)
	if len(ui.state.logLines) > maxLogLines {
		ui.state.logLines = ui.state.logLines[len(ui.state.logLines)-maxLogLines:]
	}
	snapshot := append([]string(nil), ui.state.logLines...)
	ui.state.mu.Unlock()

	text := strings.Join(snapshot, "\n") + "\n"
	cursorRow := len(snapshot)
	fyne.Do(func() {
		ui.logBox.SetText(text)
		ui.logBox.CursorRow = cursorRow
	})
}

func (ui *transferUI) setStatus(msg string) {
	fyne.Do(func() {
		ui.statusLabel.SetText(msg)
	})
}

func (ui *transferUI) updatePushEnabled() {
	fyne.Do(func() {
		ui.state.mu.Lock()
		busy := ui.state.busy
		selectedSerial := ""
		queueCount := len(ui.state.queue)
		if ui.deviceSelect.Selected != "" {
			selectedSerial = ui.state.deviceMap[ui.deviceSelect.Selected]
		}
		ui.state.mu.Unlock()

		hasRemote := strings.TrimSpace(ui.remoteDirEntry.Text) != ""
		if !busy && selectedSerial != "" && queueCount > 0 && hasRemote {
			ui.pushBtn.Enable()
		} else {
			ui.pushBtn.Disable()
		}

		hasPullRemote := strings.TrimSpace(ui.pullRemoteEntry.Text) != ""
		hasPullLocal := strings.TrimSpace(ui.pullLocalEntry.Text) != ""
		if !busy && selectedSerial != "" && hasPullRemote && hasPullLocal {
			ui.pullBtn.Enable()
		} else {
			ui.pullBtn.Disable()
		}

		if !busy && selectedSerial != "" {
			ui.scanStorageBtn.Enable()
			return
		}
		ui.scanStorageBtn.Disable()
	})
}

func (ui *transferUI) beginTask(busyMsg string) bool {
	ui.state.mu.Lock()
	if ui.state.busy {
		ui.state.mu.Unlock()
		if strings.TrimSpace(busyMsg) != "" {
			ui.appendLog(busyMsg)
		}
		return false
	}
	ui.state.busy = true
	ui.state.taskCancel = nil
	ui.state.mu.Unlock()

	fyne.Do(func() {
		ui.progress.Show()
		ui.progress.Start()
		ui.pushBtn.Disable()
		ui.pullBtn.Disable()
		ui.scanStorageBtn.Disable()
		ui.addPathBtn.Disable()
		ui.removeItemBtn.Disable()
		ui.clearQueueBtn.Disable()
		ui.retryFailedBtn.Disable()
		ui.cancelBtn.Disable()
	})
	return true
}

func (ui *transferUI) endTask() {
	ui.state.mu.Lock()
	ui.state.busy = false
	ui.state.taskCancel = nil
	ui.state.mu.Unlock()

	fyne.Do(func() {
		ui.progress.Stop()
		ui.progress.Hide()
		ui.scanStorageBtn.Disable()
		ui.cancelBtn.Disable()
	})
	ui.updateQueueButtons()
}

func (ui *transferUI) setTaskCancel(cancel context.CancelFunc) {
	ui.state.mu.Lock()
	if !ui.state.busy {
		ui.state.mu.Unlock()
		return
	}
	ui.state.taskCancel = cancel
	ui.state.mu.Unlock()

	fyne.Do(func() {
		if cancel != nil {
			ui.cancelBtn.Enable()
			return
		}
		ui.cancelBtn.Disable()
	})
}

func (ui *transferUI) CancelCurrentTask() {
	ui.state.mu.Lock()
	busy := ui.state.busy
	cancel := ui.state.taskCancel
	ui.state.mu.Unlock()

	if !busy || cancel == nil {
		return
	}

	ui.appendLog("已请求取消当前任务...")
	ui.setStatus("正在取消...")
	fyne.Do(func() {
		ui.cancelBtn.Disable()
	})
	cancel()
}

func (ui *transferUI) clearLog() {
	ui.state.mu.Lock()
	ui.state.logLines = nil
	ui.state.mu.Unlock()

	fyne.Do(func() {
		ui.logBox.SetText("")
		ui.logBox.CursorRow = 0
	})
}
