//go:build windows && (amd64 || arm64)

package text

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

// IDWriteFontFace3's methods the tests call.
const face3GetFamilyNames = 40

// faceFamily returns the English name of a font face's family.
func faceFamily(face uintptr) string {
	f3 := queryInterface(face, &iidIDWriteFontFace3)
	if f3 == 0 {
		return ""
	}
	defer release(f3)
	var strs uintptr
	if failed(call(f3, face3GetFamilyNames, uintptr(unsafe.Pointer(&strs)))) {
		return ""
	}
	defer release(strs)
	return englishName(strs)
}

// faceWeightAxis returns the value of a font face's weight axis
// (IDWriteFontFace5, Windows 10 1803 and later).
func faceWeightAxis(face uintptr) (float32, bool) {
	f5 := queryInterface(face, &iidIDWriteFontFace5)
	if f5 == 0 {
		return 0, false
	}
	defer release(f5)
	n := uint32(call(f5, face5GetFontAxisValueCount))
	if n == 0 {
		return 0, false
	}
	values := make([]axisValue, n)
	if failed(call(f5, face5GetFontAxisValues, uintptr(unsafe.Pointer(&values[0])), uintptr(n))) {
		return 0, false
	}
	for _, v := range values {
		if v.tag == tagWght {
			return v.value, true
		}
	}
	return 0, false
}

