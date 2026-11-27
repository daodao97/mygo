package main

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	androidHost "github.com/egoist/mygo/internal/mobile/android"
)

// fakeELF is the start of a 64-bit little-endian ELF shared library with a
// LOAD segment of each alignment, and a segment of another type, which
// does not count, between them.
func fakeELF(aligns ...uint64) []byte {
	const phoff, phentsize = 64, 56
	phnum := len(aligns) + 1
	b := make([]byte, phoff+phentsize*phnum+256)
	copy(b, "\x7fELF")
	b[4], b[5], b[6] = 2, 1, 1 // ELFCLASS64, ELFDATA2LSB, EV_CURRENT
	le := binary.LittleEndian
	le.PutUint16(b[0x10:], 3) // ET_DYN
	le.PutUint64(b[0x20:], phoff)
	le.PutUint16(b[0x34:], 64)
	le.PutUint16(b[0x36:], phentsize)
	le.PutUint16(b[0x38:], uint16(phnum))
	ph := b[phoff:]
	for i, a := range aligns {
		if i == 1 {
			le.PutUint32(ph, 2) // PT_DYNAMIC, aligned to 8
			le.PutUint64(ph[0x30:], 8)
			ph = ph[phentsize:]
		}
		le.PutUint32(ph, 1) // PT_LOAD
		le.PutUint64(ph[0x30:], a)
		ph = ph[phentsize:]
	}
	if len(aligns) < 2 {
		le.PutUint32(ph, 0x6474e551) // PT_GNU_STACK
	}
	return b
}

// fakeELF32 is fakeELF's 32-bit twin, one LOAD segment.
func fakeELF32(align uint32) []byte {
	const phoff, phentsize = 52, 32
	b := make([]byte, phoff+phentsize+64)
	copy(b, "\x7fELF")
	b[4], b[5], b[6] = 1, 1, 1
	le := binary.LittleEndian
	le.PutUint32(b[0x1c:], phoff)
	le.PutUint16(b[0x2a:], phentsize)
	le.PutUint16(b[0x2c:], 1)
	le.PutUint32(b[phoff:], 1)
	le.PutUint32(b[phoff+0x1c:], align)
	return b
}

