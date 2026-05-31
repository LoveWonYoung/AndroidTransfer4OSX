package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func formatQueueItemText(item queueItem) string {
	kind := "文件"
	if item.IsDir {
		kind = "目录"
	}
	base := filepath.Base(item.LocalPath)
	if base == "." || base == string(filepath.Separator) || strings.TrimSpace(base) == "" {
		base = item.LocalPath
	}
	return fmt.Sprintf("[%s][%s] %s (%s)", item.Status, kind, base, item.LocalPath)
}

func (ui *transferUI) adjustQueueItemHeight(id widget.ListItemID, itemWidth float32, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	if itemWidth <= 0 {
		itemWidth = ui.queueList.Size().Width
	}
	contentWidth := itemWidth - theme.Padding()*2
	if contentWidth <= 0 {
		return
	}

	height := estimateWrappedTextHeight(text, contentWidth)
	minHeight := fyne.MeasureText("Ag", theme.TextSize(), fyne.TextStyle{}).Height + theme.Padding()*2
	if height < minHeight {
		height = minHeight
	}

	heightChanged := false
	ui.state.mu.Lock()
	prev, ok := ui.state.queueHeights[int(id)]
	if !ok || math.Abs(float64(prev-height)) > 0.5 {
		ui.state.queueHeights[int(id)] = height
		heightChanged = true
	}
	ui.state.mu.Unlock()
	if heightChanged {
		ui.queueList.SetItemHeight(id, height)
	}
}

func estimateWrappedTextHeight(text string, width float32) float32 {
	if width <= 0 {
		return fyne.MeasureText("Ag", theme.TextSize(), fyne.TextStyle{}).Height + theme.Padding()*2
	}

	textSize := theme.TextSize()
	style := fyne.TextStyle{}
	lineHeight := fyne.MeasureText("Ag", textSize, style).Height
	if lineHeight <= 0 {
		lineHeight = 1
	}

	lines := float32(1)
	lineWidth := float32(0)
	runeWidthCache := make(map[rune]float32, 128)

	for _, r := range text {
		if r == '\n' {
			lines++
			lineWidth = 0
			continue
		}

		rw, ok := runeWidthCache[r]
		if !ok {
			rw = fyne.MeasureText(string(r), textSize, style).Width
			runeWidthCache[r] = rw
		}

		if lineWidth > 0 && lineWidth+rw > width {
			lines++
			lineWidth = rw
			continue
		}
		lineWidth += rw
	}

	return lines*lineHeight + theme.Padding()*2
}

func (ui *transferUI) resetQueueHeightsLocked() {
	ui.state.queueHeights = make(map[int]float32, len(ui.state.queue))
}

func (ui *transferUI) AddCurrentPathToQueue() {
	path := strings.TrimSpace(ui.localPathEntry.Text)
	if path == "" {
		ui.appendLog("本地路径为空，无法加入队列")
		ui.setStatus("请输入本地路径")
		return
	}
	ui.addQueuePath(path)
}

func (ui *transferUI) addQueuePath(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}

	ui.state.mu.Lock()
	busy := ui.state.busy
	ui.state.mu.Unlock()
	if busy {
		ui.appendLog("任务执行中，暂不允许修改队列")
		ui.setStatus("任务执行中")
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		ui.appendLog("本地路径不可用，无法加入队列: " + err.Error())
		ui.setStatus("本地路径无效")
		return
	}

	item := queueItem{
		LocalPath: path,
		IsDir:     info.IsDir(),
		Status:    "待传输",
	}

	ui.state.mu.Lock()
	if len(ui.state.queue) >= maxQueueItems {
		ui.state.mu.Unlock()
		ui.appendLog(fmt.Sprintf("队列已满（最多 %d 项）", maxQueueItems))
		ui.setStatus("队列已满")
		return
	}
	ui.state.queue = append(ui.state.queue, item)
	newIndex := len(ui.state.queue) - 1
	ui.state.queueSelected = newIndex
	ui.resetQueueHeightsLocked()
	ui.state.mu.Unlock()

	fyne.Do(func() {
		ui.queueList.Refresh()
		ui.queueList.Select(newIndex)
	})
	ui.updatePushEnabled()
	ui.updateQueueButtons()
	ui.appendLog("已加入队列: " + path)
}

