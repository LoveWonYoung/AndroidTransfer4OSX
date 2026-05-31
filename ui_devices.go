package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
)

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
