package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

func writeFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}
func solid(w, h int, c color.Color) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(im, im.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return im
}
func orientationData(v int) []byte {
	b := make([]byte, 26)
	copy(b, "II")
	binary.LittleEndian.PutUint16(b[2:], 42)
	binary.LittleEndian.PutUint32(b[4:], 8)
	binary.LittleEndian.PutUint16(b[8:], 1)
	binary.LittleEndian.PutUint16(b[10:], 274)
	binary.LittleEndian.PutUint16(b[12:], 3)
	binary.LittleEndian.PutUint32(b[14:], 1)
	binary.LittleEndian.PutUint16(b[18:], uint16(v))
	return b
}
func fixtures(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "photos with spaces")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct {
		name string
		w, h int
	}{{"landscape.JPG", 600, 300}, {"portrait.png", 100, 300}, {"square.bmp", 100, 100}, {"large.tiff", 4000, 2500}, {"rotated.jpg", 60, 30}} {
		im := solid(f.w, f.h, color.NRGBA{255, 0, 0, 255})
		var b bytes.Buffer
		var err error
		switch filepath.Ext(f.name) {
		case ".JPG", ".jpg":
			err = jpeg.Encode(&b, im, nil)
		case ".png":
			err = png.Encode(&b, im)
		case ".bmp":
			err = bmp.Encode(&b, im)
		case ".tiff":
			err = tiff.Encode(&b, im, nil)
		}
		if err != nil {
			t.Fatal(err)
		}
		data := b.Bytes()
		if f.name == "rotated.jpg" {
			exif := append([]byte("Exif\x00\x00"), orientationData(6)...)
			header := []byte{0xff, 0xd8, 0xff, 0xe1, 0, byte(len(exif) + 2)}
			data = append(append(header, exif...), data[2:]...)
		}
		writeFixture(t, filepath.Join(dir, f.name), data)
	}
	writeFixture(t, filepath.Join(dir, "invalid.gif"), []byte("not an image"))
	writeFixture(t, filepath.Join(dir, "ignored.txt"), []byte("ignore"))
	return dir
}
func TestDiscovery(t *testing.T) {
	dir := fixtures(t)
	files, err := discover(dir, false)
	if err != nil || len(files) != 6 {
		t.Fatalf("%v %v", files, err)
	}
	nested := filepath.Join(dir, "nested")
	if err = os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(nested, "extra.WEBP"), nil)
	for _, tc := range []struct {
		recursive bool
		want      int
	}{{false, 6}, {true, 7}} {
		files, err = discover(dir, tc.recursive)
		if err != nil || len(files) != tc.want || !sort.StringsAreSorted(files) {
			t.Fatalf("%v %v", files, err)
		}
	}
	for _, path := range []string{filepath.Join(dir, "missing"), filepath.Join(dir, "ignored.txt"), t.TempDir()} {
		if _, err = discover(path, false); err == nil {
			t.Errorf("accepted %s", path)
		}
	}
}
func TestContainAndOrientation(t *testing.T) {
	dir := fixtures(t)
	for _, tc := range []struct {
		name string
		size image.Point
	}{{"landscape.JPG", image.Pt(200, 100)}, {"portrait.png", image.Pt(67, 200)}, {"square.bmp", image.Pt(200, 200)}, {"large.tiff", image.Pt(200, 125)}, {"rotated.jpg", image.Pt(100, 200)}} {
		t.Run(tc.name, func(t *testing.T) {
			im, err := loadImage(filepath.Join(dir, tc.name), image.Pt(200, 200))
			if err != nil {
				t.Fatal(err)
			}
			if im.Bounds().Size() != tc.size {
				t.Fatalf("got %v want %v", im.Bounds(), tc.size)
			}
		})
	}
}
func TestAllOrientations(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			src.SetNRGBA(x, y, opaque(byte(y*2+x+1), 0, 0))
		}
	}
	wants := [][]byte{{1, 2, 3, 4, 5, 6}, {2, 1, 4, 3, 6, 5}, {6, 5, 4, 3, 2, 1}, {5, 6, 3, 4, 1, 2}, {1, 3, 5, 2, 4, 6}, {5, 3, 1, 6, 4, 2}, {6, 4, 2, 5, 3, 1}, {2, 4, 6, 1, 3, 5}}
	for v, want := range wants {
		out := orient(src, v+1)
		var got []byte
		for y := 0; y < out.Bounds().Dy(); y++ {
			for x := 0; x < out.Bounds().Dx(); x++ {
				r, _, _, _ := out.At(x, y).RGBA()
				got = append(got, byte(r>>8))
			}
		}
		if !bytes.Equal(got, want) {
			t.Errorf("orientation %d: %v", v+1, got)
		}
		if exifOrientation(orientationData(v+1)) != v+1 {
			t.Fatal("TIFF metadata")
		}
	}
	for _, bad := range [][]byte{nil, []byte("not exif"), {0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff}, []byte("II\x2a\x00\xff\xff\xff\xff")} {
		if exifOrientation(bad) != 1 {
			t.Fatal("bad EXIF")
		}
	}
}
func TestGIFAndWebP(t *testing.T) {
	dir := t.TempDir()
	palette := color.Palette{color.Black, color.White}
	first := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	first.SetColorIndex(0, 0, 1)
	second := image.NewPaletted(first.Bounds(), palette)
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{first, second}, Delay: []int{1, 1}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "animated.gif")
	writeFixture(t, path, b.Bytes())
	im, err := loadImage(path, image.Pt(2, 1))
	if err != nil {
		t.Fatal(err)
	}
	if im.NRGBAAt(0, 0).R != 255 {
		t.Fatal("did not show first GIF frame")
	}
	data, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, "photo.webp")
	writeFixture(t, path, data)
	if _, err = loadImage(path, image.Pt(20, 20)); err != nil {
		t.Fatal(err)
	}
}
func TestCyclesAndSeed(t *testing.T) {
	files := []string{"0", "1", "2", "3", "4", "5", "6", "7"}
	seed := "memorial"
	bag, same := newShuffleBag(files, &seed), newShuffleBag(files, &seed)
	last := ""
	cycles := map[string]bool{}
	for cycle := 0; cycle < 10; cycle++ {
		var values []string
		for i := 0; i < 8; i++ {
			v := bag.next()
			if v != same.next() {
				t.Fatal("seed is not reproducible")
			}
			if i == 0 && v == last {
				t.Fatal("cycle boundary repeats")
			}
			values = append(values, v)
			last = v
		}
		cycles[strings.Join(values, ",")] = true
		sort.Strings(values)
		if !reflect.DeepEqual(values, files) {
			t.Fatal(values)
		}
	}
	if len(cycles) < 2 {
		t.Fatal("cycles never change")
	}
}
func TestSmallBags(t *testing.T) {
	for _, files := range [][]string{{"1"}, {"1", "2"}} {
		bag := newShuffleBag(files, nil)
		last := ""
		for i := 0; i < 20; i++ {
			v := bag.next()
			if len(files) == 1 && v != "1" || len(files) == 2 && v == last {
				t.Fatal(v)
			}
			last = v
		}
	}
}
func TestIntervals(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want time.Duration
	}{{"2.5", 2500 * time.Millisecond}, {"7", 7 * time.Second}, {".00001", time.Millisecond}, {"2147483", 2147483 * time.Second}} {
		got, err := positiveSeconds(tc.s)
		if err != nil || got != tc.want {
			t.Fatalf("%s: %v %v", tc.s, got, err)
		}
	}
	for _, s := range []string{"0", "-1", "nan", "NaN", "inf", "Inf", "text", "1e100"} {
		if _, err := positiveSeconds(s); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
}

