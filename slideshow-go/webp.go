package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"io"

	"golang.org/x/image/webp"
)

func uint24(b []byte) int { return int(b[0]) | int(b[1])<<8 | int(b[2])<<16 }

// x/image decodes static WebP; unwrap the first ANMF chunk for animations.
// All chunk payloads are read through bounded sections, without buffering the file.
func decodeWebPFirst(r io.ReaderAt, config image.Config) (image.Image, error) {
	var header [12]byte
	if _, err := r.ReadAt(header[:], 0); err != nil {
		return nil, err
	}
	end := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
	for p := int64(12); p+8 <= end; {
		var chunk [8]byte
		if _, err := r.ReadAt(chunk[:], p); err != nil {
			return nil, err
		}
		n := int64(binary.LittleEndian.Uint32(chunk[4:]))
		if p+8+n > end {
			return nil, fmt.Errorf("invalid WebP chunk length")
		}
		switch string(chunk[:4]) {
		case "VP8 ", "VP8L":
			return webp.Decode(io.NewSectionReader(r, 0, end))
		case "ANMF":
			if n < 16 {
				return nil, fmt.Errorf("invalid WebP frame")
			}
			var frame [16]byte
			if _, err := r.ReadAt(frame[:], p+8); err != nil {
				return nil, err
			}
			x, y := uint24(frame[:3])*2, uint24(frame[3:6])*2
			w, h := uint24(frame[6:9])+1, uint24(frame[9:12])+1
			if x+w > config.Width || y+h > config.Height {
				return nil, fmt.Errorf("WebP frame outside canvas")
			}
			// Supply VP8X for ALPH+VP8 frames. Lossless VP8L carries its own alpha.
			var first [4]byte
			if _, err := r.ReadAt(first[:], p+24); err != nil {
				return nil, err
			}
			var extra []byte
			if string(first[:]) == "ALPH" {
				extra = make([]byte, 18)
				copy(extra, "VP8X")
				binary.LittleEndian.PutUint32(extra[4:], 10)
				extra[8] = 16
				copy(extra[12:15], frame[6:9])
				copy(extra[15:18], frame[9:12])
			}
			prefix := make([]byte, 12)
			copy(prefix, "RIFF")
			binary.LittleEndian.PutUint32(prefix[4:], uint32(4+len(extra))+uint32(n-16))
			copy(prefix[8:], "WEBP")
			reader := io.MultiReader(bytes.NewReader(prefix), bytes.NewReader(extra), io.NewSectionReader(r, p+24, n-16))
			// Validate the inner bitstream's dimensions before it can allocate pixels.
			inner, err := webp.DecodeConfig(reader)
			if err != nil {
				return nil, err
			}
			if inner.Width != w || inner.Height != h {
				return nil, fmt.Errorf("WebP frame dimensions disagree")
			}
			reader = io.MultiReader(bytes.NewReader(prefix), bytes.NewReader(extra), io.NewSectionReader(r, p+24, n-16))
			im, err := webp.Decode(reader)
			if err != nil {
				return nil, err
			}
			canvas := image.NewNRGBA(image.Rect(0, 0, config.Width, config.Height))
			draw.Draw(canvas, image.Rect(x, y, x+w, y+h), im, im.Bounds().Min, draw.Src)
			return canvas, nil
		}
		p += 8 + n + n%2
	}
	return nil, fmt.Errorf("WebP contains no readable frame")
}
