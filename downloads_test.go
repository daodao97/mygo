//go:build !js

package mygo

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/fake"
	"github.com/egoist/mygo/internal/wait"
)

func TestDownloadEvents(t *testing.T) {
	dir := t.TempDir()
	App.SetPath(PathDownloads, dir)
	defer App.SetPath(PathDownloads, "")
	w, fw := testWindow(t, WindowOptions{})

	// The default path is the suggested name in Downloads, numbered.
	os.WriteFile(filepath.Join(dir, "report.csv"), nil, 0o644)
	start := func(url, name string) string {
		return onMainValue(func() string { return fw.H.DownloadStarted(url, name) })
	}
	if got := start("https://x/r", "report.csv"); got != filepath.Join(dir, "report (2).csv") {
		t.Errorf("path = %q", got)
	}
	if got := start("https://x/r", "../../etc/passwd"); got != filepath.Join(dir, "passwd") {
		t.Errorf("unsafe name: path = %q", got)
	}
	off := w.Page().OnWillDownload(func(e *DownloadEvent) { e.PreventDefault() })
	if got := start("https://x/r", "a.txt"); got != "" {
		t.Errorf("a prevented download got path %q", got)
	}
	off()

	// Custom schemes are downloaded from their handler.
	if err := Protocol.HandleFunc("dltest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename="data.json"`)
		w.Write([]byte(`{"ok":true}`))
	}); err != nil {
		t.Fatal(err)
	}
	defer Protocol.Unhandle("dltest")
	done := make(chan *Download, 1)
	defer w.Page().OnDownloadDone(func(d *Download) { done <- d })()
	onMain(func() { fw.H.SchemeDownload("dltest://localhost/export") })
	select {
	case d := <-done:
		if d.Err != nil || filepath.Base(d.Path) != "data.json" {
			t.Fatalf("download = %+v", d)
		}
		if b, _ := os.ReadFile(d.Path); string(b) != `{"ok":true}` {
			t.Errorf("saved %q", b)
		}
	case <-time.After(wait.Guard):
		t.Fatal("the scheme download did not finish")
	}
}

// A handler that panics while a download is served from it fails the
// download, before it started or with part of the file written, which is
// removed. The app goes on.
func TestDownloadSchemePanic(t *testing.T) {
	dir := t.TempDir()
	App.SetPath(PathDownloads, dir)
	defer App.SetPath(PathDownloads, "")
	w, fw := testWindow(t, WindowOptions{})
	if err := Protocol.HandleFunc("dlpanic", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/part" {
			w.Header().Set("Content-Disposition", `attachment; filename="part.bin"`)
			w.Write([]byte("half"))
		}
		panic("boom")
	}); err != nil {
		t.Fatal(err)
	}
	defer Protocol.Unhandle("dlpanic")
	done := make(chan *Download, 1)
	defer w.Page().OnDownloadDone(func(d *Download) { done <- d })()
	for _, tt := range []struct{ url, path string }{
		{"dlpanic://localhost/none", ""},
		{"dlpanic://localhost/part", filepath.Join(dir, "part.bin")},
	} {
		onMain(func() { fw.H.SchemeDownload(tt.url) })
		select {
		case d := <-done:
			if d.Err == nil || d.Path != tt.path {
				t.Errorf("%s: download = %+v, want an error and the path %q", tt.url, d, tt.path)
			}
		case <-time.After(wait.Guard):
			t.Fatalf("%s: the download did not finish", tt.url)
		}
		if tt.path != "" {
			if _, err := os.Stat(tt.path); !os.IsNotExist(err) {
				t.Errorf("%s: the partial file was left behind (%v)", tt.url, err)
			}
		}
	}
}

// A handler tells the page whose download it serves, as it does for the
// page's other requests: that of a window, or of a web view in one.
func TestDownloadSchemeCaller(t *testing.T) {
	dir := t.TempDir()
	App.SetPath(PathDownloads, dir)
	defer App.SetPath(PathDownloads, "")
	w, fw := testWindow(t, WindowOptions{})
	show := true
	host, v, _, fv := webViewWindow(t, &show)
	type caller struct {
		win  *Window
		page *Page
	}
	asked := make(chan caller, 1)
	if err := Protocol.HandleFunc("dlcaller", func(w http.ResponseWriter, r *http.Request) {
		asked <- caller{CallerWindow(r.Context()), CallerPage(r.Context())}
		w.Write([]byte("ok"))
	}); err != nil {
		t.Fatal(err)
	}
	defer Protocol.Unhandle("dlcaller")
	for _, tt := range []struct {
		name string
		fw   *fake.Window
		want caller
	}{
		{"window", fw, caller{w, w.Page()}},
		{"web view", fv.Window, caller{host, v.Page()}},
	} {
		done := make(chan *Download, 1)
		off := tt.want.page.OnDownloadDone(func(d *Download) { done <- d })
		onMain(func() { tt.fw.H.SchemeDownload("dlcaller://localhost/a.txt") })
		select {
		case d := <-done:
			if d.Err != nil {
				t.Errorf("%s: %v", tt.name, d.Err)
			}
			if got := <-asked; got != tt.want {
				t.Errorf("%s: the handler was asked by %v, want %v", tt.name, got, tt.want)
			}
		case <-time.After(wait.Guard):
			t.Fatalf("%s: the download did not finish", tt.name)
		}
		off()
	}
}
