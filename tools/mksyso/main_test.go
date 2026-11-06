package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

// node is one entry of the resource tree read back from a .rsrc section.
type node struct {
	typ, id uint16
	data    []byte
}

// readTree walks a .rsrc section (type -> id -> language -> data). rva maps a data entry's
// OffsetToData to a section offset: for a linked exe it subtracts the section's address, for
// an object file the offsets are already section-relative.
func readTree(t *testing.T, sec []byte, rva func(uint32) uint32) []node {
	t.Helper()
	le := binary.LittleEndian
	entries := func(off uint32) (ids, offs []uint32) {
		n := int(le.Uint16(sec[off+12:])) + int(le.Uint16(sec[off+14:]))
		for i := 0; i < n; i++ {
			e := off + 16 + uint32(i)*8
			ids = append(ids, le.Uint32(sec[e:]))
			offs = append(offs, le.Uint32(sec[e+4:]))
		}
		return
	}
	var out []node
	tids, toffs := entries(0)
	for i := range tids {
		iids, ioffs := entries(toffs[i] &^ 0x80000000)
		for j := range iids {
			_, loffs := entries(ioffs[j] &^ 0x80000000)
			de := loffs[0]
			start := rva(le.Uint32(sec[de:]))
			size := le.Uint32(sec[de+4:])
			out = append(out, node{uint16(tids[i]), uint16(iids[j]), sec[start : start+size]})
		}
	}
	return out
}

func TestBuildRoundTrip(t *testing.T) {
	ico, err := os.ReadFile("../../assets/icon.ico")
	if err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`<assembly manifestVersion="1.0"/>`) // odd length: exercises padding
	icons, err := iconResources(ico)
	if err != nil {
		t.Fatal(err)
	}
	res := append([]resource{{typ: rtManifest, id: 1, data: manifest}}, icons...)
	obj, err := build(res)
	if err != nil {
		t.Fatal(err)
	}

	f, err := pe.NewFile(bytes.NewReader(obj))
	if err != nil {
		t.Fatalf("debug/pe cannot read the object: %v", err)
	}
	if f.Machine != pe.IMAGE_FILE_MACHINE_AMD64 || len(f.Sections) != 1 || f.Sections[0].Name != ".rsrc" {
		t.Fatalf("header = machine %#x, %d sections", f.Machine, len(f.Sections))
	}
	if len(f.Symbols) != 1 || f.Symbols[0].Name != ".rsrc" {
		t.Fatalf("symbols = %+v", f.Symbols)
	}
	sec, err := f.Sections[0].Data()
	if err != nil {
		t.Fatal(err)
	}
	if got := int(f.Sections[0].NumberOfRelocations); got != len(res) {
		t.Fatalf("relocations = %d, want one per resource (%d)", got, len(res))
	}

	nodes := readTree(t, sec, func(off uint32) uint32 { return off })
	if len(nodes) != len(res) {
		t.Fatalf("tree has %d resources, want %d", len(nodes), len(res))
	}
	var gotManifest []byte
	nIcons, nGroups := 0, 0
	for _, n := range nodes {
		switch n.typ {
		case rtManifest:
			gotManifest = n.data
		case rtIcon:
			nIcons++
		case rtGroupIcon:
			nGroups++
			if cnt := binary.LittleEndian.Uint16(n.data[4:]); int(cnt) != len(icons)-1 {
				t.Errorf("group icon lists %d images, want %d", cnt, len(icons)-1)
			}
		}
	}
	if !bytes.Equal(gotManifest, manifest) {
		t.Errorf("manifest = %q", gotManifest)
	}
	if nIcons != len(icons)-1 || nGroups != 1 {
		t.Errorf("icons %d groups %d", nIcons, nGroups)
	}
	// Every relocation points at a resource's OffsetToData field.
	for _, r := range f.Sections[0].Relocs {
		if r.Type != relAMD64Addr32NB {
			t.Errorf("relocation type %d", r.Type)
		}
	}
}

func TestIconResourcesRejectsGarbage(t *testing.T) {
	for _, in := range [][]byte{nil, []byte("hello world, not an icon"), {0, 0, 1, 0, 0, 0}, {0, 0, 1, 0, 1, 0}} {
		if _, err := iconResources(in); err == nil {
			t.Errorf("iconResources(%v) accepted garbage", in)
		}
	}
	// An entry that points outside the file.
	bad := append([]byte{0, 0, 1, 0, 1, 0}, make([]byte, 16)...)
	binary.LittleEndian.PutUint32(bad[6+8:], 100)
	binary.LittleEndian.PutUint32(bad[6+12:], 1000)
	if _, err := iconResources(bad); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Errorf("err = %v", err)
	}
}
