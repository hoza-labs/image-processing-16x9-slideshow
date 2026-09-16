# Full-screen random image slideshow plan

## Recommendation

Build a small Python program using Tkinter for the window and Pillow for image
loading and resizing. This is the shortest dependable route to a real
full-screen slideshow. A browser version would be visually simple, but browsers
normally require a click before entering full screen and need either a generated
file list or a local web server. Go would produce a convenient single binary,
but displaying and resizing images in a native full-screen window would require
more code or a GUI dependency.

The only non-standard dependency is Pillow:

```sh
python -m pip install Pillow
```

## Command-line interface

Create `slideshow.py` and run it like this:

```sh
python slideshow.py "C:\path\to\photos" --interval 7
```

Proposed options:

```text
positional:
  directory              Directory containing images

options:
  -i, --interval SECONDS  Time each image remains visible (default: 5)
  --background COLOR      Letterbox/pillarbox color (default: black)
  --recursive             Include images in subdirectories
  --seed VALUE            Reproduce a particular random order for testing
  --windowed              Start in a normal window instead of full screen
```

The interval should accept decimals, such as `--interval 2.5`, and reject zero
or negative values with a clear error.

## Required behavior

1. Resolve and validate the supplied directory.
2. Find supported files case-insensitively: `.jpg`, `.jpeg`, `.png`, `.webp`,
   `.gif`, `.bmp`, and `.tif`/`.tiff`.
3. Shuffle the complete file list once with `random.shuffle`. Show every image
   exactly once before starting a newly shuffled cycle. If there is more than
   one image, prevent the last image of one cycle from also being first in the
   next cycle.
4. Create a borderless black Tkinter window and enable full screen with
   `attributes("-fullscreen", True)`.
5. Read the monitor's current dimensions whenever an image is displayed. Use
   Pillow's `ImageOps.contain` (or `thumbnail`) to preserve aspect ratio and fit
   the whole image inside the available screen. Never crop or stretch it.
6. Center the resized image on the background. Portrait and unusually wide
   images will have black bars rather than losing content.
7. Correct phone-camera orientation using EXIF transpose before resizing.
8. Schedule transitions with Tkinter's `after`, keeping all GUI work on its main
   thread. Retain the active `PhotoImage` reference so it is not garbage
   collected.
9. If a file cannot be decoded, report its path to stderr and continue instead
   of ending the slideshow.

## Controls

- `Esc` or `Q`: quit
- `Space`: pause/resume
- `Right`, `N`, or mouse click: next image
- `Left` or `P`: previous image from the current session history
- `F` or `F11`: toggle full screen

Manual navigation should restart the transition timer so a newly selected image
gets the full configured display time. Keep the mouse pointer hidden while full
screen and reveal it in windowed mode.

## Implementation shape

Keep this as one file with three small responsibilities:

- Argument parsing and image discovery
- A shuffle-bag iterator that supplies random, non-repeating cycles
- A `Slideshow` Tkinter class that owns display state, keyboard bindings, image
  resizing, and timers

Do not preprocess, rename, or copy the source images. Decode only the next image
when needed so a large photo directory does not consume excessive memory. An
optional one-image look-ahead cache can be added later if transitions feel slow.

## Verification

Test with a temporary directory containing landscape, portrait, square, very
large, uppercase-extension, EXIF-rotated, and deliberately invalid image files.
Confirm that:

- every valid image appears once before any repeats;
- the ordering changes between cycles;
- no image is cropped or distorted at the monitor edges;
- pause, navigation, full-screen toggle, and quit work;
- invalid files do not stop playback;
- paths containing spaces work;
- the timer closely follows both integer and decimal intervals.

Finally, run it against the intended memorial image directory on the actual
display. Keep the photos locally available and disable sleep, notifications,
and automatic screen locking for the event.
