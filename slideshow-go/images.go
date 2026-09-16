package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/disintegration/imaging"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

var extensions = map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true, ".bmp": true, ".tif": true, ".tiff": true}

func discover(directory string, recursive bool) ([]string, error) {
	if directory == "~" || strings.HasPrefix(directory, "~/") || strings.HasPrefix(directory, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		directory = filepath.Join(home, strings.TrimLeft(directory[1:], `/\`))
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Not a directory: %s", directory)
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return nil, err
	}
	var files []string
	err = filepath.WalkDir(directory, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != directory && !recursive {
				return filepath.SkipDir
			}
			return nil
		}
		if extensions[strings.ToLower(filepath.Ext(path))] {
			info, err := d.Info()
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				info, err = os.Stat(path)
				if err != nil {
					return nil
				}
			}
			if info.Mode().IsRegular() {
				files = append(files, path)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("No supported images found in: %s", directory)
	}
	return files, nil
}

func loadImage(path string, size image.Point) (*image.NRGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	config, format, err := image.DecodeConfig(f)
	if err != nil {
		return nil, err
	}
	// Match Pillow's decompression-bomb error limit before allocating pixels.
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 178956970 {
		return nil, fmt.Errorf("image dimensions exceed safe decoding limit")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, err
	}
	var source image.Image
	if format == "webp" {
		source, err = decodeWebPFirst(f, config)
	} else {
		source, _, err = image.Decode(f)
	}
	if err != nil {
		return nil, err
	}
	if format == "gif" && source.Bounds() != image.Rect(0, 0, config.Width, config.Height) {
		// GIF's first frame may occupy only part of its logical canvas.
		canvas := image.NewNRGBA(image.Rect(0, 0, config.Width, config.Height))
		if palette, ok := source.ColorModel().(color.Palette); ok {
			transparent := false
			for _, c := range palette {
				_, _, _, a := c.RGBA()
				if a == 0 {
					transparent = true
					break
				}
			}
			var header [13]byte
			if _, err := f.ReadAt(header[:], 0); err == nil && !transparent {
				if global, ok := config.ColorModel.(color.Palette); ok && int(header[11]) < len(global) {
					draw.Draw(canvas, canvas.Bounds(), image.NewUniform(global[header[11]]), image.Point{}, draw.Src)
				}
			}
		}
		draw.Draw(canvas, source.Bounds(), source, source.Bounds().Min, draw.Over)
		source = canvas
	}
	source = orient(source, readOrientation(f))
	w, h := source.Bounds().Dx(), source.Bounds().Dy()
	ratio := math.Min(float64(max(1, size.X))/float64(w), float64(max(1, size.Y))/float64(h))
	w = max(1, int(math.RoundToEven(float64(w)*ratio)))
	h = max(1, int(math.RoundToEven(float64(h)*ratio)))
	return imaging.Resize(source, w, h, imaging.Lanczos), nil
}

func orient(src image.Image, orientation int) image.Image {
	switch orientation {
	case 2:
		return imaging.FlipH(src)
	case 3:
		return imaging.Rotate180(src)
	case 4:
		return imaging.FlipV(src)
	case 5:
		return imaging.Transpose(src)
	case 6:
		return imaging.Rotate270(src)
	case 7:
		return imaging.Transverse(src)
	case 8:
		return imaging.Rotate90(src)
	}
	return src
}

func tiffOrientationAt(r io.ReaderAt) int {
	var header [8]byte
	if _, err := r.ReadAt(header[:], 0); err != nil {
		return 1
	}
	var order binary.ByteOrder
	switch string(header[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	if order.Uint16(header[2:4]) != 42 {
		return 1
	}
	offset := int64(order.Uint32(header[4:8]))
	var count [2]byte
	if _, err := r.ReadAt(count[:], offset); err != nil {
		return 1
	}
	var entry [12]byte
	for i := 0; i < int(order.Uint16(count[:])); i++ {
		if _, err := r.ReadAt(entry[:], offset+2+int64(i)*12); err != nil {
			return 1
		}
		if order.Uint16(entry[:2]) == 274 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
			v := int(order.Uint16(entry[8:10]))
			if v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}

// Seek past image data: EXIF can follow arbitrarily large PNG/WebP chunks.
func readOrientation(r io.ReaderAt) int {
	var header [12]byte
	n, _ := r.ReadAt(header[:], 0)
	if n < 8 {
		return 1
	}
	if string(header[:2]) == "II" || string(header[:2]) == "MM" {
		return tiffOrientationAt(r)
	}
	if header[0] == 0xff && header[1] == 0xd8 {
		for p := int64(2); ; {
			var h [4]byte
			if _, err := r.ReadAt(h[:], p); err != nil {
				return 1
			}
			if h[0] != 0xff || h[1] == 0xda || h[1] == 0xd9 {
				return 1
			}
			if h[1] == 0xff {
				p++
				continue
			}
			size := int64(binary.BigEndian.Uint16(h[2:]))
			if size < 2 {
				return 1
			}
			var magic [6]byte
			if h[1] == 0xe1 && size >= 8 {
				if _, err := r.ReadAt(magic[:], p+4); err == nil && string(magic[:]) == "Exif\x00\x00" {
					return tiffOrientationAt(io.NewSectionReader(r, p+10, size-8))
				}
			}
			p += 2 + size
		}
	}
	png := string(header[:8]) == "\x89PNG\r\n\x1a\n"
	webp := n == 12 && string(header[:4]) == "RIFF" && string(header[8:]) == "WEBP"
	if !png && !webp {
		return 1
	}
	p := int64(8)
	if webp {
		p = 12
	}
	for {
		var h [8]byte
		if _, err := r.ReadAt(h[:], p); err != nil {
			return 1
		}
		var size int64
		var name string
		if png {
			size = int64(binary.BigEndian.Uint32(h[:4]))
			name = string(h[4:])
		} else {
			size = int64(binary.LittleEndian.Uint32(h[4:]))
			name = string(h[:4])
		}
		if png && name == "eXIf" || webp && name == "EXIF" {
			offset := p + 8
			var magic [6]byte
			if webp && size >= 6 {
				if _, err := r.ReadAt(magic[:], offset); err == nil && string(magic[:]) == "Exif\x00\x00" {
					offset += 6
					size -= 6
				}
			}
			return tiffOrientationAt(io.NewSectionReader(r, offset, size))
		}
		if png && name == "IEND" {
			return 1
		}
		p += 8 + size
		if png {
			p += 4
		} else {
			p += size % 2
		}
	}
}
func exifOrientation(data []byte) int { return readOrientation(bytes.NewReader(data)) }
