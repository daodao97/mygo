package i18n

import (
	"testing"
	"time"
)

// TestFormatQuarter writes quarters in the locales whose CLDR data names
// them in ways of their own, as ICU writes them.
func TestFormatQuarter(t *testing.T) {
	// The 5th of February is in the first quarter.
	tm := time.Date(2026, time.February, 5, 13, 7, 0, 0, time.UTC)
	for _, c := range []struct{ locale, skeleton, want string }{
		{"en", "yQQQ", "Q1 2026"},
		{"en", "yQQQQ", "1st quarter 2026"},
		{"en", "yQQQQQ", "1 2026"},
		{"en", "QQQ", "Q1"},
		{"en", "Q", "1"},
		{"de", "yQQQ", "Q1 2026"},
		{"de", "yQQQQ", "1. Quartal 2026"},
		{"fr", "yQQQ", "T1 2026"},
		{"fr", "yQQQQ", "1er trimestre 2026"},
		{"ja", "yQQQ", "2026/Q1"},
		{"ja", "yQQQQ", "2026年第1四半期"},
		{"zh", "yQQQ", "2026年第1季度"},
		{"zh", "yQQQQ", "2026年第1季度"},
		{"ru", "yQQQ", "1-й кв. 2026 г."},
		{"ru", "yQQQQ", "1-й квартал 2026 г."},
		{"ar", "yQQQ", "الربع الأول 2026"},
		{"es", "yQQQQ", "1.er trimestre de 2026"},
		// A numeric quarter of a locale that has only patterns of quarters
		// in words keeps the words, as ICU writes them.
		{"en", "yQ", "Q1 2026"},
		{"en", "yQQ", "Q1 2026"},
		{"ja", "yQ", "2026/Q1"},
		{"ru", "yQ", "1-й кв. 2026 г."},
	} {
		if got := FormatDate(c.locale, tm, DateOptions{Skeleton: c.skeleton}); got != c.want {
			t.Errorf("FormatDate(%q, %q) = %q, want %q", c.locale, c.skeleton, got, c.want)
		}
	}
	// Q is a quarter inside a date, q standing alone, as L stands beside
	// M for a month.
	for _, c := range []struct{ locale, skeleton, want string }{
		{"en", "qqq", "Q1"},
		{"de", "qqq", "Q1"},
		{"zh", "qqq", "1季度"},
		{"zh", "qqqq", "第一季度"},
	} {
		if got := FormatDate(c.locale, tm, DateOptions{Skeleton: c.skeleton}); got != c.want {
			t.Errorf("FormatDate(%q, %q) = %q, want %q", c.locale, c.skeleton, got, c.want)
		}
	}
	// A quarter is three months of the year.
	for _, c := range []struct {
		month  time.Month
		suffix string
	}{
		{time.January, "第1季度"},
		{time.March, "第1季度"},
		{time.April, "第2季度"},
		{time.July, "第3季度"},
		{time.October, "第4季度"},
		{time.December, "第4季度"},
	} {
		tm := time.Date(2026, c.month, 4, 0, 0, 0, 0, time.UTC)
		if got, want := FormatDate("zh", tm, DateOptions{Skeleton: "yQQQQ"}), "2026年"+c.suffix; got != want {
			t.Errorf("the quarter of %v is %q, want %q", c.month, got, want)
		}
	}
}

// TestQuarters reads the names of the quarters in a width, standing alone
// or inside a date.
func TestQuarters(t *testing.T) {
	if q := Quarters("en", Wide, false); q != [4]string{"1st quarter", "2nd quarter", "3rd quarter", "4th quarter"} {
		t.Errorf("Quarters(en, wide) = %q", q)
	}
	if q := Quarters("en", Abbreviated, false); q != [4]string{"Q1", "Q2", "Q3", "Q4"} {
		t.Errorf("Quarters(en, abbreviated) = %q", q)
	}
	if q := Quarters("en", Narrow, true); q != [4]string{"1", "2", "3", "4"} {
		t.Errorf("Quarters(en, narrow, standing alone) = %q", q)
	}
	if got := Quarters("de", Wide, true)[0]; got != "1. Quartal" {
		t.Errorf("the first quarter of Germany standing alone is %q", got)
	}
	if got := Quarters("ja", Abbreviated, false)[0]; got != "Q1" {
		t.Errorf("the first quarter of Japan is %q", got)
	}
	if got := Quarters("zh", Wide, false)[0]; got != "第一季度" {
		t.Errorf("the first quarter of China is %q", got)
	}
	// Every locale names its four quarters, in every width.
	for _, locale := range []string{"en", "de", "fr", "ja", "zh", "ru", "ar", "es", "pt", "ko", "it", "fa", "th", "he", "hi", "sw"} {
		for _, w := range []Width{Abbreviated, Narrow, Wide} {
			for _, alone := range []bool{false, true} {
				for i, name := range Quarters(locale, w, alone) {
					if name == "" {
						t.Errorf("Quarters(%q, %v, %v)[%d] is empty", locale, w, alone, i)
					}
				}
			}
		}
	}
}

// TestFormatQuarterRange ranges over quarters, which CLDR has no patterns
// for: the fallback of the locale joins the two.
func TestFormatQuarterRange(t *testing.T) {
	q1 := time.Date(2026, time.February, 5, 0, 0, 0, 0, time.UTC)
	q2 := time.Date(2026, time.May, 5, 0, 0, 0, 0, time.UTC)
	if got, want := FormatDateRange("en", q1, q2, DateOptions{Skeleton: "yQQQ"}), "Q1 2026\u2009–\u2009Q2 2026"; got != want {
		t.Errorf("FormatDateRange(en, yQQQ) = %q, want %q", got, want)
	}
	if got, want := FormatDateRange("zh", q1, q2, DateOptions{Skeleton: "yQQQ"}), "2026年第1季度 – 2026年第2季度"; got != want {
		t.Errorf("FormatDateRange(zh, yQQQ) = %q, want %q", got, want)
	}
}

// TestFormatQuarterInMessage formats a quarter as an argument of a
// message, which takes the same options as FormatDate.
func TestFormatQuarterInMessage(t *testing.T) {
	m, err := ParseMessage("{q, date, ::yQQQ}")
	if err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"q": time.Date(2026, time.February, 5, 0, 0, 0, 0, time.UTC)}
	if got, want := m.Format("en", args), "Q1 2026"; got != want {
		t.Errorf("the message is %q, want %q", got, want)
	}
	if got, want := m.Format("ja", args), "2026/Q1"; got != want {
		t.Errorf("the message in Japanese is %q, want %q", got, want)
	}
}
