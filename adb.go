package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

func resolveADBPath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", errors.New("ADB 路径为空")
	}

	clean := filepath.Clean(input)
	fi, err := os.Stat(clean)
	if err != nil {
		if !strings.ContainsAny(input, `/\`) {
			if resolved, lookErr := exec.LookPath(input); lookErr == nil {
				return resolved, nil
			}
		}
		return "", err
	}

	adbExec := clean
	if fi.IsDir() {
		adbExec = filepath.Join(clean, adbExecutableName())
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

func adbPull(ctx context.Context, adbExec, serial, remotePath, localDir string) (string, error) {
	return runADB(ctx, adbExec, serial, "pull", remotePath, localDir)
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
