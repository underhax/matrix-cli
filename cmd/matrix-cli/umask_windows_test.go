//go:build windows

package main

import "testing"

func TestSetUmask(t *testing.T) {
	setUmask()
}
