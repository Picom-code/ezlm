//go:build !windows

package app

func volumeLabel(string) string { return "" }