func (ui *transferUI) resetQueueStatuses(status string) {
	ui.state.mu.Lock()
	for i := range ui.state.queue {
		ui.state.queue[i].Status = status
	}
	ui.resetQueueHeightsLocked()
	ui.state.mu.Unlock()

	fyne.Do(func() {
		ui.queueList.Refresh()
	})
}

func (ui *transferUI) RemoveSelectedQueueItem() {
	ui.state.mu.Lock()
	selected := ui.state.queueSelected
	if selected < 0 || selected >= len(ui.state.queue) {
		ui.state.mu.Unlock()
		return
	}
	removed := ui.state.queue[selected]
	ui.state.queue = append(ui.state.queue[:selected], ui.state.queue[selected+1:]...)
	if len(ui.state.queue) == 0 {
		ui.state.queueSelected = -1
	} else if selected >= len(ui.state.queue) {
		ui.state.queueSelected = len(ui.state.queue) - 1
	}
	ui.resetQueueHeightsLocked()
	nextSelected := ui.state.queueSelected
	ui.state.mu.Unlock()

	fyne.Do(func() {
		ui.queueList.Refresh()
		if nextSelected >= 0 {
			ui.queueList.Select(nextSelected)
			return
		}
		ui.queueList.UnselectAll()
	})
	ui.updatePushEnabled()
	ui.updateQueueButtons()
	ui.appendLog("已移除队列项: " + removed.LocalPath)
}

func (ui *transferUI) ClearQueue() {
	ui.state.mu.Lock()
	if len(ui.state.queue) == 0 {
		ui.state.mu.Unlock()
		return
	}
	ui.state.queue = nil
	ui.state.queueSelected = -1
	ui.resetQueueHeightsLocked()
	ui.state.mu.Unlock()

	fyne.Do(func() {
		ui.queueList.Refresh()
		ui.queueList.UnselectAll()
	})
	ui.updatePushEnabled()
	ui.updateQueueButtons()
	ui.appendLog("已清空待传输队列")
}

func (ui *transferUI) updateQueueItemStatus(index int, status string) {
	ui.state.mu.Lock()
	if index < 0 || index >= len(ui.state.queue) {
		ui.state.mu.Unlock()
		return
	}
	ui.state.queue[index].Status = status
	ui.resetQueueHeightsLocked()
	ui.state.mu.Unlock()

	fyne.Do(func() {
		ui.queueList.Refresh()
	})
}

func (ui *transferUI) updateQueueButtons() {
	fyne.Do(func() {
		ui.state.mu.Lock()
		busy := ui.state.busy
		queueCount := len(ui.state.queue)
		selected := ui.state.queueSelected
		retryableCount := countRetryableQueueItems(ui.state.queue)
		ui.state.mu.Unlock()

		hasPathInput := strings.TrimSpace(ui.localPathEntry.Text) != ""
		if !busy && hasPathInput {
			ui.addPathBtn.Enable()
		} else {
			ui.addPathBtn.Disable()
		}
		if !busy && queueCount > 0 {
			ui.clearQueueBtn.Enable()
		} else {
			ui.clearQueueBtn.Disable()
		}
		if !busy && retryableCount > 0 {
			ui.retryFailedBtn.Enable()
		} else {
			ui.retryFailedBtn.Disable()
		}
		if !busy && selected >= 0 && selected < queueCount {
			ui.removeItemBtn.Enable()
			return
		}
		ui.removeItemBtn.Disable()
	})
}

func countRetryableQueueItems(items []queueItem) int {
	count := 0
	for _, item := range items {
		if isRetryableQueueStatus(item.Status) {
			count++
		}
	}
	return count
}

func isRetryableQueueStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "失败", "超时", "已取消", "待重试":
		return true
	default:
		return false
	}
}
