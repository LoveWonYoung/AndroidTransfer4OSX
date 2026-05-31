package main

import (
	"fmt"
	"time"
)

type adbPushSpeedInfo struct {
	RawSpeed string
	Bytes    int64
	Duration time.Duration
}

type adbPushProgressInfo struct {
	Percent int
	Speed   string
	RawLine string
}

type deviceInfo struct {
	Serial string
	Status string
	Model  string
	Raw    string
}

func (d deviceInfo) Label() string {
	if d.Model != "" {
		return fmt.Sprintf("%s (%s)", d.Model, d.Serial)
	}
	return d.Serial
}
