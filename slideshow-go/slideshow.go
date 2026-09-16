package main

import (
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math/rand"
	"time"
)

type desktop interface {
	Size() image.Point
	ScreenSize() image.Point
	Present(*image.NRGBA) error
	Fullscreen(bool)
	Poll() ([]string, error)
	Close()
}

type shuffleBag struct {
	files, pending []string
	last           string
	random         *rand.Rand
}

func newShuffleBag(files []string, seed *string) *shuffleBag {
	if len(files) == 0 {
		panic("A shuffle bag needs at least one image")
	}
	s := time.Now().UnixNano()
	if seed != nil {
		h := fnv.New64a()
		_, _ = h.Write([]byte(*seed))
		s = int64(h.Sum64())
	}
	return &shuffleBag{files: append([]string(nil), files...), random: rand.New(rand.NewSource(s))}
}
func (b *shuffleBag) next() string {
	if len(b.pending) == 0 {
		b.pending = append([]string(nil), b.files...)
		b.random.Shuffle(len(b.pending), func(i, j int) { b.pending[i], b.pending[j] = b.pending[j], b.pending[i] })
		n := len(b.pending) - 1
		if n > 0 && b.pending[n] == b.last {
			b.pending[0], b.pending[n] = b.pending[n], b.pending[0]
		}
	}
	n := len(b.pending) - 1
	b.last = b.pending[n]
	b.pending = b.pending[:n]
	return b.last
}

type slideshow struct {
	w                          desktop
	bag                        *shuffleBag
	interval                   time.Duration
	history                    []string
	position                   int
	bad                        map[string]bool
	current                    string
	photo                      *image.NRGBA
	background                 color.NRGBA
	paused, fullscreen, closed bool
	exitCode                   int
	deadline, resizeDeadline   time.Time
	stderr                     io.Writer
	clock                      func() time.Time
}

func newSlideshow(w desktop, files []string, o options, bg color.NRGBA, stderr io.Writer) *slideshow {
	a := &slideshow{w: w, bag: newShuffleBag(files, o.seed), interval: o.interval, position: -1, bad: map[string]bool{}, background: bg, fullscreen: !o.windowed, stderr: stderr, clock: time.Now}
	w.Fullscreen(a.fullscreen)
	return a
}
func (a *slideshow) restartTimer(now time.Time) {
	a.deadline = time.Time{}
	if !a.paused && !a.closed {
		a.deadline = now.Add(a.interval)
	}
}
func (a *slideshow) display(path string) bool {
	size, screen := a.w.Size(), a.w.ScreenSize()
	size.X, size.Y = max(1, size.X), max(1, size.Y)
	photo, err := loadImage(path, image.Pt(max(1, min(size.X, screen.X)), max(1, min(size.Y, screen.Y))))
	if err != nil {
		fmt.Fprintf(a.stderr, "Skipping %s: %v\n", path, err)
		a.bad[path] = true
		return false
	}
	frame := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
	draw.Draw(frame, frame.Bounds(), image.NewUniform(a.background), image.Point{}, draw.Src)
	offset := image.Pt((size.X-photo.Bounds().Dx())/2, (size.Y-photo.Bounds().Dy())/2)
	draw.Draw(frame, photo.Bounds().Add(offset), photo, image.Point{}, draw.Over)
	if err := a.w.Present(frame); err != nil {
		fmt.Fprintln(a.stderr, "Desktop error:", err)
		a.exitCode = 1
		a.close()
		return false
	}
	a.photo, a.current = photo, path
	return true
}
func (a *slideshow) nextImage(now time.Time) {
	if a.closed {
		return
	}
	for a.position+1 < len(a.history) {
		a.position++
		path := a.history[a.position]
		if !a.bad[path] && a.display(path) {
			a.restartAfterDisplay(now)
			return
		}
		if a.closed {
			return
		}
	}
	for i := 0; i < 2*len(a.bag.files); i++ {
		path := a.bag.next()
		if !a.bad[path] && a.display(path) {
			a.history = append(a.history, path)
			a.position = len(a.history) - 1
			a.restartAfterDisplay(now)
			return
		}
		if a.closed {
			return
		}
	}
	fmt.Fprintln(a.stderr, "No readable images remain; exiting.")
	a.exitCode = 1
	a.close()
}

// Start after decoding so slow files still get a full interval on screen.
func (a *slideshow) restartAfterDisplay(now time.Time) { a.restartTimer(a.clock()) }
func (a *slideshow) previousImage(now time.Time) {
	original := a.position
	for a.position > 0 {
		a.position--
		path := a.history[a.position]
		if !a.bad[path] && a.display(path) {
			a.restartAfterDisplay(now)
			return
		}
		if a.closed {
			return
		}
	}
	a.position = original
	a.restartTimer(now)
}
func (a *slideshow) handle(key string, now time.Time) {
	if a.closed {
		return
	}
	switch key {
	case "Escape", "q", "Q", "close":
		a.close()
	case "space":
		a.paused = !a.paused
		a.restartTimer(now)
	case "Right", "n", "N", "click":
		a.nextImage(now)
	case "Left", "p", "P":
		a.previousImage(now)
	case "f", "F", "F11":
		a.fullscreen = !a.fullscreen
		a.w.Fullscreen(a.fullscreen)
		a.resizeDeadline = now.Add(100 * time.Millisecond)
	case "resize":
		a.resizeDeadline = now.Add(100 * time.Millisecond)
	case "expose":
		if a.current != "" && !a.display(a.current) {
			a.nextImage(now)
		}
	}
}
func (a *slideshow) tick(now time.Time) {
	if a.closed {
		return
	}
	if !a.resizeDeadline.IsZero() && !now.Before(a.resizeDeadline) {
		a.resizeDeadline = time.Time{}
		if a.current != "" && !a.display(a.current) {
			a.nextImage(now)
		}
	}
	if !a.deadline.IsZero() && !now.Before(a.deadline) {
		a.nextImage(now)
	}
}
func (a *slideshow) close() {
	a.closed = true
	a.deadline = time.Time{}
	a.resizeDeadline = time.Time{}
}
