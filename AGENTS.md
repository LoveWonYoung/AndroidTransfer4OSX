# Repository Guidelines

## Project Structure & Module Organization

This repository is a small Go desktop app (Fyne) for transferring files to Android via ADB.

- `main.go`: app entry point and window startup.
- `ui.go`: Fyne UI, dialogs, button handlers, status/log updates.
- `adb.go`: ADB command execution, device parsing, and transfer helpers.
- `go.mod` / `go.sum`: Go module and dependency locks.
- `at4m`: local build artifact (generated binary, do not treat as source).

Keep new code in the root package unless a clear reusable package boundary emerges.

## Build, Test, and Development Commands

- `go run .`: run the app locally.
- `go build -o at4m .`: build the desktop binary.
- `go test ./...`: run all tests (currently none may exist).
- `gofmt -w *.go`: format Go files before committing.

If dependencies change, update `go.mod`/`go.sum` via normal `go` commands (`go get`, `go mod tidy`).

## Coding Style & Naming Conventions

- Use standard Go formatting (`gofmt`), tabs for indentation, and idiomatic Go naming.
- Keep exported identifiers minimal; this project currently uses unexported helpers (`newTransferUI`, `runADB`) within `package main`.
- Prefer small focused functions for UI actions and ADB helpers.
- Name test files with `*_test.go` and test functions as `TestXxx`.

## Testing Guidelines

No formal test suite is present yet. Add unit tests for parsing and command logic first (for example `parseADBDevices` in `adb.go`) before UI integration tests.

- Place tests beside source files in the root package.
- Use table-driven tests for parsing/validation behavior.
- Run `go test ./...` before opening a PR.

## Commit & Pull Request Guidelines

Git history is not available in this workspace snapshot, so no local convention can be inferred. Use clear, imperative commit messages, e.g. `ui: disable push during device refresh`.

For pull requests:

- Describe the behavior change and why it was needed.
- List manual test steps (device refresh, file push, error handling).
- Include screenshots/GIFs for UI changes (`ui.go`).
- Note OS/environment assumptions (ADB path, platform-tools location).

## Security & Configuration Tips

- Avoid committing machine-specific paths (for example hard-coded ADB defaults).
- Do not commit generated binaries like `at4m` unless intentionally releasing artifacts.