func TestELFLoadAlign(t *testing.T) {
	for _, tt := range []struct {
		name string
		data []byte
		want uint64
		err  string
	}{
		{"16 KB", fakeELF(0x4000, 0x4000), 0x4000, ""},
		{"64 KB", fakeELF(0x10000), 0x10000, ""},
		{"one 4 KB segment", fakeELF(0x4000, 0x1000, 0x4000), 0x1000, ""},
		{"32-bit", fakeELF32(0x1000), 0x1000, ""},
		{"no LOAD", fakeELF(), 0, "no LOAD segment"},
		{"not ELF", []byte("PK\x03\x04 not a library, but long enough to have a header's 64 bytes......."), 0, "not an ELF file"},
		{"short", []byte("\x7fELF"), 0, "not an ELF file"},
		{"cut in its headers", fakeELF(0x4000, 0x4000)[:100], 0, "reading ELF program headers"},
	} {
		got, err := elfLoadAlign(bytes.NewReader(tt.data))
		if tt.err != "" {
			if err == nil || !strings.Contains(err.Error(), tt.err) {
				t.Errorf("%s: %d, %v, want an error %q", tt.name, got, err, tt.err)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("%s: %#x, %v, want %#x", tt.name, got, err, tt.want)
		}
	}
}

// apkEntry is a file of a test APK: stored uncompressed at an offset that
// is mod modulo 16 KB, as zipalign pads the extra field, or compressed.
type apkEntry struct {
	name       string
	data       []byte
	compressed bool
	mod        int64
}

// countingWriter counts what it writes.
type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func writeAPK(t *testing.T, entries ...apkEntry) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.apk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	cw := &countingWriter{w: f}
	zw := zip.NewWriter(cw)
	for _, e := range entries {
		// Raw entries, whose sizes the local header holds: no data
		// descriptor follows them, which would move what comes next.
		h := &zip.FileHeader{Name: e.name, Method: zip.Store, CRC32: crc32.ChecksumIEEE(e.data), UncompressedSize64: uint64(len(e.data))}
		data := e.data
		if e.compressed {
			var b bytes.Buffer
			fw, _ := flate.NewWriter(&b, flate.BestCompression)
			fw.Write(e.data)
			fw.Close()
			h.Method, data = zip.Deflate, b.Bytes()
		} else {
			if err := zw.Flush(); err != nil {
				t.Fatal(err)
			}
			// The local header is 30 bytes and the name; zipalign's
			// padding is an extra field 0xd935 of at least 4 bytes.
			start := cw.n + 30 + int64(len(e.name)) + 4
			pad := ((e.mod-start)%androidPageSize + androidPageSize) % androidPageSize
			h.Extra = make([]byte, 4+pad)
			binary.LittleEndian.PutUint16(h.Extra, 0xd935)
			binary.LittleEndian.PutUint16(h.Extra[2:], uint16(pad))
		}
		h.CompressedSize64 = uint64(len(data))
		w, err := zw.CreateRaw(h)
		if err == nil {
			_, err = w.Write(data)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	// The offsets are as asked.
	z, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	for i, f := range z.File {
		if e := entries[i]; !e.compressed {
			if off, err := f.DataOffset(); err != nil || off%androidPageSize != e.mod {
				t.Fatalf("%s at %d (%v), want %d modulo 16 KB", f.Name, off, err, e.mod)
			}
		}
	}
	return path
}

// An APK is checked as Android checks it on a device with 16 KB pages:
// the libraries of 64-bit ABIs are linked for 16 KB pages, and those
// stored uncompressed start on 16 KB boundaries of the APK.
func TestAPKPageSizeProblems(t *testing.T) {
	good, fourK := fakeELF(0x4000, 0x4000), fakeELF(0x4000, 0x1000)
	path := writeAPK(t,
		apkEntry{name: "AndroidManifest.xml", data: []byte("<manifest/>"), compressed: true},
		apkEntry{name: "classes.dex", data: []byte("dex\n035"), mod: 4},
		apkEntry{name: "lib/arm64-v8a/libmygoapp.so", data: good},
		apkEntry{name: "lib/arm64-v8a/libsurface_util_jni.so", data: good, mod: 4096},
		apkEntry{name: "lib/arm64-v8a/libsqlitecore.so", data: fourK},
		apkEntry{name: "lib/x86_64/libboth.so", data: fourK, mod: 8},
		apkEntry{name: "lib/x86_64/libextracted.so", data: good, compressed: true},
		apkEntry{name: "lib/x86_64/libextracted4k.so", data: fourK, compressed: true},
		apkEntry{name: "lib/x86_64/libnotelf.so", data: []byte("#!/bin/sh\necho this is no library, but a script of more than 64 bytes\n")},
		// 32-bit ABIs run on no device with 16 KB pages.
		apkEntry{name: "lib/armeabi-v7a/libmygoapp.so", data: fakeELF32(0x1000), mod: 4},
		apkEntry{name: "lib/arm64-v8a/wrap.sh", data: []byte("#!/system/bin/sh"), mod: 4},
		apkEntry{name: "assets/lib/arm64-v8a/libnot.so", data: fourK, mod: 4},
	)
	got, err := apkPageSizeProblems(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"lib/arm64-v8a/libsurface_util_jni.so: stored uncompressed at offset",
		"lib/arm64-v8a/libsqlitecore.so: a LOAD segment is aligned to 4096 bytes, not 16 KB (link it with -Wl,-z,max-page-size=16384)",
		"lib/x86_64/libboth.so: stored uncompressed at offset",
		"lib/x86_64/libboth.so: a LOAD segment is aligned to 4096 bytes",
		"lib/x86_64/libextracted4k.so: a LOAD segment is aligned to 4096 bytes",
		"lib/x86_64/libnotelf.so: not an ELF file",
	}
	if len(got) != len(want) {
		t.Fatalf("problems:\n%s\nwant %d", strings.Join(got, "\n"), len(want))
	}
	for i, w := range want {
		if !strings.HasPrefix(got[i], w) {
			t.Errorf("problem %d: %q, want %q", i, got[i], w)
		}
	}
	if !strings.Contains(got[0], "not on a 16 KB boundary") {
		t.Errorf("%q", got[0])
	}

	ok := writeAPK(t,
		apkEntry{name: "lib/arm64-v8a/libmygoapp.so", data: good},
		apkEntry{name: "lib/arm64-v8a/libimage_processing_util_jni.so", data: fakeELF(0x10000)},
		apkEntry{name: "lib/x86_64/libmygoapp.so", data: good, compressed: true},
	)
	if got, err := apkPageSizeProblems(ok); err != nil || len(got) != 0 {
		t.Errorf("an aligned APK: %q, %v", got, err)
	}
	if _, err := apkPageSizeProblems(filepath.Join(t.TempDir(), "none.apk")); err == nil {
		t.Error("no error for a missing APK")
	}
}

// Go for Android links for 16 KB pages, after the user's own CGO_LDFLAGS,
// which it keeps.
func TestAndroidCgoLDFlags(t *testing.T) {
	for user, want := range map[string]string{
		"":                               "-Wl,-z,max-page-size=16384",
		"  ":                             "-Wl,-z,max-page-size=16384",
		"-L/opt/lib -lfoo":               "-L/opt/lib -lfoo -Wl,-z,max-page-size=16384",
		"-Wl,-z,max-page-size=4096":      "-Wl,-z,max-page-size=4096 -Wl,-z,max-page-size=16384",
		"-Wl,-z,max-page-size=65536 -lm": "-Wl,-z,max-page-size=65536 -lm -Wl,-z,max-page-size=16384",
	} {
		if got := androidCgoLDFlags(user); got != want {
			t.Errorf("androidCgoLDFlags(%q) = %q, want %q", user, got, want)
		}
	}
}

// The C of native modules links for 16 KB pages: the host's Gradle build
// asks it of the NDK's CMake for every module, and the SQLite plugin's
// links so with any NDK, which before r27 ignores what the host asks.
func TestAndroidModulesPageSize(t *testing.T) {
	src, err := fs.ReadFile(androidHost.Files, "build.gradle.kts")
	if err != nil {
		t.Fatal(err)
	}
	if want := `arguments += "-DANDROID_SUPPORT_FLEXIBLE_PAGE_SIZES=ON"`; !strings.Contains(string(src), want) {
		t.Errorf("the host's build.gradle.kts has no %s", want)
	}
	cmake, err := os.ReadFile(filepath.Join("..", "..", "plugins", "sqlite", "internal", "sqlitecore", "kotlin", "src", "main", "c", "CMakeLists.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(strings.Split(string(cmake), "\n"), func(l string) bool {
		return strings.HasPrefix(l, "target_link_options(sqlitecore ") && strings.Contains(l, "max-page-size=16384")
	}) {
		t.Error("sqlitecore's CMakeLists.txt does not link for 16 KB pages")
	}
}
