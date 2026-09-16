package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"path/filepath"
	"testing"
)

func pngChunk(name string, data []byte) []byte {
	b := make([]byte, 12+len(data))
	binary.BigEndian.PutUint32(b, uint32(len(data)))
	copy(b[4:], name)
	copy(b[8:], data)
	binary.BigEndian.PutUint32(b[8+len(data):], crc32.ChecksumIEEE(b[4:8+len(data)]))
	return b
}
func riffChunk(name string, data []byte) []byte {
	b := make([]byte, 8+len(data)+len(data)%2)
	copy(b, name)
	binary.LittleEndian.PutUint32(b[4:], uint32(len(data)))
	copy(b[8:], data)
	return b
}
func webpContainer(chunks ...[]byte) []byte {
	b := []byte("RIFF\x00\x00\x00\x00WEBP")
	for _, c := range chunks {
		b = append(b, c...)
	}
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	return b
}

func TestPNGExifAfterLargeData(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, solid(60, 30, opaque(0, 0, 255))); err != nil {
		t.Fatal(err)
	}
	data := append([]byte(nil), b.Bytes()[:b.Len()-12]...)
	data = append(data, pngChunk("teSt", make([]byte, 5<<20))...)
	data = append(data, pngChunk("eXIf", orientationData(6))...)
	data = append(data, pngChunk("IEND", nil)...)
	path := filepath.Join(t.TempDir(), "exif.png")
	writeFixture(t, path, data)
	im, err := loadImage(path, image.Pt(200, 200))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Size() != image.Pt(100, 200) {
		t.Fatal(im.Bounds())
	}
}
func TestAnimatedWebPFirstFrame(t *testing.T) {
	static, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	// A 1x1 frame at (2,0) in a 3x2 animation canvas.
	extended := []byte{2, 0, 0, 0, 2, 0, 0, 1, 0, 0}
	frame := make([]byte, 16)
	frame[0] = 1
	frame[12] = 100
	frame = append(frame, static[12:]...)
	data := webpContainer(riffChunk("VP8X", extended), riffChunk("ANIM", make([]byte, 6)), riffChunk("ANMF", frame), riffChunk("ANMF", frame))
	path := filepath.Join(t.TempDir(), "animated.webp")
	writeFixture(t, path, data)
	im, err := loadImage(path, image.Pt(3, 2))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Size() != image.Pt(3, 2) || im.NRGBAAt(0, 0).A != 0 || im.NRGBAAt(2, 0).A != 255 {
		t.Fatal("first frame composition", im)
	}
}
func TestWebPExifAndMalformedChunks(t *testing.T) {
	data := webpContainer(riffChunk("JUNK", make([]byte, 5<<20)), riffChunk("EXIF", append([]byte("Exif\x00\x00"), orientationData(8)...)))
	if exifOrientation(data) != 8 {
		t.Fatal("WebP trailing EXIF")
	}
	for i := 0; i < 30; i++ {
		if got := exifOrientation(data[:i]); got != 1 {
			t.Fatalf("truncated metadata: %d", got)
		}
	}
}

func TestGIFFirstFrameCanvas(t *testing.T) {
	palette := color.Palette{color.Black, color.White}
	frame := image.NewPaletted(image.Rect(2, 1, 3, 2), palette)
	frame.SetColorIndex(2, 1, 1)
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{frame}, Delay: []int{0}, Config: image.Config{ColorModel: palette, Width: 4, Height: 3}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "offset.gif")
	writeFixture(t, path, b.Bytes())
	im, err := loadImage(path, image.Pt(4, 3))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Size() != image.Pt(4, 3) || im.NRGBAAt(2, 1).R != 255 || im.NRGBAAt(0, 0) != opaque(0, 0, 0) {
		t.Fatal("GIF logical canvas", im)
	}
}
