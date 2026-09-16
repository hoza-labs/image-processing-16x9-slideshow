import argparse
from contextlib import redirect_stderr
import io
from pathlib import Path
import tempfile
import time
import tkinter as tk
import unittest

from PIL import Image
from slideshow import Slideshow, ShuffleBag, discover, load_image, positive_seconds


class ImageFixtures:
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="photos with spaces ")
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        for name, size in [("landscape.JPG", (600, 300)), ("portrait.png", (100, 300)),
                           ("square.bmp", (100, 100)), ("large.tiff", (4000, 2500))]:
            Image.new("RGB", size, "red").save(self.directory / name)
        exif = Image.Exif()
        exif[274] = 6
        Image.new("RGB", (60, 30), "blue").save(self.directory / "rotated.jpg", exif=exif)
        (self.directory / "invalid.gif").write_text("not an image")
        (self.directory / "ignored.txt").write_text("ignore")


class Images(ImageFixtures, unittest.TestCase):
    def test_discovery(self):
        self.assertEqual(len(discover(self.directory)), 6)
        nested = self.directory / "nested"
        nested.mkdir()
        Image.new("RGB", (10, 10)).save(nested / "extra.WEBP")
        self.assertEqual(len(discover(self.directory)), 6)
        self.assertEqual(len(discover(self.directory, True)), 7)
        with self.assertRaises(ValueError):
            discover(self.directory / "missing")

    def test_contain_and_orientation(self):
        for name, expected in [("landscape.JPG", (200, 100)), ("portrait.png", (67, 200)),
                               ("square.bmp", (200, 200)), ("large.tiff", (200, 125)),
                               ("rotated.jpg", (100, 200))]:
            with self.subTest(name=name):
                self.assertEqual(load_image(self.directory / name, (200, 200)).size, expected)


class Ordering(unittest.TestCase):
    def test_cycles_and_seed(self):
        bag = ShuffleBag(range(8), "memorial")
        values = [next(bag) for _ in range(80)]
        cycles = [values[i:i + 8] for i in range(0, 80, 8)]
        for cycle in cycles:
            self.assertEqual(sorted(cycle), list(range(8)))
        for previous, current in zip(cycles, cycles[1:]):
            self.assertNotEqual(previous[-1], current[0])
        self.assertGreater(len({tuple(c) for c in cycles}), 1)
        same = ShuffleBag(range(8), "memorial")
        self.assertEqual(values, [next(same) for _ in range(80)])

    def test_small_bags(self):
        bag = ShuffleBag([1])
        self.assertEqual([next(bag) for _ in range(5)], [1] * 5)
        bag = ShuffleBag([1, 2], 4)
        values = [next(bag) for _ in range(20)]
        self.assertTrue(all(a != b for a, b in zip(values, values[1:])))

    def test_intervals(self):
        self.assertEqual(positive_seconds("2.5"), 2.5)
        self.assertEqual(positive_seconds("7"), 7)
        for value in ["0", "-1", "nan", "inf", "text", "1e100"]:
            with self.subTest(value=value), self.assertRaises(argparse.ArgumentTypeError):
                positive_seconds(value)


class Desktop(ImageFixtures, unittest.TestCase):
    def start(self, files=None, interval=5):
        self.root = tk.Tk()
        self.app = Slideshow(self.root, files or discover(self.directory), interval=interval,
                             windowed=True, seed="test")
        self.addCleanup(lambda: self.app.close() if not self.app.closed else None)
        if not self.app.closed:
            self.root.update()
        return self.app

    def pump(self, seconds):
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline and not self.app.closed:
            self.root.update()
            time.sleep(.005)

    def test_valid_cycle_and_invalid_skipping(self):
        error = io.StringIO()
        with redirect_stderr(error):
            app = self.start()
            for _ in range(4):
                app.next_image()
            self.assertEqual(len(set(app.history)), 5)
            app.next_image()
        self.assertIn("invalid.gif", error.getvalue())
        self.assertFalse(app.closed)

    def test_navigation_pause_resize_and_bindings(self):
        app = self.start([self.directory / "landscape.JPG", self.directory / "portrait.png"])
        first = app.current_path
        app.next_image()
        second = app.current_path
        app.previous_image()
        self.assertEqual(app.current_path, first)
        app.next_image()
        self.assertEqual(app.current_path, second)
        self.assertEqual(len(app.history), 2)
        app.toggle_pause()
        self.assertIsNone(app.timer)
        app.next_image()
        self.assertIsNone(app.timer)
        app.toggle_pause()
        self.assertIsNotNone(app.timer)
        app.toggle_fullscreen()
        self.root.update()
        self.assertTrue(app.fullscreen)
        self.assertEqual(self.root.cget("cursor"), "none")
        app.toggle_fullscreen()
        self.root.geometry("640x480")
        self.pump(.2)
        self.assertLessEqual(app.photo.width(), app.canvas.winfo_width())
        self.assertLessEqual(app.photo.height(), app.canvas.winfo_height())
        for key in ["<Escape>", "q", "Q", "<space>", "<Right>", "n", "N", "<Button-1>",
                    "<Left>", "p", "P", "f", "F", "<F11>"]:
            self.assertTrue(self.root.bind(key))
        self.root.focus_force()
        self.root.event_generate("<Escape>")
        self.assertTrue(app.closed)

    def test_timers(self):
        for interval in [.15, 1]:
            with self.subTest(interval=interval):
                app = self.start([self.directory / "landscape.JPG"], interval)
                app.toggle_pause()
                count = len(app.history)
                self.pump(interval + .05)
                self.assertEqual(len(app.history), count)
                app.toggle_pause()
                self.pump(interval * .6)
                app.next_image()
                count = len(app.history)
                self.pump(interval * .6)
                self.assertEqual(len(app.history), count)
                self.pump(interval * .5)
                self.assertEqual(len(app.history), count + 1)
                app.close()

    def test_all_invalid_exits(self):
        with redirect_stderr(io.StringIO()) as error:
            app = self.start([self.directory / "invalid.gif"])
        self.assertTrue(app.closed)
        self.assertEqual(app.exit_code, 1)
        self.assertIn("No readable images", error.getvalue())


if __name__ == "__main__":
    unittest.main()
