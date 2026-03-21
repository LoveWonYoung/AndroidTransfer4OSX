package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const fileDialogWidth = 1000
const fileDialogHeight = 720

const (
	refreshDevicesTimeout = 8 * time.Second
	scanStorageTimeout    = 5 * time.Second
	adbEnsureDirTimeout   = 8 * time.Second
	adbPushTimeout        = 30 * time.Minute
	maxLogLines           = 1000
	maxQueueItems         = 500
)

type queueItem struct {
	LocalPath string
	IsDir     bool
	Status    string
}

type uiState struct {
	mu             sync.Mutex
	busy           bool
	deviceMap      map[string]string
	taskCancel     context.CancelFunc
	logLines       []string
	queue          []queueItem
	queueHeights   map[int]float32
	queueSelected  int
	storageScanSeq int
}

type transferUI struct {
	window fyne.Window
	state  *uiState

	adbPathEntry    *widget.Entry
	deviceSelect    *widget.Select
	dirPresetSelect *widget.Select
	localPathEntry  *widget.Entry
	remoteDirEntry  *widget.Entry
	statusLabel     *widget.Label
	logBox          *widget.Entry
	queueList       *widget.List
	progress        *widget.ProgressBarInfinite

	pushBtn        *widget.Button
	refreshBtn     *widget.Button
	scanStorageBtn *widget.Button
	addPathBtn     *widget.Button
	removeItemBtn  *widget.Button
	clearQueueBtn  *widget.Button
	cancelBtn      *widget.Button
	clearLogBtn    *widget.Button
}

func newTransferUI(w fyne.Window) *transferUI {
	ui := &transferUI{
		window: w,
		state: &uiState{
			deviceMap:     make(map[string]string),
			queueHeights:  make(map[int]float32),
			queueSelected: -1,
		},
	}

	ui.initWidgets()
	ui.bindEvents()
	return ui
}

func (ui *transferUI) initWidgets() {
	ui.adbPathEntry = widget.NewEntry()
	ui.adbPathEntry.SetPlaceHolder("ADB 可执行文件或 platform-tools 目录")
	ui.adbPathEntry.SetText(defaultADBInput)

	ui.deviceSelect = widget.NewSelect(nil, nil)
	ui.deviceSelect.PlaceHolder = "点击刷新设备列表"

	ui.dirPresetSelect = widget.NewSelect(nil, nil)
	ui.dirPresetSelect.PlaceHolder = "识别设备后可选择内置存储 / TF 卡目录"

	ui.localPathEntry = widget.NewEntry()
	ui.localPathEntry.SetPlaceHolder("选择或输入要传输的文件/目录路径")

	ui.remoteDirEntry = widget.NewEntry()
	ui.remoteDirEntry.SetPlaceHolder("/sdcard/Download")
	ui.remoteDirEntry.SetText("/sdcard/Download")

	ui.statusLabel = widget.NewLabel("就绪")

	ui.queueList = widget.NewList(
		func() int {
			ui.state.mu.Lock()
			defer ui.state.mu.Unlock()
			return len(ui.state.queue)
		},
		func() fyne.CanvasObject {
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapBreak
			return label
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			label, _ := obj.(*widget.Label)
			if label == nil {
				return
			}
			ui.state.mu.Lock()
			if id < 0 || id >= len(ui.state.queue) {
				ui.state.mu.Unlock()
				label.SetText("")
				return
			}
			item := ui.state.queue[id]
			ui.state.mu.Unlock()

			text := formatQueueItemText(item)
			label.SetText(text)
			ui.adjustQueueItemHeight(id, label.Size().Width, text)
		},
	)
	ui.queueList.OnSelected = func(id widget.ListItemID) {
		ui.state.mu.Lock()
		ui.state.queueSelected = int(id)
		ui.state.mu.Unlock()
		ui.updateQueueButtons()
	}
	ui.queueList.OnUnselected = func(widget.ListItemID) {
		ui.state.mu.Lock()
		ui.state.queueSelected = -1
		ui.state.mu.Unlock()
		ui.updateQueueButtons()
	}

	ui.logBox = widget.NewMultiLineEntry()
	ui.logBox.Wrapping = fyne.TextWrapWord
	ui.logBox.Disable()
	ui.logBox.SetMinRowsVisible(14)

	ui.progress = widget.NewProgressBarInfinite()
	ui.progress.Hide()

	ui.pushBtn = widget.NewButton("开始传输", ui.StartPush)
	ui.pushBtn.Disable()

	ui.refreshBtn = widget.NewButton("刷新设备", ui.RefreshDevices)
	ui.scanStorageBtn = widget.NewButton("识别存储", func() {
		ui.refreshRemoteDirPresets(true)
	})
	ui.scanStorageBtn.Disable()
	ui.addPathBtn = widget.NewButton("加入队列", ui.AddCurrentPathToQueue)
	ui.addPathBtn.Disable()
	ui.removeItemBtn = widget.NewButton("移除选中", ui.RemoveSelectedQueueItem)
	ui.removeItemBtn.Disable()
	ui.clearQueueBtn = widget.NewButton("清空队列", ui.ClearQueue)
	ui.clearQueueBtn.Disable()
	ui.cancelBtn = widget.NewButton("取消任务", ui.CancelCurrentTask)
	ui.cancelBtn.Disable()

	ui.clearLogBtn = widget.NewButton("清空日志", ui.clearLog)
}

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

