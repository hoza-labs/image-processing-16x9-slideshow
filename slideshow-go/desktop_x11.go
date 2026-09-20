//go:build linux || darwin

package main

import (
	"fmt"
	"image"
	"io"
	"math/bits"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xinerama"
	"github.com/jezek/xgb/xproto"
)

type linuxDesktop struct {
	c                *xgb.Conn
	setup            *xproto.SetupInfo
	screen           *xproto.ScreenInfo
	window           xproto.Window
	gc               xproto.Gcontext
	cursor           xproto.Cursor
	atoms            map[string]xproto.Atom
	width, height    int
	red, green, blue uint32
	bpp, pad         int
	keys             *xproto.GetKeyboardMappingReply
	xinerama         bool
	events           chan xEvent
	done             chan struct{}
}

type xEvent struct {
	event xgb.Event
	err   error
}

func newDesktop() (desktop, error) {
	c, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	setup := xproto.Setup(c)
	w := &linuxDesktop{c: c, setup: setup, screen: setup.DefaultScreen(c), atoms: map[string]xproto.Atom{}, width: 1280, height: 720, events: make(chan xEvent, 128), done: make(chan struct{})}
	fail := func(err error) (desktop, error) { c.Close(); return nil, err }
	for _, depth := range w.screen.AllowedDepths {
		for _, v := range depth.Visuals {
			if v.VisualId == w.screen.RootVisual {
				if v.Class != xproto.VisualClassTrueColor {
					return fail(fmt.Errorf("X11 desktop requires TrueColor"))
				}
				w.red, w.green, w.blue = v.RedMask, v.GreenMask, v.BlueMask
			}
		}
	}
	for _, f := range setup.PixmapFormats {
		if f.Depth == w.screen.RootDepth {
			w.bpp, w.pad = int(f.BitsPerPixel), int(f.ScanlinePad)
		}
	}
	if w.bpp != 16 && w.bpp != 24 && w.bpp != 32 {
		return fail(fmt.Errorf("unsupported X11 pixel format: %d", w.bpp))
	}
	w.window, err = xproto.NewWindowId(c)
	if err != nil {
		return fail(err)
	}
	err = xproto.CreateWindowChecked(c, w.screen.RootDepth, w.window, w.screen.Root, 0, 0, 1280, 720, 0, xproto.WindowClassInputOutput, w.screen.RootVisual, xproto.CwBackPixel|xproto.CwEventMask, []uint32{w.screen.BlackPixel, xproto.EventMaskExposure | xproto.EventMaskKeyPress | xproto.EventMaskButtonPress | xproto.EventMaskStructureNotify}).Check()
	if err != nil {
		return fail(err)
	}
	w.gc, err = xproto.NewGcontextId(c)
	if err != nil {
		return fail(err)
	}
	if err = xproto.CreateGCChecked(c, w.gc, xproto.Drawable(w.window), 0, nil).Check(); err != nil {
		return fail(err)
	}
	for _, name := range []string{"WM_PROTOCOLS", "WM_DELETE_WINDOW", "_NET_WM_STATE", "_NET_WM_STATE_FULLSCREEN"} {
		r, e := xproto.InternAtom(c, false, uint16(len(name)), name).Reply()
		if e != nil {
			return fail(e)
		}
		w.atoms[name] = r.Atom
	}
	data := make([]byte, 4)
	xgb.Put32(data, uint32(w.atoms["WM_DELETE_WINDOW"]))
	xproto.ChangeProperty(c, xproto.PropModeReplace, w.window, w.atoms["WM_PROTOCOLS"], xproto.AtomAtom, 32, 1, data)
	title := []byte("Photo slideshow")
	xproto.ChangeProperty(c, xproto.PropModeReplace, w.window, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(title)), title)
	// A zero-filled 1-bit pixmap used as both source and mask makes an invisible cursor.
	pix, err := xproto.NewPixmapId(c)
	if err != nil {
		return fail(err)
	}
	xproto.CreatePixmap(c, 1, pix, xproto.Drawable(w.window), 1, 1)
	gc, _ := xproto.NewGcontextId(c)
	xproto.CreateGC(c, gc, xproto.Drawable(pix), xproto.GcForeground, []uint32{0})
	xproto.PolyFillRectangle(c, xproto.Drawable(pix), gc, []xproto.Rectangle{{Width: 1, Height: 1}})
	w.cursor, err = xproto.NewCursorId(c)
	if err != nil {
		return fail(err)
	}
	xproto.CreateCursor(c, w.cursor, pix, pix, 0, 0, 0, 0, 0, 0, 0, 0)
	xproto.FreeGC(c, gc)
	xproto.FreePixmap(c, pix)
	if err = w.readKeys(); err != nil {
		return fail(err)
	}
	w.xinerama = xinerama.Init(c) == nil
	if err = xproto.MapWindowChecked(c, w.window).Check(); err != nil {
		return fail(err)
	}
	go func() {
		for {
			event, err := c.WaitForEvent()
			var resultErr error = err
			if event == nil && err == nil {
				resultErr = io.EOF
			}
			select {
			case w.events <- xEvent{event, resultErr}:
			case <-w.done:
				return
			}
			if resultErr != nil {
				return
			}
		}
	}()
	return w, nil
}
func (w *linuxDesktop) readKeys() error {
	s := w.setup
	r, err := xproto.GetKeyboardMapping(w.c, s.MinKeycode, byte(int(s.MaxKeycode)-int(s.MinKeycode)+1)).Reply()
	w.keys = r
	return err
}
func (w *linuxDesktop) Size() image.Point { return image.Pt(max(1, w.width), max(1, w.height)) }
func (w *linuxDesktop) ScreenSize() image.Point {
	if w.xinerama {
		r, err := xinerama.QueryScreens(w.c).Reply()
		if err == nil {
			p, e := xproto.TranslateCoordinates(w.c, w.window, w.screen.Root, 0, 0).Reply()
			if e == nil {
				cx, cy := int(p.DstX)+w.width/2, int(p.DstY)+w.height/2
				for _, s := range r.ScreenInfo {
					if cx >= int(s.XOrg) && cy >= int(s.YOrg) && cx < int(s.XOrg)+int(s.Width) && cy < int(s.YOrg)+int(s.Height) {
						return image.Pt(int(s.Width), int(s.Height))
					}
				}
			}
		}
	}
	r, err := xproto.GetGeometry(w.c, xproto.Drawable(w.screen.Root)).Reply()
	if err == nil {
		return image.Pt(int(r.Width), int(r.Height))
	}
	return image.Pt(int(w.screen.WidthInPixels), int(w.screen.HeightInPixels))
}
func (w *linuxDesktop) Fullscreen(on bool) {
	action := uint32(0)
	cursor := uint32(0)
	if on {
		action = 1
		cursor = uint32(w.cursor)
	}
	e := xproto.ClientMessageEvent{Format: 32, Window: w.window, Type: w.atoms["_NET_WM_STATE"], Data: xproto.ClientMessageDataUnionData32New([]uint32{action, uint32(w.atoms["_NET_WM_STATE_FULLSCREEN"]), 0, 1, 0})}
	xproto.SendEvent(w.c, false, w.screen.Root, xproto.EventMaskSubstructureRedirect|xproto.EventMaskSubstructureNotify, string(e.Bytes()))
	xproto.ChangeWindowAttributes(w.c, w.window, xproto.CwCursor, []uint32{cursor})
}
func colorBits(v byte, mask uint32) uint32 {
	if mask == 0 {
		return 0
	}
	shift := bits.TrailingZeros32(mask)
	return ((uint32(v)*(mask>>shift) + 127) / 255) << shift
}
func (w *linuxDesktop) Present(img *image.NRGBA) error {
	width, height := img.Bounds().Dx(), img.Bounds().Dy()
	stride := ((width*w.bpp + w.pad - 1) / w.pad) * (w.pad / 8)
	// Core X11 requests have a 16-bit length. Upload strips then copy the complete pixmap.
	rows := (int(w.setup.MaximumRequestLength)*4 - 24) / stride
	if rows < 1 {
		return fmt.Errorf("window is too wide for X11 image transfer")
	}
	pix, err := xproto.NewPixmapId(w.c)
	if err != nil {
		return err
	}
	defer xproto.FreePixmap(w.c, pix)
	if err = xproto.CreatePixmapChecked(w.c, w.screen.RootDepth, pix, xproto.Drawable(w.window), uint16(width), uint16(height)).Check(); err != nil {
		return err
	}
	for y := 0; y < height; y += rows {
		h := min(rows, height-y)
		data := make([]byte, stride*h)
		for row := 0; row < h; row++ {
			for x := 0; x < width; x++ {
				s := (y+row)*img.Stride + x*4
				v := colorBits(img.Pix[s], w.red) | colorBits(img.Pix[s+1], w.green) | colorBits(img.Pix[s+2], w.blue)
				d := row*stride + x*(w.bpp/8)
				for b := 0; b < w.bpp/8; b++ {
					shift := b * 8
					if w.setup.ImageByteOrder == xproto.ImageOrderMSBFirst {
						shift = w.bpp - 8 - b*8
					}
					data[d+b] = byte(v >> shift)
				}
			}
		}
		if err = xproto.PutImageChecked(w.c, xproto.ImageFormatZPixmap, xproto.Drawable(pix), w.gc, uint16(width), uint16(h), 0, int16(y), 0, w.screen.RootDepth, data).Check(); err != nil {
			return err
		}
	}
	return xproto.CopyAreaChecked(w.c, xproto.Drawable(pix), xproto.Drawable(w.window), w.gc, 0, 0, 0, 0, uint16(width), uint16(height)).Check()
}
func xKey(sym xproto.Keysym) string {
	switch sym {
	case 0xff1b:
		return "Escape"
	case 0x20:
		return "space"
	case 0xff53:
		return "Right"
	case 0xff51:
		return "Left"
	case 0xffc8:
		return "F11"
	}
	if sym < 128 {
		return string(rune(sym))
	}
	return ""
}
func (w *linuxDesktop) Poll() ([]string, error) {
	var result []string
	for count := 0; count < 128; count++ {
		var item xEvent
		select {
		case item = <-w.events:
		default:
			return result, nil
		}
		if item.err != nil {
			return nil, item.err
		}
		switch e := item.event.(type) {
		case xproto.ExposeEvent:
			if e.Count == 0 {
				result = append(result, "expose")
			}
		case xproto.ConfigureNotifyEvent:
			if int(e.Width) != w.width || int(e.Height) != w.height {
				w.width, w.height = int(e.Width), int(e.Height)
				result = append(result, "resize")
			}
		case xproto.KeyPressEvent:
			i := (int(e.Detail) - int(w.setup.MinKeycode)) * int(w.keys.KeysymsPerKeycode)
			if i >= 0 && i < len(w.keys.Keysyms) {
				result = append(result, xKey(w.keys.Keysyms[i]))
			}
		case xproto.ButtonPressEvent:
			if e.Detail == 1 {
				result = append(result, "click")
			}
		case xproto.ClientMessageEvent:
			if e.Type == w.atoms["WM_PROTOCOLS"] && e.Data.Data32[0] == uint32(w.atoms["WM_DELETE_WINDOW"]) {
				result = append(result, "close")
			}
		case xproto.DestroyNotifyEvent:
			result = append(result, "close")
		case xproto.MappingNotifyEvent:
			if err := w.readKeys(); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}
func (w *linuxDesktop) Close() { close(w.done); xproto.DestroyWindow(w.c, w.window); w.c.Close() }
