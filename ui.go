package main

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

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