func (ui *transferUI) bindEvents() {
	ui.deviceSelect.OnChanged = func(string) {
		ui.updatePushEnabled()
		ui.refreshRemoteDirPresets(false)
	}
	ui.dirPresetSelect.OnChanged = func(value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		ui.remoteDirEntry.SetText(value)
	}
	ui.localPathEntry.OnChanged = func(string) {
		ui.updatePushEnabled()
		ui.updateQueueButtons()
	}
	ui.remoteDirEntry.OnChanged = func(string) {
		ui.updatePushEnabled()
	}
	ui.window.SetOnDropped(func(_ fyne.Position, items []fyne.URI) {
		ui.handleWindowDrop(items)
	})
}

func (ui *transferUI) Content() fyne.CanvasObject {
	chooseFileBtn := widget.NewButton("添加文件", ui.pickLocalFile)
	chooseDirBtn := widget.NewButton("添加目录", ui.pickLocalDir)

	form := widget.NewForm(
		widget.NewFormItem("ADB 路径", ui.adbPathEntry),
		widget.NewFormItem("设备", container.NewBorder(nil, nil, nil, ui.refreshBtn, ui.deviceSelect)),
		widget.NewFormItem("常用目录", container.NewBorder(nil, nil, nil, ui.scanStorageBtn, ui.dirPresetSelect)),
		widget.NewFormItem("本地路径", ui.localPathEntry),
		widget.NewFormItem("安卓目录", ui.remoteDirEntry),
	)

	queuePanel := container.NewBorder(
		widget.NewLabel("待传输队列（支持多文件 / 多目录，按顺序执行，也支持拖拽到窗口）"),
		nil,
		nil,
		nil,
		ui.queueList,
	)
	logPanel := container.NewBorder(
		widget.NewLabel("日志"),
		nil,
		nil,
		nil,
		container.NewVScroll(ui.logBox),
	)
	leftPanel := container.NewVScroll(container.NewVBox(
		widget.NewLabel("控制面板"),
		container.NewVBox(
			widget.NewLabel("使用 ADB 将文件或目录传输到 Android（USB 调试需开启）"),
			form,
			widget.NewSeparator(),
			widget.NewLabel("队列操作"),
			container.NewGridWithColumns(2, chooseFileBtn, chooseDirBtn),
			ui.addPathBtn,
			ui.removeItemBtn,
			ui.clearQueueBtn,
			widget.NewSeparator(),
			widget.NewLabel("任务操作"),
			ui.pushBtn,
			ui.cancelBtn,
			ui.clearLogBtn,
			container.NewHBox(widget.NewLabel("状态:"), ui.statusLabel),
			ui.progress,
		),
	))

	queueLogSplit := container.NewHSplit(queuePanel, logPanel)
	queueLogSplit.Offset = 0.45

	rootSplit := container.NewHSplit(leftPanel, queueLogSplit)
	rootSplit.Offset = 0.34
	return rootSplit
}

