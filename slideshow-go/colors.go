package main

import (
	_ "embed"
	"fmt"
	"image/color"
	"strconv"
	"strings"
)

// X.Org rgb database, embedded so named backgrounds work without an installation.
// Source: https://gitlab.freedesktop.org/xorg/app/rgb
//
//go:embed rgb.txt
var rgbNames string

func parseColor(s string) (color.NRGBA, error) {
	if strings.HasPrefix(s, "#") {
		digits := len(s) - 1
		if digits == 3 || digits == 6 || digits == 9 || digits == 12 {
			n := digits / 3
			channels := [3]uint8{}
			for i := range channels {
				v, err := strconv.ParseUint(s[1+i*n:1+(i+1)*n], 16, 16)
				if err != nil {
					return color.NRGBA{}, fmt.Errorf("invalid background color: %q", s)
				}
				if n == 1 {
					v *= 17
				} else if n > 2 {
					v >>= uint((n - 2) * 4)
				}
				channels[i] = uint8(v)
			}
			return opaque(channels[0], channels[1], channels[2]), nil
		}
	}
	normalize := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }
	name := normalize(s)
	for _, line := range strings.Split(rgbNames, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || normalize(strings.Join(fields[3:], "")) != name {
			continue
		}
		r, e1 := strconv.Atoi(fields[0])
		g, e2 := strconv.Atoi(fields[1])
		b, e3 := strconv.Atoi(fields[2])
		if e1 == nil && e2 == nil && e3 == nil {
			return opaque(uint8(r), uint8(g), uint8(b)), nil
		}
	}
	return color.NRGBA{}, fmt.Errorf("invalid background color: %q", s)
}
