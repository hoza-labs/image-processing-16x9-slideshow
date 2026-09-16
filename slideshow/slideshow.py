"""A full-screen, shuffled photo slideshow. Run with --help for options."""

import argparse
import math
from pathlib import Path
import random
import sys
import tkinter as tk

from PIL import Image, ImageOps, ImageTk


EXTENSIONS = {".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp", ".tif", ".tiff"}


def positive_seconds(value):
    try:
        seconds = float(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError("interval must be a positive number") from exc
    if not math.isfinite(seconds) or seconds <= 0 or seconds > 2147483:
        raise argparse.ArgumentTypeError("interval must be finite and between 0 (exclusive) and 2147483 seconds")
    return seconds


def discover(directory, recursive=False):
    directory = Path(directory).expanduser().resolve()
    if not directory.is_dir():
        raise ValueError(f"Not a directory: {directory}")
    files = sorted(path for path in (directory.rglob("*") if recursive else directory.iterdir())
                   if path.is_file() and path.suffix.lower() in EXTENSIONS)
    if not files:
        raise ValueError(f"No supported images found in: {directory}")
    return files


class ShuffleBag:
    def __init__(self, files, seed=None):
        self.files = list(files)
        if not self.files:
            raise ValueError("A shuffle bag needs at least one image")
        self.random = random.Random(seed)
        self.pending = []
        self.last = None

    def __next__(self):
        if not self.pending:
            self.pending = self.files.copy()
            self.random.shuffle(self.pending)
            if len(self.pending) > 1 and self.pending[-1] == self.last:
                self.pending[0], self.pending[-1] = self.pending[-1], self.pending[0]
        self.last = self.pending.pop()
        return self.last


def load_image(path, size):
    with Image.open(path) as source:
        oriented = ImageOps.exif_transpose(source)
        # Animated/multipage files display their first frame only.
        return ImageOps.contain(oriented.convert("RGBA"), size, Image.Resampling.LANCZOS)


class Slideshow:
    def __init__(self, root, files, interval=5, background="black", seed=None, windowed=False):
        self.root = root
        self.bag = ShuffleBag(files, seed)
        self.interval_ms = max(1, round(interval * 1000))
        self.history = []
        self.position = -1
        self.bad_files = set()
        self.photo = None
        self.current_path = None
        self.paused = False
        self.fullscreen = not windowed
        self.timer = None
        self.resize_timer = None
        self.closed = False
        self.exit_code = 0
        root.title("Photo slideshow")
        root.geometry("1280x720")
        self.canvas = tk.Canvas(root, background=background, highlightthickness=0)
        self.canvas.pack(fill="both", expand=True)
        self.image_item = self.canvas.create_image(0, 0, anchor="center")
        self.apply_fullscreen()
        for key in ("<Escape>", "q", "Q"):
            root.bind(key, self.close)
        root.bind("<space>", self.toggle_pause)
        for key in ("<Right>", "n", "N", "<Button-1>"):
            root.bind(key, self.next_image)
        for key in ("<Left>", "p", "P"):
            root.bind(key, self.previous_image)
        for key in ("f", "F", "<F11>"):
            root.bind(key, self.toggle_fullscreen)
        self.canvas.bind("<Configure>", self.on_resize)
        root.protocol("WM_DELETE_WINDOW", self.close)
        root.update_idletasks()
        self.next_image()

    def apply_fullscreen(self):
        self.root.attributes("-fullscreen", self.fullscreen)
        self.root.configure(cursor="none" if self.fullscreen else "")

    def toggle_fullscreen(self, event=None):
        self.fullscreen = not self.fullscreen
        self.apply_fullscreen()

    def restart_timer(self):
        if self.timer is not None:
            self.root.after_cancel(self.timer)
            self.timer = None
        if not self.paused and not self.closed:
            self.timer = self.root.after(self.interval_ms, self.next_image)

    def toggle_pause(self, event=None):
        self.paused = not self.paused
        self.restart_timer()

    def display(self, path):
        # Query dimensions on every display; canvas dimensions also follow resizing.
        screen = (self.root.winfo_screenwidth(), self.root.winfo_screenheight())
        width = max(1, self.canvas.winfo_width())
        height = max(1, self.canvas.winfo_height())
        size = (min(width, screen[0]), min(height, screen[1]))
        try:
            resized = load_image(path, size)
        except (OSError, ValueError, SyntaxError, Image.DecompressionBombError) as exc:
            print(f"Skipping {path}: {exc}", file=sys.stderr)
            self.bad_files.add(path)
            return False
        self.photo = ImageTk.PhotoImage(resized, master=self.root)
        self.canvas.itemconfigure(self.image_item, image=self.photo)
        self.canvas.coords(self.image_item, width / 2, height / 2)
        self.current_path = path
        return True

    def next_image(self, event=None):
        while self.position + 1 < len(self.history):
            self.position += 1
            path = self.history[self.position]
            if path not in self.bad_files and self.display(path):
                self.restart_timer()
                return
        # Two bag lengths cover a partial cycle and a full cycle, even after failures.
        for _ in range(2 * len(self.bag.files)):
            path = next(self.bag)
            if path not in self.bad_files and self.display(path):
                self.history.append(path)
                self.position = len(self.history) - 1
                self.restart_timer()
                return
        print("No readable images remain; exiting.", file=sys.stderr)
        self.exit_code = 1
        self.close()

    def previous_image(self, event=None):
        original = self.position
        while self.position > 0:
            self.position -= 1
            path = self.history[self.position]
            if path not in self.bad_files and self.display(path):
                self.restart_timer()
                return
        self.position = original
        self.restart_timer()

    def on_resize(self, event=None):
        if self.resize_timer is not None:
            self.root.after_cancel(self.resize_timer)
        self.resize_timer = self.root.after(100, self.redraw)

    def redraw(self):
        self.resize_timer = None
        if self.current_path is not None and not self.display(self.current_path):
            self.next_image()

    def close(self, event=None):
        self.closed = True
        for timer in (self.timer, self.resize_timer):
            if timer is not None:
                self.root.after_cancel(timer)
        self.timer = self.resize_timer = None
        self.root.destroy()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", help="Directory containing images")
    parser.add_argument("-i", "--interval", type=positive_seconds, default=5, metavar="SECONDS")
    parser.add_argument("--background", default="black", metavar="COLOR")
    parser.add_argument("--recursive", action="store_true")
    parser.add_argument("--seed")
    parser.add_argument("--windowed", action="store_true")
    args = parser.parse_args(argv)
    try:
        files = discover(args.directory, args.recursive)
    except (ValueError, OSError) as exc:
        parser.error(str(exc))
    root = None
    try:
        root = tk.Tk()
        root.winfo_rgb(args.background)
        app = Slideshow(root, files, args.interval, args.background, args.seed, args.windowed)
        if not app.closed:
            root.mainloop()
        return app.exit_code
    except tk.TclError as exc:
        print(f"Cannot open slideshow: {exc}. Check the desktop/DISPLAY and background color.", file=sys.stderr)
        if root is not None:
            try:
                root.destroy()
            except tk.TclError:
                pass
        return 1


if __name__ == "__main__":
    sys.exit(main())
