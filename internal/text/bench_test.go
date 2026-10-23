package text

import "testing"

// BenchmarkLayout lays out a wrapped paragraph without the cache, as each
// keystroke in an editor does.
func BenchmarkLayout(b *testing.B) {
	s := Shared()
	p := Params{Text: "The quick brown fox jumps over the lazy dog, and then some more words follow to wrap the paragraph over a few lines.", Style: Style{Size: 14}, Width: 300}
	s.Layout(p)
	b.ResetTimer()
	for range b.N {
		s.mu.Lock()
		s.layout(p)
		s.mu.Unlock()
	}
}

// BenchmarkLayoutLabel lays out a short label without the cache, as each
// text of a list's row coming into view does, and BenchmarkLayoutTruncated
// a line cut short with an ellipsis.
func BenchmarkLayoutLabel(b *testing.B) {
	benchLayout(b, Params{Text: "Grace Hopper", Style: Style{Size: 12}})
}

func BenchmarkLayoutChinese(b *testing.B) {
	benchLayout(b, Params{Text: "不必把每一分钟都填满。一杯茶、一段散步，或是随手记下的一句话，都可以成为一天里值得收藏的片段。从一件小事开始：留意窗外的光，整理桌面，给自己留十分钟不被打扰的时间。", Style: Style{Size: 14}, Width: 300})
}

func BenchmarkLayoutChineseLabel(b *testing.B) {
	benchLayout(b, Params{Text: "给日常留一点空白", Style: Style{Size: 12}})
}

func BenchmarkLayoutTruncated(b *testing.B) {
	benchLayout(b, Params{Text: "fix the list scroll when rows change height and keep focus", Style: Style{Size: 12}, Width: 160, MaxLines: 1})
}

func benchLayout(b *testing.B, p Params) {
	s := Shared()
	s.Layout(p)
	b.ReportAllocs()
	for b.Loop() {
		s.mu.Lock()
		s.layout(p)
		s.mu.Unlock()
	}
}

// BenchmarkLayoutDocument lays out the paragraphs of a document taking a
// little more than the cache's budget, as each frame of a window showing
// it does, with the cache.
func BenchmarkLayoutDocument(b *testing.B) {
	s := newSystem()
	ps := document(s)
	for _, p := range ps {
		s.Layout(p)
	}
	s.EndFrame()
	made := s.LayoutsMade()
	b.ReportAllocs()
	for b.Loop() {
		for _, p := range ps {
			s.Layout(p)
		}
		s.EndFrame()
	}
	b.ReportMetric(float64(s.LayoutsMade()-made)/float64(b.N), "layouts/op")
}

// BenchmarkLayoutLanguage includes the explicit language normal UI windows
// supply, without the display-text cache, as changing labels and input do.
func BenchmarkLayoutLanguage(b *testing.B) {
	for _, lang := range []string{"en", "zh-Hans", "ja"} {
		b.Run(lang, func(b *testing.B) {
			benchLayout(b, Params{Text: "2026年1月5日星期一 Metrics 1234", Style: Style{Size: 14, Lang: lang}, Width: 300})
		})
	}
}
