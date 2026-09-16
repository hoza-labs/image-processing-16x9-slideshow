package main

import (
	"fmt"
	"image"
	"runtime"
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")
var gdi32 = syscall.NewLazyDLL("gdi32.dll")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var defWindowProc = user32.NewProc("DefWindowProcW")
var nativeWindow *windowsDesktop // Windows messages are dispatched on the locked GUI thread.

type winRect struct{ Left, Top, Right, Bottom int32 }
type winPoint struct{ X, Y int32 }
type winMessage struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Point   winPoint
	Private uint32
}
type winClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}
type monitorInfo struct {
	Size          uint32
	Monitor, Work winRect
	Flags         uint32
}
type bitmapInfo struct {
	Size                        uint32
	Width, Height               int32
	Planes, BitCount            uint16
	Compression, SizeImage      uint32
	XPels, YPels                int32
	ColorsUsed, ColorsImportant uint32
}
type windowsDesktop struct {
	hwnd          uintptr
	events        []string
	frame         []byte
	width, height int
	fullscreen    bool
	saved         winRect
	cursor        uintptr
}

func windowProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	w := nativeWindow
	if w != nil {
		switch msg {
		case 0x10:
			w.events = append(w.events, "close")
			return 0 // WM_CLOSE
		case 0x05:
			w.events = append(w.events, "resize")
		case 0x0f: // WM_PAINT: validate after presenting our retained frame.
			w.paint()
			user32.NewProc("ValidateRect").Call(hwnd, 0)
			return 0
		case 0x14:
			return 1 // avoid erasing/flashing between photos
		case 0x20: // WM_SETCURSOR: hide only the application's client cursor.
			if lparam&0xffff == 1 {
				cursor := w.cursor
				if w.fullscreen {
					cursor = 0
				}
				user32.NewProc("SetCursor").Call(cursor)
				return 1
			}
		case 0x100, 0x104:
			if key := windowsKey(wparam); key != "" {
				w.events = append(w.events, key)
				return 0
			}
		case 0x201:
			w.events = append(w.events, "click")
			return 0
		}
	}
	r, _, _ := defWindowProc.Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func windowsKey(v uintptr) string {
	switch v {
	case 27:
		return "Escape"
	case 32:
		return "space"
	case 39:
		return "Right"
	case 37:
		return "Left"
	case 122:
		return "F11"
	case 'Q':
		return "q"
	case 'N':
		return "n"
	case 'P':
		return "p"
	case 'F':
		return "f"
	}
	return ""
}

func newDesktop() (desktop, error) {
	runtime.LockOSThread()
	user32.NewProc("SetProcessDPIAware").Call()
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)
	name, _ := syscall.UTF16PtrFromString("GoPhotoSlideshow")
	cursor, _, _ := user32.NewProc("LoadCursorW").Call(0, 32512)
	class := winClass{Size: uint32(unsafe.Sizeof(winClass{})), Proc: syscall.NewCallback(windowProc), Instance: instance, Cursor: cursor, Name: name}
	r, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if r == 0 && err != syscall.Errno(1410) {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("register window: %w", err)
	}
	w := &windowsDesktop{cursor: cursor}
	nativeWindow = w
	title, _ := syscall.UTF16PtrFromString("Photo slideshow")
	rect := winRect{Right: 1280, Bottom: 720}
	user32.NewProc("AdjustWindowRect").Call(uintptr(unsafe.Pointer(&rect)), 0x00cf0000, 0)
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(title)), 0x00cf0000, 0x80000000, 0x80000000, uintptr(rect.Right-rect.Left), uintptr(rect.Bottom-rect.Top), 0, 0, instance, 0)
	if hwnd == 0 {
		nativeWindow = nil
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("create window: %w", err)
	}
	w.hwnd = hwnd
	user32.NewProc("ShowWindow").Call(hwnd, 5)
	user32.NewProc("UpdateWindow").Call(hwnd)
	return w, nil
}
func (w *windowsDesktop) Size() image.Point {
	var r winRect
	user32.NewProc("GetClientRect").Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	return image.Pt(max(1, int(r.Right)), max(1, int(r.Bottom)))
}
func (w *windowsDesktop) monitor() winRect {
	m, _, _ := user32.NewProc("MonitorFromWindow").Call(w.hwnd, 2)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	user32.NewProc("GetMonitorInfoW").Call(m, uintptr(unsafe.Pointer(&info)))
	return info.Monitor
}
func (w *windowsDesktop) ScreenSize() image.Point {
	r := w.monitor()
	return image.Pt(int(r.Right-r.Left), int(r.Bottom-r.Top))
}
func (w *windowsDesktop) Fullscreen(on bool) {
	if on == w.fullscreen {
		return
	}
	w.fullscreen = on
	index := int32(-16)
	// SetWindowLongW suffices for the 32-bit style field on both supported architectures.
	if on {
		user32.NewProc("GetWindowRect").Call(w.hwnd, uintptr(unsafe.Pointer(&w.saved)))
		r := w.monitor()
		user32.NewProc("SetWindowLongW").Call(w.hwnd, uintptr(index), 0x90000000)
		user32.NewProc("SetWindowPos").Call(w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x0020)
	} else {
		r := w.saved
		user32.NewProc("SetWindowLongW").Call(w.hwnd, uintptr(index), 0x10cf0000)
		user32.NewProc("SetWindowPos").Call(w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), 0x0020)
	}
	cursor := w.cursor
	if on {
		cursor = 0
	}
	user32.NewProc("SetCursor").Call(cursor)
}
func (w *windowsDesktop) Present(img *image.NRGBA) error {
	w.width, w.height = img.Bounds().Dx(), img.Bounds().Dy()
	w.frame = make([]byte, w.width*w.height*4)
	for y := 0; y < w.height; y++ {
		for x := 0; x < w.width; x++ {
			s := y*img.Stride + x*4
			d := (y*w.width + x) * 4
			w.frame[d], w.frame[d+1], w.frame[d+2] = img.Pix[s+2], img.Pix[s+1], img.Pix[s]
		}
	}
	return w.paint()
}
func (w *windowsDesktop) paint() error {
	if w.hwnd == 0 || len(w.frame) == 0 {
		return nil
	}
	dc, _, err := user32.NewProc("GetDC").Call(w.hwnd)
	if dc == 0 {
		return fmt.Errorf("get display context: %w", err)
	}
	defer user32.NewProc("ReleaseDC").Call(w.hwnd, dc)
	info := bitmapInfo{Size: 40, Width: int32(w.width), Height: -int32(w.height), Planes: 1, BitCount: 32}
	r, _, _ := gdi32.NewProc("SetDIBitsToDevice").Call(dc, 0, 0, uintptr(w.width), uintptr(w.height), 0, 0, 0, uintptr(w.height), uintptr(unsafe.Pointer(&w.frame[0])), uintptr(unsafe.Pointer(&info)), 0)
	if r == 0 {
		return fmt.Errorf("could not draw image")
	}
	return nil
}
func (w *windowsDesktop) Poll() ([]string, error) {
	var m winMessage
	for {
		r, _, _ := user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 1)
		if r == 0 {
			break
		}
		if m.Message == 0x12 {
			w.events = append(w.events, "close")
		}
		user32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
		user32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
	}
	events := w.events
	w.events = nil
	return events, nil
}
func (w *windowsDesktop) Close() {
	user32.NewProc("DestroyWindow").Call(w.hwnd)
	nativeWindow = nil
	runtime.UnlockOSThread()
}
