package main

import (
	"testing"
)

func nativeInput(t *testing.T, d desktop, key string) {
	t.Helper()
	w := d.(*windowsDesktop)
	code := map[string]uintptr{"space": 32, "Right": 39, "Left": 37, "n": 'N', "N": 'N', "p": 'P', "P": 'P', "f": 'F', "F": 'F', "F11": 122, "Escape": 27}[key]
	msg := uintptr(0x100)
	if key == "click" {
		msg = 0x201
	}
	r, _, err := user32.NewProc("PostMessageW").Call(w.hwnd, msg, code, 0)
	if r == 0 {
		t.Fatal(err)
	}
}
func nativeResize(t *testing.T, d desktop) {
	t.Helper()
	w := d.(*windowsDesktop)
	r, _, err := user32.NewProc("SetWindowPos").Call(w.hwnd, 0, 0, 0, 640, 480, 0x0006)
	if r == 0 {
		t.Fatal(err)
	}
}