func (ui *transferUI) handleWindowDrop(items []fyne.URI) {
	if len(items) == 0 {
		return
	}

	paths := make([]string, 0, len(items))
	skipped := 0
	for _, item := range items {
		if item == nil {
			skipped++
			continue
		}
		path := strings.TrimSpace(item.Path())
		if path == "" {
			skipped++
			continue
		}
		paths = append(paths, path)
	}

	if len(paths) == 0 {
		ui.appendLog("拖拽失败：未识别到可用的本地文件/目录路径")
		ui.setStatus("未识别到拖拽路径")
		return
	}

	if skipped > 0 {
		ui.appendLog(fmt.Sprintf("拖拽项中有 %d 项无法识别，已跳过", skipped))
	}
	ui.appendLog(fmt.Sprintf("收到拖拽项 %d 个，正在加入队列...", len(paths)))

	go func(droppedPaths []string) {
		for i, path := range droppedPaths {
			if i == len(droppedPaths)-1 {
				lastPath := path
				fyne.Do(func() {
					ui.localPathEntry.SetText(lastPath)
				})
			}
			ui.addQueuePath(path)
		}
	}(append([]string(nil), paths...))
}

func (ui *transferUI) RefreshDevices() {
	if !ui.beginTask("有任务正在执行，暂不刷新设备列表") {
		return
	}
	adbInput := strings.TrimSpace(ui.adbPathEntry.Text)

	fyne.Do(func() {
		ui.deviceSelect.Disable()
		ui.pushBtn.Disable()
	})
	ui.setStatus("正在刷新设备列表...")

	go func() {
		hasUsableDevice := false
		defer func() {
			ui.endTask()
			fyne.Do(func() {
				ui.deviceSelect.Enable()
			})
			ui.updatePushEnabled()
			if hasUsableDevice {
				ui.refreshRemoteDirPresets(false)
			}
		}()

		adbExec, err := resolveADBPath(adbInput)
		if err != nil {
			ui.appendLog("ADB 路径错误: " + err.Error())
			ui.setStatus("ADB 路径无效")
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), refreshDevicesTimeout)
		defer cancel()
		ui.setTaskCancel(cancel)

		devices, raw, err := adbListDevices(ctx, adbExec)
		if raw != "" {
			ui.appendLog("$ " + adbExec + " devices -l")
			ui.appendLog(raw)
		}
		if err != nil {
			if errors.Is(err, context.Canceled) {
				ui.appendLog("已取消刷新设备")
				ui.setStatus("已取消刷新")
				return
			}
			if errors.Is(err, context.DeadlineExceeded) {
				ui.appendLog("刷新设备超时，请检查 adb 状态或 USB 连接")
				ui.setStatus("刷新设备超时")
				return
			}
			ui.appendLog("刷新设备失败: " + err.Error())
			ui.setStatus("刷新设备失败")
			return
		}

		nextMap := make(map[string]string)
		labels := make([]string, 0, len(devices))
		for _, d := range devices {
			if d.Status != "device" {
				ui.appendLog(fmt.Sprintf("设备 %s 当前状态: %s", d.Serial, d.Status))
				continue
			}

			label := d.Label()
			nextMap[label] = d.Serial
			labels = append(labels, label)
		}

		fyne.Do(func() {
			ui.state.mu.Lock()
			ui.state.deviceMap = nextMap
			ui.state.mu.Unlock()

			ui.deviceSelect.Options = labels
			ui.deviceSelect.Refresh()
			if len(labels) > 0 {
				hasUsableDevice = true
				ui.deviceSelect.SetSelected(labels[0])
				ui.setStatus(fmt.Sprintf("已发现 %d 台可用设备", len(labels)))
				return
			}
			ui.deviceSelect.ClearSelected()
			ui.setDirPresets(nil)
			ui.setStatus("未发现可用设备（请检查 USB 调试授权）")
		})
	}()
}

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
			ui.appendLog("$ " + adbExec + " -s " + serial + " push " + item.LocalPath + " " + remoteDir)

			pushCtx, pushCancel := context.WithTimeout(taskCtx, adbPushTimeout)
			startAt := time.Now()
			out, err := adbPush(pushCtx, adbExec, serial, item.LocalPath, remoteDir)
			elapsed := time.Since(startAt)
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

