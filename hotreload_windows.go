//go:build windows
// +build windows

package main

func setupReloadSignal(recompile func(string)) {
	// Windows doesn't support SIGUSR1, so we skip signal-based reload
}
