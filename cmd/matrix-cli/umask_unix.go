//go:build unix

package main

import "syscall"

func setUmask() {
	syscall.Umask(0o077)
}
