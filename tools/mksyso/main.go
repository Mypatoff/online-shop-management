// Command mksyso writes a Go linker object (.syso) that adds Windows resources to an
// executable: an application manifest and an icon. It replaces the third-party "rsrc" tool
// with the few lines of COFF this project needs, using only the standard library.
//
//	go run ./tools/mksyso -manifest cmd/server/app.manifest -ico assets/icon.ico -o cmd/server/rsrc_windows_amd64.syso
//
// go build links any *_windows_amd64.syso file in the package directory automatically.
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
)

func main() {
	manifest := flag.String("manifest", "", "application manifest (XML)")
	ico := flag.String("ico", "", "icon (.ico)")
	out := flag.String("o", "", "output .syso")
	flag.Parse()
	if *out == "" || (*manifest == "" && *ico == "") {
		flag.Usage()
		os.Exit(2)
	}

	var res []resource
	if *manifest != "" {
		data, err := os.ReadFile(*manifest)
		if err != nil {
			fatal(err)
		}
		res = append(res, resource{typ: rtManifest, id: 1, data: data})
	}
	if *ico != "" {
		data, err := os.ReadFile(*ico)
		if err != nil {
			fatal(err)
		}
		icons, err := iconResources(data)
		if err != nil {
			fatal(err)
		}
		res = append(res, icons...)
	}
	obj, err := build(res)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*out, obj, 0o644); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "mksyso:", err)
	os.Exit(1)
}

// Resource types (winuser.h).
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtManifest  = 24
)

type resource struct {
	typ, id uint16
	data    []byte
}

// iconResources turns an .ico file into one RT_ICON resource per image plus an RT_GROUP_ICON
// resource that lists them. The images are stored as they are in the file (PNG or bitmap).
func iconResources(ico []byte) ([]resource, error) {
	if len(ico) < 6 || binary.LittleEndian.Uint16(ico[0:]) != 0 || binary.LittleEndian.Uint16(ico[2:]) != 1 {
		return nil, errors.New("not an .ico file")
	}
	n := int(binary.LittleEndian.Uint16(ico[4:]))
	if n == 0 || len(ico) < 6+16*n {
		return nil, errors.New("empty or truncated .ico file")
	}
	var res []resource
	var group bytes.Buffer
	binary.Write(&group, binary.LittleEndian, [3]uint16{0, 1, uint16(n)})
	for i := 0; i < n; i++ {
		e := ico[6+16*i : 6+16*(i+1)]
		size := binary.LittleEndian.Uint32(e[8:])
		off := binary.LittleEndian.Uint32(e[12:])
		if uint64(off)+uint64(size) > uint64(len(ico)) {
			return nil, fmt.Errorf("icon image %d is outside the file", i)
		}
		id := uint16(i + 1)
		res = append(res, resource{typ: rtIcon, id: id, data: ico[off : off+size]})
		// GRPICONDIRENTRY: the first 12 bytes of the .ico entry, then the resource id.
		group.Write(e[:12])
		binary.Write(&group, binary.LittleEndian, id)
	}
	res = append(res, resource{typ: rtGroupIcon, id: 1, data: group.Bytes()})
	return res, nil
}

const (
	machineAMD64     = 0x8664
	relAMD64Addr32NB = 3 // IMAGE_REL_AMD64_ADDR32NB: 32-bit address without image base
	langEnglishUS    = 0x0409
)

