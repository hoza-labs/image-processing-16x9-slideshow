# Go photo slideshow

A native Go implementation of [slideshow](../slideshow), with standalone Windows
Linux, and macOS executables. Photos are read in place and never modified.

## Build

Install Go 1.24 or newer and run from Git Bash on Windows or Bash on Linux/macOS:

```bash
bash build-slideshow-go.sh
```

The script lives at the repository root, works from any working directory, runs
the tests on the build host, and cross-compiles all three outputs:

```text
slideshow-go/dist/slideshow-windows-amd64.exe
slideshow-go/dist/slideshow-linux-amd64
slideshow-go/dist/slideshow-darwin-amd64
```

Dependencies download on the first build. There is no C compiler requirement.
`CGO_ENABLED=0` embeds the Go runtime, image decoders, resampler, and color table.
Linux has no dynamic loader or shared-library dependencies. Windows calls the
built-in Windows system DLLs; no extra DLLs or language runtimes are needed.
Copy the executable to the target machine, along with the redistribution notices
in `dist/LICENSE` and `dist/THIRD_PARTY_NOTICES.txt`. Build outputs are git-ignored.

The default architecture is x86-64. For ARM64 targets:

```bash
GOARCH=arm64 bash build-slideshow-go.sh
```

Linux needs an active **X11 desktop with a window manager**, or XWayland on a
Wayland desktop, with `DISPLAY` and X authorization set normally. Windows uses
native Win32/GDI. A desktop is required; this is not a browser or terminal viewer.

macOS uses the X11 backend and requires [XQuartz](https://www.xquartz.org/).
Start XQuartz and run from its terminal with `DISPLAY` and X authorization set.
The `darwin-amd64` executable targets Intel Macs.

## Run

Windows (PowerShell):

```powershell
.\slideshow-go\dist\slideshow-windows-amd64.exe ".\16x9 a-v final" --interval 7
```

Linux (Bash):

```bash
./slideshow-go/dist/slideshow-linux-amd64 './16x9 a-v final' --interval 7
```

Options can appear before or after the directory:

```bash
./slideshow-go/dist/slideshow-linux-amd64 './photos' --interval 2.5 --recursive --seed rehearsal --windowed --background '#202020'
```

| Option | Behavior |
| --- | --- |
| `-i`, `--interval SECONDS` | Default 5; positive integer or decimal, at most 2147483 seconds |
| `--background COLOR` | Default black; X11/Tk names such as `Light Sky Blue`, or hex colors |
| `--recursive` | Include subdirectories |
| `--seed VALUE` | Reproduce a shuffle for the same sorted file list in this Go version |
| `--windowed` | Start with a 1280×720 client area instead of full screen |
| `-h`, `--help` | Show usage without opening a window |

| Control | Action |
| --- | --- |
| Esc / Q | Quit |
| Space | Pause / resume |
| Right / N / left click | Next |
| Left / P | Previous from session history |
| F / F11 | Toggle full screen; hide the pointer in full screen |

JPG, JPEG, PNG, WEBP, GIF, BMP, TIF, and TIFF extensions are recognized without
regard to case. Images are EXIF-oriented, resized with Lanczos filtering, centered,
and fitted within the window and current monitor without cropping or stretching.
Transparent pixels blend into the background. Animated files and multipage TIFFs
show their first frame/page. Uncommon encodings unsupported by Go's image decoders
are reported and skipped, like corrupt images.

Every shuffled cycle visits each file once, avoiding a repeated image at cycle
boundaries. Two images alternate. Going forward after going back replays session
history. Manual navigation resets the timer, stays paused if paused, and resume
gives the current image a full interval. Paths, rather than decoded photos, are
retained in history. Corrupt/unreadable files are logged to stderr and skipped for
the session; no readable images exits with status 1. Invalid arguments or image
directories exit with status 2. The seed is deterministic within the Go version;
it does not reproduce Python's random-number sequence.

## Tests

```bash
cd slideshow-go
go test ./...
go vet ./...
```

The portable suite covers all behaviors tested in the Python project: discovery,
paths with spaces, uppercase extensions, landscape/portrait/square/large images,
EXIF orientation, corrupt-file skipping, cycles, seed reproducibility, history,
controls, pause/resume, resizing, closing, and decimal/integer timers. It uses an
injected clock to check exact timer boundaries without timing flakiness. Additional
tests cover transparency, named colors, all eight EXIF transforms, late PNG/WebP
metadata, animated GIF/WebP first frames, and files deleted during playback.

For real native-window tests, including OS keyboard/mouse events, window resize,
full-screen dimensions, history, drawing, and timers:

```bash
SLIDESHOW_GUI_TEST=1 go test -count=1 -timeout 60s ./...
```

In PowerShell, set `$env:SLIDESHOW_GUI_TEST='1'` before `go test`. The native test
briefly opens a window. On headless Linux, install Xvfb, xauth, Openbox, and binutils
and run `bash test-linux.sh`. Alternatively, from the repository root:

```bash
docker build -f slideshow-go/Dockerfile.test -t memorial-slideshow-go-test slideshow-go
docker run --rm memorial-slideshow-go-test
```

This runs the suite under Xvfb/Openbox, builds the Linux executable, and checks
that it has no ELF dynamic-loader segment. Docker is only a testing convenience;
it is not required to build or run the slideshow.

Rehearse on the intended presentation monitor. Full screen does not disable sleep,
notifications, or screen locking.
