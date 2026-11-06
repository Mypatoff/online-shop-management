package main

import (
	"bytes"
	"os"
	"testing"
)

func TestIcoImagePicksClosestSize(t *testing.T) {
	ico, err := os.ReadFile("icon_windows.ico")
	if err != nil {
		t.Fatal(err)
	}
	pngMagic := []byte("\x89PNG")
	for _, want := range []int{16, 20, 32, 40, 48, 256, 512} {
		data := icoImage(ico, want)
		if !bytes.HasPrefix(data, pngMagic) {
			t.Fatalf("icoImage(%d) is not PNG data", want)
		}
	}
	// 16 and 256 must be different images, and an oversize request falls back to the largest.
	if bytes.Equal(icoImage(ico, 16), icoImage(ico, 256)) {
		t.Error("16px and 256px requests returned the same image")
	}
	if !bytes.Equal(icoImage(ico, 512), icoImage(ico, 256)) {
		t.Error("oversize request should give the largest image")
	}
}

func TestIcoImageMalformed(t *testing.T) {
	for _, in := range [][]byte{nil, {0, 0, 1}, {0, 0, 2, 0, 1, 0}, {0, 0, 1, 0, 1, 0, 16, 16, 0, 0, 1, 0, 32, 0, 255, 255, 0, 0, 22, 0, 0, 0}} {
		if got := icoImage(in, 32); got != nil {
			t.Errorf("icoImage(%v) = %v, want nil", in, got)
		}
	}
}