func (ui *transferUI) pickLocalFile() {
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if reader == nil {
			return
		}
		path := reader.URI().Path()
		_ = reader.Close()
		ui.localPathEntry.SetText(path)
		ui.addQueuePath(path)
	}, ui.window)
	fd.Resize(fyne.NewSize(fileDialogWidth, fileDialogHeight))
	fd.Show()
}

func (ui *transferUI) pickLocalDir() {
	fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, ui.window)
			return
		}
		if uri == nil {
			return
		}
		path := uri.Path()
		ui.localPathEntry.SetText(path)
		ui.addQueuePath(path)
	}, ui.window)
	fd.Resize(fyne.NewSize(fileDialogWidth, fileDialogHeight))
	fd.Show()
}

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
		ui.scanStorageBtn.Disable()
		ui.addPathBtn.Disable()
		ui.removeItemBtn.Disable()
		ui.clearQueueBtn.Disable()
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
		if !busy && selected >= 0 && selected < queueCount {
			ui.removeItemBtn.Enable()
			return
		}
		ui.removeItemBtn.Disable()
	})
}

func (ui *transferUI) refreshRemoteDirPresets(manual bool) {
	adbInput := strings.TrimSpace(ui.adbPathEntry.Text)
	deviceLabel := ui.deviceSelect.Selected

	ui.state.mu.Lock()
	busy := ui.state.busy
	serial := ui.state.deviceMap[deviceLabel]
	ui.state.storageScanSeq++
	seq := ui.state.storageScanSeq
	ui.state.mu.Unlock()

	if busy {
		if manual {
			ui.appendLog("有任务正在执行，暂不识别存储目录")
		}
		return
	}
	if serial == "" {
		if manual {
			ui.appendLog("请先选择设备，再识别存储目录")
			ui.setStatus("请先选择设备")
		}
		ui.setDirPresets(nil)
		return
	}

	go func(scanSeq int, adbInput, serial string, manual bool) {
		adbExec, err := resolveADBPath(adbInput)
		if err != nil {
			if manual {
				ui.appendLog("ADB 路径错误: " + err.Error())
				ui.setStatus("ADB 路径无效")
			}
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), scanStorageTimeout)
		defer cancel()

		paths, raw, err := adbListStorageSuggestions(ctx, adbExec, serial)
		if err != nil {
			if manual {
				ui.appendLog("$ " + adbExec + " -s " + serial + " shell ls /storage")
				if strings.TrimSpace(raw) != "" {
					ui.appendLog(raw)
				}
				if errors.Is(err, context.DeadlineExceeded) {
					ui.appendLog("识别存储目录超时")
					ui.setStatus("识别存储目录超时")
				} else {
					ui.appendLog("识别存储目录失败: " + err.Error())
					ui.setStatus("识别存储目录失败")
				}
			}
			return
		}

		ui.state.mu.Lock()
		stale := scanSeq != ui.state.storageScanSeq
		ui.state.mu.Unlock()
		if stale {
			return
		}

		ui.setDirPresets(paths)
		if manual {
			ui.appendLog(fmt.Sprintf("已识别 %d 个常用目录（含内置存储与 TF 卡）", len(paths)))
			ui.setStatus("存储目录识别完成")
		}
	}(seq, adbInput, serial, manual)
}

func (ui *transferUI) setDirPresets(options []string) {
	fyne.Do(func() {
		currentRemote := strings.TrimSpace(ui.remoteDirEntry.Text)
		ui.dirPresetSelect.Options = options
		ui.dirPresetSelect.Refresh()

		if len(options) == 0 {
			ui.dirPresetSelect.ClearSelected()
			return
		}

		for _, opt := range options {
			if opt == currentRemote {
				ui.dirPresetSelect.SetSelected(opt)
				return
			}
		}

		// Keep existing manual input unchanged; only select a sensible default in the dropdown.
		ui.dirPresetSelect.SetSelected(options[0])
		if currentRemote == "" {
			ui.remoteDirEntry.SetText(options[0])
		}
	})
}