type fakeDesktop struct {
	size, screen image.Point
	frame        *image.NRGBA
	fullscreen   bool
	presents     int
	err          error
}

func (w *fakeDesktop) Size() image.Point             { return w.size }
func (w *fakeDesktop) ScreenSize() image.Point       { return w.screen }
func (w *fakeDesktop) Present(im *image.NRGBA) error { w.frame = im; w.presents++; return w.err }
func (w *fakeDesktop) Fullscreen(v bool)             { w.fullscreen = v }
func (w *fakeDesktop) Poll() ([]string, error)       { return nil, nil }
func (w *fakeDesktop) Close()                        {}
func startTest(t *testing.T, files []string, interval time.Duration) (*slideshow, *fakeDesktop, *time.Time, *bytes.Buffer) {
	t.Helper()
	w := &fakeDesktop{size: image.Pt(320, 200), screen: image.Pt(1920, 1080)}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	seed := "test"
	errors := &bytes.Buffer{}
	a := newSlideshow(w, files, options{interval: interval, seed: &seed, windowed: true}, opaque(0, 0, 0), errors)
	a.clock = func() time.Time { return now }
	a.nextImage(now)
	return a, w, &now, errors
}
func TestValidCycleAndInvalidSkipping(t *testing.T) {
	dir := fixtures(t)
	files, _ := discover(dir, false)
	a, _, now, errs := startTest(t, files, time.Second)
	for i := 0; i < 4; i++ {
		a.nextImage(*now)
	}
	unique := map[string]bool{}
	for _, p := range a.history {
		unique[p] = true
	}
	if len(unique) != 5 {
		t.Fatal(a.history)
	}
	a.nextImage(*now)
	if a.closed || !strings.Contains(errs.String(), "invalid.gif") {
		t.Fatalf("closed=%v errors=%s", a.closed, errs)
	}
	for i := 0; i < 20; i++ {
		a.nextImage(*now)
	}
	if strings.Count(errs.String(), "invalid.gif") != 1 {
		t.Fatal("invalid file retried")
	}
}
func TestNavigationPauseResizeAndBindings(t *testing.T) {
	dir := fixtures(t)
	files := []string{filepath.Join(dir, "landscape.JPG"), filepath.Join(dir, "portrait.png")}
	a, w, now, _ := startTest(t, files, 5*time.Second)
	first := a.current
	a.nextImage(*now)
	second := a.current
	a.previousImage(*now)
	if a.current != first {
		t.Fatal("previous")
	}
	a.nextImage(*now)
	if a.current != second || len(a.history) != 2 {
		t.Fatal("forward history")
	}
	a.handle("space", *now)
	a.nextImage(*now)
	if !a.deadline.IsZero() {
		t.Fatal("paused navigation resumed")
	}
	a.handle("space", *now)
	if a.deadline.IsZero() {
		t.Fatal("resume")
	}
	a.handle("f", *now)
	if !a.fullscreen || !w.fullscreen {
		t.Fatal("fullscreen")
	}
	a.handle("F11", *now)
	if a.fullscreen {
		t.Fatal("windowed")
	}
	w.size = image.Pt(160, 120)
	a.handle("resize", *now)
	*now = now.Add(101 * time.Millisecond)
	a.tick(*now)
	if a.photo.Bounds().Dx() > 160 || a.photo.Bounds().Dy() > 120 {
		t.Fatal(a.photo.Bounds())
	}
	for _, key := range []string{"Right", "n", "N", "click"} {
		count := len(a.history)
		a.handle(key, *now)
		if len(a.history) != count+1 {
			t.Errorf("binding %s", key)
		}
	}
	for _, key := range []string{"Left", "p", "P"} {
		pos := a.position
		a.handle(key, *now)
		if a.position != pos-1 {
			t.Errorf("binding %s", key)
		}
	}
	for _, key := range []string{"f", "F", "F11"} {
		old := a.fullscreen
		a.handle(key, *now)
		if old == a.fullscreen {
			t.Errorf("binding %s", key)
		}
	}
	for _, key := range []string{"Escape", "q", "Q", "close"} {
		a, _, now, _ := startTest(t, files, time.Second)
		a.handle(key, *now)
		if !a.closed || !a.deadline.IsZero() || !a.resizeDeadline.IsZero() {
			t.Errorf("binding %s", key)
		}
	}
}
func TestTimers(t *testing.T) {
	dir := fixtures(t)
	for _, interval := range []time.Duration{150 * time.Millisecond, time.Second} {
		t.Run(interval.String(), func(t *testing.T) {
			a, _, now, _ := startTest(t, []string{filepath.Join(dir, "landscape.JPG")}, interval)
			a.handle("space", *now)
			count := len(a.history)
			*now = now.Add(interval + 50*time.Millisecond)
			a.tick(*now)
			if len(a.history) != count {
				t.Fatal("advanced while paused")
			}
			a.handle("space", *now)
			*now = now.Add(interval * 6 / 10)
			a.tick(*now)
			a.nextImage(*now)
			count = len(a.history)
			*now = now.Add(interval * 6 / 10)
			a.tick(*now)
			if len(a.history) != count {
				t.Fatal("manual navigation did not restart timer")
			}
			*now = now.Add(interval / 2)
			a.tick(*now)
			if len(a.history) != count+1 {
				t.Fatal("timer did not fire")
			}
			a.close()
			*now = now.Add(interval * 2)
			a.tick(*now)
			if len(a.history) != count+1 {
				t.Fatal("timer survived close")
			}
		})
	}
}
func TestAllInvalidExits(t *testing.T) {
	dir := fixtures(t)
	a, _, _, errs := startTest(t, []string{filepath.Join(dir, "invalid.gif")}, time.Second)
	if !a.closed || a.exitCode != 1 || !strings.Contains(errs.String(), "No readable images") {
		t.Fatalf("%+v %s", a, errs)
	}
}
func TestDeletedHistoryAndRedraw(t *testing.T) {
	dir := fixtures(t)
	a, _, now, _ := startTest(t, []string{filepath.Join(dir, "landscape.JPG"), filepath.Join(dir, "portrait.png")}, time.Second)
	first := a.current
	a.nextImage(*now)
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	a.previousImage(*now)
	if a.position != 1 || a.current == first {
		t.Fatal("failed previous damaged history position")
	}
	if err := os.Remove(a.current); err != nil {
		t.Fatal(err)
	}
	a.handle("resize", *now)
	*now = now.Add(time.Second)
	a.tick(*now)
	if !a.closed || a.exitCode != 1 {
		t.Fatal("redraw did not handle deleted file")
	}
}
func TestBackgroundCenterAndAlpha(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transparent.png")
	var b bytes.Buffer
	png.Encode(&b, solid(100, 50, color.NRGBA{255, 0, 0, 128}))
	writeFixture(t, path, b.Bytes())
	a, w, _, _ := startTest(t, []string{path}, time.Second)
	a.background = opaque(0, 0, 255)
	a.display(path)
	if w.frame.NRGBAAt(0, 0) != opaque(0, 0, 255) {
		t.Fatal("background not filled")
	}
	c := w.frame.NRGBAAt(160, 100)
	if c.R < 127 || c.B < 126 || c.A != 255 {
		t.Fatalf("alpha: %v", c)
	}
	w.screen = image.Pt(100, 100)
	a.display(path)
	if a.photo.Bounds().Size() != image.Pt(100, 50) {
		t.Fatal("screen cap")
	}
}
func TestColorsAndCLI(t *testing.T) {
	for _, s := range []string{"-i2.5", "-i=2.5", "--interval=2.5"} {
		o, err := parseArgs([]string{"photos", s})
		if err != nil || o.interval != 2500*time.Millisecond {
			t.Fatalf("%s: %+v %v", s, o, err)
		}
	}
	for _, s := range []string{"black", "Light Sky Blue", "gray42", "#abc", "#202020", "#123456789", "#111122223333"} {
		if _, err := parseColor(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []string{"", "garbage", "#gggggg", "#12"} {
		if _, err := parseColor(s); err == nil {
			t.Fatal(s)
		}
	}
	for _, args := range [][]string{{"photos with spaces", "--interval", "2.5", "--seed=rehearsal", "--recursive", "--windowed"}, {"--interval=2.5", "--seed", "rehearsal", "--recursive", "--windowed", "photos with spaces"}} {
		o, err := parseArgs(args)
		if err != nil || o.directory != "photos with spaces" || o.interval != 2500*time.Millisecond || !o.recursive || !o.windowed || o.seed == nil || *o.seed != "rehearsal" {
			t.Fatalf("%+v %v", o, err)
		}
	}
	for _, args := range [][]string{nil, {"photos", "--interval"}, {"photos", "--interval=0"}, {"photos", "--unknown"}, {"a", "b"}, {"a", "--recursive=true"}} {
		if _, err := parseArgs(args); err == nil {
			t.Fatal(args)
		}
	}
	var out bytes.Buffer
	if run([]string{"--help"}, &out, io.Discard) != 0 || !strings.Contains(out.String(), "Usage:") {
		t.Fatal("help")
	}
	if run([]string{filepath.Join(t.TempDir(), "missing")}, io.Discard, io.Discard) != 2 {
		t.Fatal("directory exit code")
	}
	dir := fixtures(t)
	if run([]string{dir, "--background=invalid-color"}, io.Discard, io.Discard) != 1 {
		t.Fatal("color exit code")
	}
}

func TestDiscoveryThroughDirectorySymlink(t *testing.T) {
	dir := fixtures(t)
	link := filepath.Join(t.TempDir(), "photo link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	files, err := discover(link, false)
	if err != nil || len(files) != 6 {
		t.Fatalf("%v %v", files, err)
	}
}

// Opt-in real desktop test, run on Windows or under Xvfb + a window manager.
func TestNativeDesktop(t *testing.T) {
	if os.Getenv("SLIDESHOW_GUI_TEST") != "1" {
		t.Skip("set SLIDESHOW_GUI_TEST=1 to exercise the native desktop")
	}
	dir := fixtures(t)
	files, _ := discover(dir, false)
	w, err := newDesktop()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	a := newSlideshow(w, files, options{windowed: true, interval: 150 * time.Millisecond}, opaque(0, 0, 0), io.Discard)
	a.nextImage(time.Now())
	if a.closed {
		t.Fatal("failed to load native image")
	}
	pump := func(d time.Duration) {
		until := time.Now().Add(d)
		for time.Now().Before(until) {
			events, err := w.Poll()
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range events {
				a.handle(e, time.Now())
			}
			a.tick(time.Now())
			time.Sleep(5 * time.Millisecond)
		}
	}
	nativeInput(t, w, "space")
	pump(50 * time.Millisecond)
	count := len(a.history)
	pump(200 * time.Millisecond)
	if len(a.history) != count {
		t.Fatal("native pause")
	}
	nativeInput(t, w, "space")
	pump(400 * time.Millisecond)
	if len(a.history) <= count {
		t.Fatal("native timer")
	}
	nativeInput(t, w, "space")
	pump(50 * time.Millisecond)
	for _, key := range []string{"Right", "n", "N", "click"} {
		count := len(a.history)
		nativeInput(t, w, key)
		pump(50 * time.Millisecond)
		if len(a.history) != count+1 {
			t.Fatalf("native binding %s", key)
		}
	}
	for _, key := range []string{"Left", "p", "P"} {
		pos := a.position
		nativeInput(t, w, key)
		pump(50 * time.Millisecond)
		if a.position != pos-1 {
			t.Fatalf("native binding %s", key)
		}
	}
	nativeResize(t, w)
	pump(250 * time.Millisecond)
	if w.Size().X > 640 || w.Size().Y > 480 || a.photo.Bounds().Dx() > w.Size().X || a.photo.Bounds().Dy() > w.Size().Y {
		t.Fatal("native resize", w.Size(), a.photo.Bounds())
	}
	nativeInput(t, w, "f")
	pump(200 * time.Millisecond)
	if !a.fullscreen || w.Size() != w.ScreenSize() {
		t.Fatal("native fullscreen", w.Size(), w.ScreenSize())
	}
	nativeInput(t, w, "F11")
	pump(200 * time.Millisecond)
	if a.fullscreen || w.Size().X > 640 {
		t.Fatal("native window restore")
	}
	nativeInput(t, w, "Escape")
	pump(50 * time.Millisecond)
	if !a.closed {
		t.Fatal("native close")
	}
}
