// Slideshow is a shuffled photo viewer for Windows, Linux, and macOS.
package main

import (
	"errors"
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

type options struct {
	directory, background string
	interval              time.Duration
	seed                  *string
	recursive, windowed   bool
}

const usage = `Usage: slideshow DIRECTORY [options]
A full-screen, shuffled photo slideshow.

  -i, --interval SECONDS  Positive interval (default 5)
  --background COLOR     X11/Tk color name or #RGB/#RRGGBB (default black)
  --recursive            Include subdirectories
  --seed VALUE           Reproducible shuffle
  --windowed             Start in a 1280x720 window
  -h, --help             Show help

Esc/Q: quit; Space: pause; Right/N/click: next; Left/P: previous; F/F11: full screen.
`

var errHelp = errors.New("help")

func positiveSeconds(s string) (time.Duration, error) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v <= 0 || v > 2147483 {
		return 0, fmt.Errorf("interval must be finite and between 0 (exclusive) and 2147483 seconds")
	}
	return time.Duration(max(1, math.RoundToEven(v*1000))) * time.Millisecond, nil
}

// Accept options before or after the directory, just like argparse.
func parseArgs(args []string) (options, error) {
	o := options{background: "black", interval: 5 * time.Second}
	positional := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !positional && a == "--" {
			positional = true
			continue
		}
		if !positional && strings.HasPrefix(a, "-") {
			if strings.HasPrefix(a, "-i") && !strings.HasPrefix(a, "-i=") && len(a) > 2 {
				a = "-i=" + a[2:]
			}
			key, value, hasValue := strings.Cut(a, "=")
			switch key {
			case "-h", "--help":
				return o, errHelp
			case "--recursive", "--windowed":
				if hasValue {
					return o, fmt.Errorf("%s does not take a value", key)
				}
				if key == "--recursive" {
					o.recursive = true
				} else {
					o.windowed = true
				}
			case "-i", "--interval", "--background", "--seed":
				if !hasValue {
					i++
					if i >= len(args) {
						return o, fmt.Errorf("%s needs a value", key)
					}
					value = args[i]
				}
				switch key {
				case "-i", "--interval":
					var err error
					o.interval, err = positiveSeconds(value)
					if err != nil {
						return o, err
					}
				case "--background":
					o.background = value
				case "--seed":
					v := value
					o.seed = &v
				}
			default:
				return o, fmt.Errorf("unknown option: %s", a)
			}
		} else {
			if o.directory != "" {
				return o, fmt.Errorf("expected one image directory")
			}
			o.directory = a
		}
	}
	if o.directory == "" {
		return o, fmt.Errorf("an image directory is required")
	}
	return o, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	o, err := parseArgs(args)
	if errors.Is(err, errHelp) {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprint(stderr, usage)
		return 2
	}
	files, err := discover(o.directory, o.recursive)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 2
	}
	bg, err := parseColor(o.background)
	if err != nil {
		fmt.Fprintln(stderr, "Cannot open slideshow:", err)
		return 1
	}
	w, err := newDesktop()
	if err != nil {
		fmt.Fprintln(stderr, "Cannot open slideshow:", err, "Check the desktop/DISPLAY.")
		return 1
	}
	defer w.Close()
	a := newSlideshow(w, files, o, bg, stderr)
	a.nextImage(time.Now())
	for !a.closed {
		events, err := w.Poll()
		if err != nil {
			fmt.Fprintln(stderr, "Desktop error:", err)
			return 1
		}
		for _, e := range events {
			a.handle(e, time.Now())
		}
		a.tick(time.Now())
		time.Sleep(5 * time.Millisecond)
	}
	return a.exitCode
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

// Retain opaque backgrounds; transparent photos are composited onto this color.
func opaque(r, g, b uint8) color.NRGBA { return color.NRGBA{r, g, b, 255} }
