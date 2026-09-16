package main

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func nativeInput(t *testing.T, d desktop, key string) {
	t.Helper()
	w := d.(*linuxDesktop)
	var data string
	mask := uint32(xproto.EventMaskKeyPress)
	if key == "click" {
		mask = xproto.EventMaskButtonPress
		data = string((xproto.ButtonPressEvent{Detail: 1, Event: w.window, Root: w.screen.Root, SameScreen: true}).Bytes())
	} else {
		var code xproto.Keycode
		for i, sym := range w.keys.Keysyms {
			if xKey(sym) == key {
				code = xproto.Keycode(i/int(w.keys.KeysymsPerKeycode) + int(xproto.Setup(w.c).MinKeycode))
				break
			}
		}
		if code == 0 {
			t.Fatalf("no keycode for %s", key)
		}
		data = string((xproto.KeyPressEvent{Detail: code, Event: w.window, Root: w.screen.Root, SameScreen: true}).Bytes())
	}
	if err := xproto.SendEventChecked(w.c, false, w.window, mask, data).Check(); err != nil {
		t.Fatal(err)
	}
}
func nativeResize(t *testing.T, d desktop) {
	t.Helper()
	w := d.(*linuxDesktop)
	if err := xproto.ConfigureWindowChecked(w.c, w.window, xproto.ConfigWindowWidth|xproto.ConfigWindowHeight, []uint32{640, 480}).Check(); err != nil {
		t.Fatal(err)
	}
}
