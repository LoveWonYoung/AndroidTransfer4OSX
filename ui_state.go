package main

import (
	"context"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

const fileDialogWidth = 1000
const fileDialogHeight = 720

const (
	refreshDevicesTimeout = 8 * time.Second
	scanStorageTimeout    = 5 * time.Second
	adbEnsureDirTimeout   = 8 * time.Second
	adbPushTimeout        = 30 * time.Minute
	adbPullTimeout        = 30 * time.Minute
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
	pullRemoteEntry *widget.Entry
	pullLocalEntry  *widget.Entry
	statusLabel     *widget.Label
	logBox          *widget.Entry
	queueList       *widget.List
	progress        *widget.ProgressBarInfinite

	pushBtn        *widget.Button
	pullBtn        *widget.Button
	refreshBtn     *widget.Button
	scanStorageBtn *widget.Button
	addPathBtn     *widget.Button
	removeItemBtn  *widget.Button
	clearQueueBtn  *widget.Button
	retryFailedBtn *widget.Button
	cancelBtn      *widget.Button
	clearLogBtn    *widget.Button
}
