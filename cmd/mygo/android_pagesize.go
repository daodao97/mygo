package main

import (
	"archive/zip"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Android 15 and later run on devices with 16 KB pages, and an app whose
// native libraries are laid out for 4 KB ones runs there in page size
// compatibility mode, with a warning on every launch; Google Play refuses
// it. Each library must be linked for 16 KB pages (its LOAD segments
// aligned to 16 KB), and one the APK stores uncompressed, as it does
// unless android.extractNativeLibs is set, must start on a 16 KB boundary
// of the APK, as Android maps it from there.

// androidPageSize is the largest page size of Android's devices.
const androidPageSize = 16384

// androidPageSizeLDFlag links a library for pages of up to 16 KB, which
// also load where pages are 4 KB.
const androidPageSizeLDFlag = "-Wl,-z,max-page-size=16384"

// androidPageSizeABIs are the ABIs of devices with 16 KB pages: 64-bit
// ones only.
var androidPageSizeABIs = []string{"arm64-v8a", "x86_64"}

// androidCgoLDFlags is CGO_LDFLAGS of the Go build for Android: the
// user's, then 16 KB pages, last so that the linker takes it over a
// max-page-size of the user's.
func androidCgoLDFlags(user string) string {
	return strings.TrimSpace(user + " " + androidPageSizeLDFlag)
}

// warnAPKPageSize warns of the native libraries of the APK at path that
// do not load as they are on devices with 16 KB pages.
func warnAPKPageSize(path string) {
	problems, err := apkPageSizeProblems(path)
	if err != nil {
		warnf("checking the APK's native libraries for 16 KB pages: %v", err)
		return
	}
	if len(problems) > 0 {
		warnf("The APK isn't 16 KB compatible: Android 15 and later run it in page size compatibility mode, and Google Play refuses it\n%s",
			strings.Join(problems, "\n"))
	}
}

// apkPageSizeProblems returns what keeps the native libraries of the APK
// at path from loading as they are on devices with 16 KB pages, as
// Android checks them when it installs the app: a LOAD segment aligned
// to less than 16 KB, or a library stored uncompressed at an offset that
// is not a multiple of 16 KB. Libraries of 32-bit ABIs, which no such
// device runs, are not checked.
func apkPageSizeProblems(path string) ([]string, error) {
	z, err := zip.OpenReader(path)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	var problems []string
	for _, f := range z.File {
		parts := strings.Split(f.Name, "/")
		if len(parts) != 3 || parts[0] != "lib" || !slices.Contains(androidPageSizeABIs, parts[1]) || !strings.HasSuffix(parts[2], ".so") {
			continue
		}
		if f.Method == zip.Store {
			off, err := f.DataOffset()
			if err != nil {
				return nil, fmt.Errorf("%s: %w", f.Name, err)
			}
			if off%androidPageSize != 0 {
				problems = append(problems, fmt.Sprintf("%s: stored uncompressed at offset %d of the APK, not on a 16 KB boundary (zipalign -P 16)", f.Name, off))
			}
		}
		r, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Name, err)
		}
		align, err := elfLoadAlign(r)
		r.Close()
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("%s: %v", f.Name, err))
		case align < androidPageSize:
			problems = append(problems, fmt.Sprintf("%s: a LOAD segment is aligned to %d bytes, not 16 KB (link it with %s)", f.Name, align, androidPageSizeLDFlag))
		}
	}
	return problems, nil
}

// elfLoadAlign returns the smallest alignment of the LOAD segments of the
// ELF file r reads, from its program headers, which follow its header at
// the start of the file: it reads no more of it.
func elfLoadAlign(r io.Reader) (uint64, error) {
	buf := make([]byte, 64) // the header of a 64-bit file, longer than a 32-bit one's
	if _, err := io.ReadFull(r, buf); err != nil || string(buf[:4]) != elf.ELFMAG {
		return 0, errors.New("not an ELF file")
	}
	var bo binary.ByteOrder = binary.LittleEndian
	if elf.Data(buf[elf.EI_DATA]) == elf.ELFDATA2MSB {
		bo = binary.BigEndian
	}
	is64 := elf.Class(buf[elf.EI_CLASS]) == elf.ELFCLASS64
	var phoff, phentsize, phnum, minEntry uint64
	if is64 {
		phoff, phentsize, phnum, minEntry = bo.Uint64(buf[0x20:]), uint64(bo.Uint16(buf[0x36:])), uint64(bo.Uint16(buf[0x38:])), 56
	} else {
		phoff, phentsize, phnum, minEntry = uint64(bo.Uint32(buf[0x1c:])), uint64(bo.Uint16(buf[0x2a:])), uint64(bo.Uint16(buf[0x2c:])), 32
	}
	end := phoff + phentsize*phnum
	if phentsize < minEntry || phoff < 52 || end > 1<<20 {
		return 0, errors.New("bad ELF program headers")
	}
	if end > uint64(len(buf)) {
		rest := make([]byte, end-uint64(len(buf)))
		if _, err := io.ReadFull(r, rest); err != nil {
			return 0, fmt.Errorf("reading ELF program headers: %w", err)
		}
		buf = append(buf, rest...)
	}
	align, loads := uint64(0), 0
	for i := range phnum {
		ph := buf[phoff+i*phentsize:]
		if elf.ProgType(bo.Uint32(ph)) != elf.PT_LOAD {
			continue
		}
		a := uint64(bo.Uint32(ph[0x1c:]))
		if is64 {
			a = bo.Uint64(ph[0x30:])
		}
		if loads == 0 || a < align {
			align = a
		}
		loads++
	}
	if loads == 0 {
		return 0, errors.New("no LOAD segment")
	}
	return align, nil
}
