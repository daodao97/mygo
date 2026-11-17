//go:build darwin && !ios

package darwin

import (
	"slices"
	"testing"
)

func TestHourCycle(t *testing.T) {
	for pattern, want := range map[string]int{"h a": 12, "HH": 24, "K a": 12, "k": 24, "'h'": 0, "a 'o''clock' h": 12, "": 0} {
		if got := hourCycle(pattern); got != want {
			t.Errorf("hourCycle(%q) = %d, want %d", pattern, got, want)
		}
	}
}

// The current locale's change is observed under the name Foundation posts
// it under: observed by the constant's name, it never comes, and the region
// is read only at the time format's notification, before the current
// locale has the new format, one change behind.
func TestCurrentLocaleDidChangeName(t *testing.T) {
	load()
	if got := goString(currentLocaleDidChange()); got != "kCFLocaleCurrentLocaleDidChangeNotification" {
		t.Errorf("NSCurrentLocaleDidChangeNotification is posted as %q", got)
	}
}

// TestLocales reads the user's languages and formats, and keeps the app's
// language in its own defaults, where the global languages do not see it.
func TestLocales(t *testing.T) {
	load()
	a := appController{}
	langs := a.PreferredLanguages()
	if len(langs) == 0 {
		t.Fatal("no preferred languages")
	}
	if r := a.Region(); r.Locale == "" || r.FirstWeekday < 0 || r.HourCycle == 0 {
		t.Errorf("Region() = %+v", r)
	}
	if tag, kept := a.SavedLocale(); !kept || tag != "" {
		t.Fatalf("SavedLocale() = %q, %v before saving", tag, kept)
	}
	t.Cleanup(func() { a.SaveLocale("") })
	a.SaveLocale("x-test")
	if tag, _ := a.SavedLocale(); tag != "x-test" {
		t.Errorf("SavedLocale() = %q after saving x-test", tag)
	}
	if got := a.PreferredLanguages(); !slices.Equal(got, langs) {
		t.Errorf("the app's language changed the user's: %q, were %q", got, langs)
	}
	a.SaveLocale("")
	if tag, _ := a.SavedLocale(); tag != "" {
		t.Errorf("SavedLocale() = %q after forgetting it", tag)
	}
}
