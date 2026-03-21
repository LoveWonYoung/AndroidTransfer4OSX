package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	a := app.NewWithID("com.lianmin.at4m")
	w := a.NewWindow("Android 文件传输工具 (ADB)")
	w.Resize(fyne.NewSize(1600, 900))

	ui := newTransferUI(w)
	w.SetContent(ui.Content())
	w.Show()

	ui.RefreshDevices()
	a.Run()
}
