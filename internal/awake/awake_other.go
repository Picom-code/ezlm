//go:build !darwin && !linux && !windows

package awake

func hold(string) func() { return func() {} }