// build lays out the .rsrc section (a three-level directory: type, id, language), the one
// symbol and one relocation per data entry that the linker needs to place it, and wraps it in
// a COFF object.
func build(res []resource) ([]byte, error) {
	// Group by type, then id, in the ascending order the PE format requires.
	sort.Slice(res, func(i, j int) bool {
		if res[i].typ != res[j].typ {
			return res[i].typ < res[j].typ
		}
		return res[i].id < res[j].id
	})
	var types []uint16
	byType := map[uint16][]resource{}
	for _, r := range res {
		if len(byType[r.typ]) == 0 {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], r)
	}

	const dirSize, entrySize, dataEntrySize = 16, 8, 16

	// Sizes: root dir, one id-dir per type, one language dir per resource, data entries, blobs.
	rootSize := dirSize + entrySize*len(types)
	idDirsSize := 0
	for _, t := range types {
		idDirsSize += dirSize + entrySize*len(byType[t])
	}
	langDirsSize := len(res) * (dirSize + entrySize)
	dataEntriesOff := rootSize + idDirsSize + langDirsSize
	blobsOff := dataEntriesOff + len(res)*dataEntrySize

	var sec bytes.Buffer
	le := binary.LittleEndian
	put := func(v any) { binary.Write(&sec, le, v) }
	dir := func(entries int) {
		put(uint32(0))       // characteristics
		put(uint32(0))       // timestamp
		put(uint16(0))       // major
		put(uint16(0))       // minor
		put(uint16(0))       // named entries
		put(uint16(entries)) // id entries
	}

	// Root directory: types.
	dir(len(types))
	idDirOff := rootSize
	idDirOffs := map[uint16]int{}
	for _, t := range types {
		put(uint32(t))
		put(uint32(0x80000000 | uint32(idDirOff)))
		idDirOffs[t] = idDirOff
		idDirOff += dirSize + entrySize*len(byType[t])
	}
	// Id directories: one per type, listing its resources.
	langDirOff := rootSize + idDirsSize
	langOffs := map[*resource]int{}
	for _, t := range types {
		dir(len(byType[t]))
		for i := range byType[t] {
			r := &byType[t][i]
			put(uint32(r.id))
			put(uint32(0x80000000 | uint32(langDirOff)))
			langOffs[r] = langDirOff
			langDirOff += dirSize + entrySize
		}
	}
	// Language directories: one entry each, pointing at the data entry.
	k := 0
	order := make([]*resource, 0, len(res))
	for _, t := range types {
		for i := range byType[t] {
			order = append(order, &byType[t][i])
		}
	}
	for _, r := range order {
		if sec.Len() != langOffs[r] {
			return nil, fmt.Errorf("internal: language directory misplaced (%d != %d)", sec.Len(), langOffs[r])
		}
		dir(1)
		put(uint32(langEnglishUS))
		put(uint32(dataEntriesOff + k*dataEntrySize))
		k++
	}
	// Data entries (their OffsetToData is relocated by the linker) and the blobs they describe.
	var relocOffsets []uint32
	blobOff := blobsOff
	var blobs bytes.Buffer
	for _, r := range order {
		relocOffsets = append(relocOffsets, uint32(sec.Len()))
		put(uint32(blobOff)) // section-relative; the relocation adds the section's address
		put(uint32(len(r.data)))
		put(uint32(0)) // code page
		put(uint32(0)) // reserved
		blobs.Write(r.data)
		pad := (4 - len(r.data)%4) % 4
		blobs.Write(make([]byte, pad))
		blobOff += len(r.data) + pad
	}
	if sec.Len() != blobsOff {
		return nil, fmt.Errorf("internal: blobs misplaced (%d != %d)", sec.Len(), blobsOff)
	}
	sec.Write(blobs.Bytes())

	// COFF object: file header, section header, section data, relocations, symbol, strings.
	const fileHdr, sectHdr = 20, 40
	dataOff := fileHdr + sectHdr
	relocTableOff := dataOff + sec.Len()
	symTableOff := relocTableOff + 10*len(relocOffsets)

	var obj bytes.Buffer
	w := func(v any) { binary.Write(&obj, le, v) }
	w(uint16(machineAMD64))
	w(uint16(1))           // sections
	w(uint32(0))           // timestamp
	w(uint32(symTableOff)) // symbol table
	w(uint32(1))           // symbols
	w(uint16(0))           // optional header
	w(uint16(0))           // characteristics

	name := [8]byte{'.', 'r', 's', 'r', 'c'}
	obj.Write(name[:])
	w(uint32(0))                 // virtual size
	w(uint32(0))                 // virtual address
	w(uint32(sec.Len()))         // size of raw data
	w(uint32(dataOff))           // pointer to raw data
	w(uint32(relocTableOff))     // pointer to relocations
	w(uint32(0))                 // pointer to line numbers
	w(uint16(len(relocOffsets))) // relocations
	w(uint16(0))                 // line numbers
	w(uint32(0x40000040))        // initialized data, readable
	obj.Write(sec.Bytes())

	for _, off := range relocOffsets {
		w(off)       // address of the OffsetToData field
		w(uint32(0)) // symbol index (the section symbol)
		w(uint16(relAMD64Addr32NB))
	}

	// Symbol: ".rsrc", at the start of section 1, static.
	obj.Write(name[:])
	w(uint32(0))
	w(uint16(1))
	w(uint16(0))
	w(uint8(3)) // IMAGE_SYM_CLASS_STATIC
	w(uint8(0))
	w(uint32(4)) // empty string table
	return obj.Bytes(), nil
}
