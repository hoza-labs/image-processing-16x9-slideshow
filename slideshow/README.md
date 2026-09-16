# Memorial photo slideshow

Python/Tkinter slideshow following [the plan](../slideshow-plan.md). Photos are
read directly without modifying or copying them. Pillow, Python, Tkinter and
the desktop tools install inside the container only.

## Run in the dev container

1. Start Docker Desktop with Linux containers. Open the **repository root** in
   VS Code with the Dev Containers extension, then run **Dev Containers: Reopen
   in Container**. The first build downloads the runtime and desktop dependencies.
2. In VS Code's Ports panel, open **Slideshow desktop**, port **6080**, in your
   browser. Connect using password `vscode`. Keep the forwarded port private.
3. In the container terminal (which starts in `/workspaces/memorial/slideshow`):

   ```sh
   python slideshow.py "../16x9 a-v final" --interval 7
   ```

   The repository, including its photo directories, is mounted into the container.
   Use Linux paths here; another example is `"../16x9 a-v final - random names"`.

4. The Tkinter window fills the **container's virtual desktop**. For presentation,
   move the browser onto the event display, enable noVNC's full-screen control,
   and select remote resizing in noVNC's scaling settings so the virtual desktop
   matches the display. Browser full screen requires a user action. Confirm the
   complete image is visible on the actual display before the event.

The desktop uses the official
[desktop-lite dev-container feature](https://github.com/devcontainers/features/tree/main/src/desktop-lite).
Its virtual desktop makes the native Tkinter GUI accessible on Windows without
installing Python or an X server on the host.

## Options and controls

```sh
python slideshow.py "../16x9 a-v final" --interval 2.5 --recursive --seed rehearsal --windowed
python slideshow.py --help
```

The interval defaults to 5 seconds; `--background COLOR` defaults to black and
accepts Tk colors such as `"#202020"`. Supported extensions, case-insensitively:
JPG, JPEG, PNG, WEBP, GIF, BMP, TIF, TIFF. Animated and multipage files show their
first frame. Orientation is corrected from EXIF; images fit without cropping or
stretching and are centered on the chosen background.

| Control | Action |
| --- | --- |
| Esc / Q | Quit |
| Space | Pause / resume |
| Right / N / click | Next |
| Left / P | Previous from session history |
| F / F11 | Toggle application full screen |

Manual navigation restarts the timer; navigation while paused stays paused.
Resume gives the current image a full interval. Going forward after going back
replays history before drawing another random image. Each shuffled cycle visits
all files once and avoids repeating the boundary image; two images necessarily
alternate. Unreadable files are reported to stderr and skipped for the remainder
of the session. If none are readable, the program exits with status 1. History
stores paths only; decoded images are not accumulated in memory.

## Verification

Inside the dev container:

```sh
python -m unittest discover -s tests -v
```

For an isolated headless test run from the repository root, without opening VS Code:

```sh
docker build -t memorial-slideshow-test ./slideshow
docker run --rm --init --mount "type=bind,source=${PWD},target=/workspaces/memorial,readonly" memorial-slideshow-test xvfb-run -a python -B -m unittest discover -s tests -v
```

Tests generate temporary fixtures, covering shapes, a large image, uppercase
extensions, EXIF rotation, invalid files, paths with spaces, shuffled cycles,
history, controls, resizing, and integer/decimal timers. They do not alter photos.
Before the memorial, rehearse with the intended directory on the actual display,
keep photos locally available, and disable sleep, notifications and automatic
screen locking for the event. Container/remote-desktop full screen does not
control the host's sleep or browser settings.
