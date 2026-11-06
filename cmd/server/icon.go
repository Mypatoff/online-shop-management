package main

import "encoding/binary"

// icoImage returns the image data (PNG) of the entry in an .ico file whose width is the
// smallest one that is at least want pixels, or the largest if none is. nil if the file is
// malformed.
func icoImage(ico []byte, want int) []byte {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil
	}
	count := int(binary.LittleEndian.Uint16(ico[4:]))
	bestW, bestOff, bestSize := 0, 0, 0
	for i := 0; i < count; i++ {
		e := 6 + i*16
		if e+16 > len(ico) {
			break
		}
		w := int(ico[e])
		if w == 0 {
			w = 256
		}
		size := int(binary.LittleEndian.Uint32(ico[e+8:]))
		off := int(binary.LittleEndian.Uint32(ico[e+12:]))
		if off < 0 || size <= 0 || off+size > len(ico) {
			continue
		}
		better := bestW == 0 ||
			(bestW < want && w > bestW) || // still too small: bigger is better
			(w >= want && bestW >= want && w < bestW) // big enough: the smallest such
		if better {
			bestW, bestOff, bestSize = w, off, size
		}
	}
	if bestW == 0 {
		return nil
	}
	return ico[bestOff : bestOff+bestSize]
}
