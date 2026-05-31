package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

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
