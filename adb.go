package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultADBInput = "/Users/lianmin/platform-tools"

var adbPushSummaryRe = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([kmg]?b/s)\s*\((\d+)\s+bytes in\s+([0-9]+(?:\.[0-9]+)?)s\)`)
var adbPushPercentRe = regexp.MustCompile(`(?i)(\d{1,3})%`)
var adbPushRateRe = regexp.MustCompile(`(?i)([0-9]+(?:\.[0-9]+)?)\s*([kmg]?b/s)`)

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

func resolveADBPath(input string) (string, error) {
	if input == "" {
		return "", errors.New("ADB 路径为空")
	}

	clean := filepath.Clean(input)
	fi, err := os.Stat(clean)
	if err != nil {
		return "", err
	}

	adbExec := clean
	if fi.IsDir() {
		adbExec = filepath.Join(clean, "adb")
	}

	info, err := os.Stat(adbExec)
	if err != nil {
		return "", fmt.Errorf("找不到 adb 可执行文件: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("路径不是 adb 可执行文件: %s", adbExec)
	}
	return adbExec, nil
}

func adbListDevices(ctx context.Context, adbExec string) ([]deviceInfo, string, error) {
	out, err := runADB(ctx, adbExec, "", "devices", "-l")
	devices := parseADBDevices(out)
	if err != nil {
		return devices, out, err
	}
	return devices, out, nil
}

func adbEnsureDir(ctx context.Context, adbExec, serial, remoteDir string) (string, error) {
	return runADB(ctx, adbExec, serial, "shell", "mkdir", "-p", remoteDir)
}

func adbPush(ctx context.Context, adbExec, serial, localPath, remoteDir string) (string, error) {
	return runADB(ctx, adbExec, serial, "push", localPath, remoteDir)
}

func adbPushWithProgress(ctx context.Context, adbExec, serial, localPath, remoteDir string, onProgress func(adbPushProgressInfo)) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	fullArgs := make([]string, 0, 7)
	if serial != "" {
		fullArgs = append(fullArgs, "-s", serial)
	}
	fullArgs = append(fullArgs, "push", "-p", localPath, remoteDir)

	cmd := exec.CommandContext(ctx, adbExec, fullArgs...)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}

	if err := cmd.Start(); err != nil {
		return "", err
	}

	var output strings.Builder
	var outputMu sync.Mutex
	appendChunk := func(chunk string) {
		if chunk == "" {
			return
		}
		outputMu.Lock()
		output.WriteString(chunk)
		outputMu.Unlock()
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		consumeADBOutput(stdoutPipe, appendChunk, onProgress)
	}()
	go func() {
		defer wg.Done()
		consumeADBOutput(stderrPipe, appendChunk, onProgress)
	}()

	waitErr := cmd.Wait()
	wg.Wait()
	if waitErr != nil && ctx.Err() != nil {
		waitErr = ctx.Err()
	}

	outputMu.Lock()
	out := strings.TrimSpace(output.String())
	outputMu.Unlock()
	if waitErr != nil {
		if isADBPushProgressUnsupported(out) {
			// Compatibility fallback for older adb versions that don't support `push -p`.
			return adbPush(ctx, adbExec, serial, localPath, remoteDir)
		}
		return out, waitErr
	}
	return out, nil
}

func adbListStorageSuggestions(ctx context.Context, adbExec, serial string) ([]string, string, error) {
	out, err := runADB(ctx, adbExec, serial, "shell", "ls", "/storage")
	suggestions := parseStorageSuggestions(out)
	if err != nil {
		return suggestions, out, err
	}
	return suggestions, out, nil
}

func runADB(ctx context.Context, adbExec, serial string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	fullArgs := make([]string, 0, len(args)+2)
	if serial != "" {
		fullArgs = append(fullArgs, "-s", serial)
	}
	fullArgs = append(fullArgs, args...)

	cmd := exec.CommandContext(ctx, adbExec, fullArgs...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil && ctx.Err() != nil {
		// Surface cancellation/timeout errors explicitly for clearer UI handling.
		err = ctx.Err()
	}

	out := stdout.String()
	if strings.TrimSpace(stderr.String()) != "" {
		if strings.TrimSpace(out) != "" {
			out += "\n"
		}
		out += stderr.String()
	}
	if err != nil {
		return strings.TrimSpace(out), err
	}
	return strings.TrimSpace(out), nil
}

func parseADBDevices(out string) []deviceInfo {
	lines := strings.Split(out, "\n")
	var devices []deviceInfo
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "List of devices attached") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		d := deviceInfo{
			Serial: fields[0],
			Status: fields[1],
			Raw:    line,
		}
		for _, f := range fields[2:] {
			if strings.HasPrefix(f, "model:") {
				d.Model = strings.TrimPrefix(f, "model:")
				break
			}
		}
		devices = append(devices, d)
	}
	return devices
}

func parseStorageSuggestions(out string) []string {
	// Always provide a sensible internal-storage default.
	const internalDefault = "/sdcard/Download"

	excluded := map[string]struct{}{
		"self":         {},
		"emulated":     {},
		"enc_emulated": {},
	}

	seen := map[string]struct{}{
		internalDefault: {},
	}
	external := make([]string, 0)

	for _, token := range strings.Fields(out) {
		name := strings.TrimSpace(token)
		if name == "" {
			continue
		}
		// `ls /storage` should return names, but tolerate accidental paths.
		name = strings.TrimPrefix(name, "/storage/")
		name = strings.Trim(name, "/")
		if name == "" {
			continue
		}
		if _, skip := excluded[name]; skip {
			continue
		}

		suggestion := "/storage/" + name + "/Download/mymac"
		if _, ok := seen[suggestion]; ok {
			continue
		}
		seen[suggestion] = struct{}{}
		external = append(external, suggestion)
	}

	sort.Strings(external)
	return append([]string{internalDefault}, external...)
}

func parseADBPushSpeed(out string) (adbPushSpeedInfo, bool) {
	matches := adbPushSummaryRe.FindAllStringSubmatch(out, -1)
	if len(matches) == 0 {
		return adbPushSpeedInfo{}, false
	}

	last := matches[len(matches)-1]
	if len(last) < 5 {
		return adbPushSpeedInfo{}, false
	}

	unit := strings.ToUpper(strings.TrimSuffix(last[2], "/s")) + "/s"
	speedText := strings.TrimSpace(last[1] + " " + unit)
	bytesVal, err := strconv.ParseInt(last[3], 10, 64)
	if err != nil {
		return adbPushSpeedInfo{}, false
	}
	seconds, err := strconv.ParseFloat(last[4], 64)
	if err != nil || seconds <= 0 {
		return adbPushSpeedInfo{}, false
	}

	duration := time.Duration(seconds * float64(time.Second))
	return adbPushSpeedInfo{
		RawSpeed: speedText,
		Bytes:    bytesVal,
		Duration: duration,
	}, true
}

func parseADBPushProgressLine(line string) (adbPushProgressInfo, bool) {
	text := strings.TrimSpace(line)
	if text == "" {
		return adbPushProgressInfo{}, false
	}

	info := adbPushProgressInfo{
		Percent: -1,
		RawLine: text,
	}

	if m := adbPushPercentRe.FindStringSubmatch(text); len(m) == 2 {
		if pct, err := strconv.Atoi(m[1]); err == nil {
			if pct < 0 {
				pct = 0
			}
			if pct > 100 {
				pct = 100
			}
			info.Percent = pct
		}
	}
	if m := adbPushRateRe.FindStringSubmatch(text); len(m) == 3 {
		unit := strings.ToUpper(strings.TrimSuffix(m[2], "/s")) + "/s"
		info.Speed = strings.TrimSpace(m[1] + " " + unit)
	}

	if info.Percent < 0 && info.Speed == "" {
		return adbPushProgressInfo{}, false
	}
	return info, true
}

func consumeADBOutput(reader io.Reader, appendChunk func(string), onProgress func(adbPushProgressInfo)) {
	if reader == nil {
		return
	}

	r := bufio.NewReader(reader)
	var lineBuf strings.Builder
	flushLine := func() {
		line := strings.TrimSpace(lineBuf.String())
		lineBuf.Reset()
		if line == "" || onProgress == nil {
			return
		}
		if info, ok := parseADBPushProgressLine(line); ok {
			onProgress(info)
		}
	}

	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			chunk := string(tmp[:n])
			appendChunk(chunk)
			for i := 0; i < n; i++ {
				b := tmp[i]
				if b == '\n' || b == '\r' {
					flushLine()
					continue
				}
				lineBuf.WriteByte(b)
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				flushLine()
			}
			return
		}
	}
}

func isADBPushProgressUnsupported(out string) bool {
	lower := strings.ToLower(out)
	if !strings.Contains(lower, "-p") {
		return false
	}
	if strings.Contains(lower, "unknown option") || strings.Contains(lower, "invalid option") {
		return true
	}
	return strings.Contains(lower, "usage: adb push")
}