// Chinese text in no language that writes Han characters, as an English
// app's, takes the first of the user's preferred languages that does: one
// font, that of Chinese text, rather than the English fallback's mix of a
// Japanese font where it has the characters and a Chinese one where it
// lacks them, which draws one label in two weights (#1454).
func TestDWriteUserHanLanguage(t *testing.T) {
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	saved, savedRead := e.han, e.hanRead
	defer func() { e.han, e.hanRead = saved, savedRead }()
	e.hanRead = true // e.han as set below, not as the user's settings say
	texts := []string{"我们的音乐播放器正在播放周杰伦的歌曲", "这是一个中文标签，看看字重是否一致。", "Hello 世界 说话 东西"}
	// hanFamilies returns the families of the runs of Han characters of
	// text in a style and spans.
	hanFamilies := func(text string, style Style, spans []Span) map[string]bool {
		r := []rune(text)
		e.resetScratch()
		fams := map[string]bool{}
		for _, line := range e.shapeText(r, style, spans, 0, false, false) {
			for _, run := range line.runs {
				if slices.ContainsFunc(r[run.start:run.end], isCJK) {
					fams[faceFamily(run.font.native)] = true
				}
			}
		}
		return fams
	}

	// The fallback of the user's locale alone, the runner's en-US.
	e.han = nil
	for _, text := range texts {
		t.Logf("%q in the locale %s: %v", text, syscall.UTF16ToString(e.locale), slices.Sorted(maps.Keys(hanFamilies(text, Style{Size: 13}, nil))))
	}

	// A user of English Windows who reads Chinese (userHanLanguage).
	e.han = utf16z("zh-Hans-CN")
	for _, style := range []Style{{Size: 13}, {Size: 13, Lang: "en"}, {Size: 13, Weight: 700}, {Size: 13, Weight: 300}, {Size: 13, Lang: "en-US", Weight: 600}} {
		chinese := style
		chinese.Lang = "zh-Hans-CN"
		for _, text := range texts {
			got := hanFamilies(text, style, nil)
			if len(got) != 1 {
				t.Errorf("%q in %+v is drawn in %v, not one font", text, style, slices.Sorted(maps.Keys(got)))
			}
			if want := hanFamilies(text, chinese, nil); !maps.Equal(got, want) {
				t.Errorf("%q in %+v is drawn in %v, Chinese text in %v", text, style, slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
			}
		}
	}
	// Text in a language that writes Han characters keeps its own.
	for _, text := range texts {
		e.han = nil
		want := hanFamilies(text, Style{Size: 13, Lang: "ja"}, nil)
		wantSpan := hanFamilies(text, Style{Size: 13, Lang: "en"}, []Span{{End: 100, Lang: "ja"}})
		e.han = utf16z("zh-Hans-CN")
		if got := hanFamilies(text, Style{Size: 13, Lang: "ja"}, nil); !maps.Equal(got, want) {
			t.Errorf("Japanese %q is drawn in %v, %v without the user's Chinese", text, slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
		}
		if got := hanFamilies(text, Style{Size: 13, Lang: "en"}, []Span{{End: 100, Lang: "ja"}}); !maps.Equal(got, wantSpan) {
			t.Errorf("a Japanese span %q is drawn in %v, %v without the user's Chinese", text, slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(wantSpan)))
		}
	}
}

// The user's language for Han characters is read the first time a text
// has Han characters in no language that writes them, never for others'
// (#1479), and the languages of the user's profile read as Windows lists
// them.
func TestDWriteUserHanLazy(t *testing.T) {
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	for _, p := range []struct {
		text  string
		style Style
	}{{"Hello, world", Style{Size: 13}}, {"日本語", Style{Size: 13, Lang: "ja"}}, {"中文", Style{Size: 13, Lang: "zh-Hans"}}} {
		e.resetScratch()
		e.shapeText([]rune(p.text), p.style, nil, 0, false, false)
		if e.hanRead {
			t.Fatalf("%q in %+v read the user's language for Han characters", p.text, p.style)
		}
	}
	e.resetScratch()
	e.shapeText([]rune("Hello 世界"), Style{Size: 13, Lang: "en"}, nil, 0, false, false)
	if !e.hanRead {
		t.Errorf("Han characters in English did not read the user's language for them")
	}
	profile := userProfileLanguages()
	t.Logf("locale %s, display languages %q, profile languages %q: Han characters in %q", syscall.UTF16ToString(e.locale), preferredLanguages(), profile, syscall.UTF16ToString(e.han))
	for _, lang := range profile {
		if lang == "" || strings.ContainsRune(lang, 0) {
			t.Errorf("profile languages %q have %q", profile, lang)
		}
	}
}

// A variable font's glyphs are rasterized at the instance of the weight,
// as fallback picks them too: the face of the glyph run, which carries the
// instance's axis values, is the one rasterized, and a heavier weight
// covers more pixels. Weights between the font's named instances are
// instances of the weight axis too, as DirectWrite alone would draw the
// nearest named one (500 in Segoe UI Variable drew its Semibold, 600,
// #2137). Windows has variable fonts of its own (Bahnschrift from Windows
// 10, Segoe UI Variable from Windows 11).
func TestDWriteVariableWeight(t *testing.T) {
	s := newSystem()
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.engine().(*dwrite)
	tested := false
	for _, family := range []string{"Bahnschrift", "Segoe UI Variable Text"} {
		if !e.hasSystem(family) {
			continue
		}
		last := 0
		for _, weight := range []int{300, 400, 450, 500, 550, 600, 650, 700} {
			e.resetScratch()
			lines := e.shapeText([]rune("H"), Style{Size: 40, Family: family, Weight: weight}, nil, 0, false, false)
			if len(lines) != 1 || len(lines[0].runs) != 1 {
				t.Fatalf("H in %s %d shaped into %+v", family, weight, lines)
			}
			run := lines[0].runs[0]
			wght, ok := faceWeightAxis(run.font.native)
			if !ok {
				t.Logf("%s's face has no axis values here", family)
				break
			}
			tested = true
			if int(wght) != weight {
				t.Errorf("%s %d's run is of the instance of weight %v", family, weight, wght)
			}
			// The face of its metrics too.
			if wght, _ := faceWeightAxis(e.font(Style{Size: 40, Family: family, Weight: weight}).native); int(wght) != weight {
				t.Errorf("%s %d's font is of the instance of weight %v", family, weight, wght)
			}
			coverage := 0
			for _, p := range e.glyph(run.font, run.glyphs[0].ID, 1, 0, 0, false).pix {
				coverage += int(p)
			}
			if coverage <= last {
				t.Errorf("H in %s %d covers %d, no more than a lighter weight's %d", family, weight, coverage, last)
			}
			last = coverage
		}
	}
	if !tested {
		t.Skip("no variable font with axis values")
	}

	// runWeights returns the family and weight axis value of each run of
	// a text.
	runWeights := func(text string, style Style, spans []Span) []string {
		e.resetScratch()
		var runs []string
		for _, line := range e.shapeText([]rune(text), style, spans, 0, false, false) {
			for _, run := range line.runs {
				wght, _ := faceWeightAxis(run.font.native)
				runs = append(runs, fmt.Sprintf("%s %v", faceFamily(run.font.native), wght))
			}
		}
		return runs
	}
	// Spans of other weights and families take their own: a weight of the
	// font's, a named instance, or a static family's as DirectWrite
	// matches it.
	family := "Segoe UI Variable Text"
	if !e.hasSystem(family) {
		family = "Bahnschrift"
	}
	arial := runWeights("Hg", Style{Size: 14, Family: "Arial"}, nil)
	got := runWeights("aaa bbb ccc ddd eee", Style{Size: 14, Family: family, Weight: 500},
		[]Span{{End: 4}, {End: 8, Weight: 400}, {End: 12, Weight: 700}, {End: 16, Weight: 550}, {End: 19, Family: "Arial"}})
	if want := []string{family + " 500", family + " 400", family + " 700", family + " 550", arial[0]}; !slices.Equal(got, want) {
		t.Errorf("spans drew runs %q, want %q", got, want)
	}
	// Static families, and the families the fallback picks, match weights
	// as before: the weight axis's value would change what they match
	// (Segoe UI's 500 its Regular rather than its Semibold, Yu Gothic UI's
	// 650 its Semilight rather than its Bold).
	if e.hasSystem("Segoe UI") {
		for _, weight := range []int{500, 650} {
			static := runWeights("Hg", Style{Size: 14, Family: "Segoe UI", Weight: weight}, nil)
			legacy := e.matchFace(e.collection(false), "Segoe UI", weight, false)
			want, _ := faceWeightAxis(legacy)
			release(legacy)
			if len(static) != 1 || static[0] != fmt.Sprintf("Segoe UI %v", want) {
				t.Errorf("Segoe UI %d drew %q, want weight %v", weight, static, want)
			}
		}
	}
	for _, weight := range []int{500, 650} {
		runs := runWeights("H中", Style{Size: 14, Family: family, Weight: weight}, nil)
		static := runWeights("中", Style{Size: 14, Family: "Arial", Weight: weight}, nil)
		if len(runs) != 2 || len(static) != 1 || runs[1] != static[0] {
			t.Errorf("H中 in %s %d drew %q, its fallback unlike Arial's %q", family, weight, runs, static)
		}
	}
}
