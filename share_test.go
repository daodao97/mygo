package mygo

import (
	"errors"
	"github.com/egoist/mygo/internal/platform"
	"os"
	"path/filepath"
	"testing"
)

func TestShareValidationAndNativeResult(t *testing.T) {
	file := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(file, []byte("share round trip"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, o := range []ShareOptions{{}, {URL: "relative"}, {URL: "file:///tmp/report"}, {Files: []string{filepath.Dir(file)}}, {Files: []string{file + "missing"}}} {
		if _, err := Share.Show(o); err == nil {
			t.Fatalf("accepted invalid share: %+v", o)
		}
	}
	onMain(func() { fb.ShareResult = platform.ShareResult{Completed: true, Activity: "copy"}; fb.ShareError = nil })
	t.Cleanup(func() { onMain(func() { fb.ShareResult = platform.ShareResult{}; fb.ShareError = nil }) })
	check := func() {
		res, err := Share.Show(ShareOptions{Text: "hello", URL: "https://example.com", Files: []string{file}})
		if err != nil || !res.Completed || res.Activity != "copy" {
			t.Errorf("share result: %+v %v", res, err)
		}
	}
	check()
	onMain(check)
	onMain(func() {
		if fb.LastShare.Text != "hello" || fb.LastShare.URL != "https://example.com" || len(fb.LastShare.Files) != 1 || fb.LastShare.Files[0] != file {
			t.Errorf("wrong native items: %+v", fb.LastShare)
		}
		fb.ShareResult = platform.ShareResult{}
	})
	if res, err := Share.Show(ShareOptions{Text: "cancel"}); err != nil || res.Completed {
		t.Fatalf("cancellation: %+v %v", res, err)
	}
	onMain(func() { fb.ShareError = platform.ErrUnsupported })
	if _, err := Share.Show(ShareOptions{Text: "unsupported"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported error: %v", err)
	}
}

func TestMessagePresentationValidation(t *testing.T) {
	if _, err := Dialog.Message(MessageOptions{Style: "unknown"}); err == nil {
		t.Fatal("accepted unknown style")
	}
	if _, err := Dialog.Message(MessageOptions{Buttons: []string{"OK"}, DestructiveButtons: []int{1}}); err == nil {
		t.Fatal("accepted invalid destructive index")
	}
}
