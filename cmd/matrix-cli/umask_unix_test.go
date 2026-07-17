//go:build unix

package main

import (
	"syscall"
	"testing"
)

func TestSetUmask(t *testing.T) {
	oldUmask := syscall.Umask(0)
	syscall.Umask(oldUmask)
	t.Cleanup(func() { syscall.Umask(oldUmask) })

	setUmask()

	currentUmask := syscall.Umask(oldUmask)
	if currentUmask != 0o077 {
		t.Errorf("expected umask 0o077, got %04o", currentUmask)
	}
}
