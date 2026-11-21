//go:build !js

package mygo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
	"github.com/egoist/mygo/internal/wait"
)

// Handlers see a body's length, which backends know, as servers' requests
// carry it: ContentLength 0 would mean no body, and httputil.ReverseProxy
// would forward none (#2177).
func TestProtocolContentLength(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)

	var length int64
	var noBody bool
	if err := Protocol.HandleFunc("bodylen", func(w http.ResponseWriter, r *http.Request) {
		length, noBody = r.ContentLength, r.Body == http.NoBody
		if r.URL.Path == "/proxy" {
			r.Host = target.Host
			proxy.ServeHTTP(w, r)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer Protocol.Unhandle("bodylen")
	_, fw := testWindow(t, WindowOptions{})
	serve := func(path string, header http.Header, body io.ReadCloser) *recorder {
		t.Helper()
		rec := &recorder{finished: make(chan struct{})}
		if header == nil {
			header = http.Header{}
		}
		req := &platform.SchemeRequest{Context: context.Background(), Method: "POST", URL: "bodylen://localhost" + path, Header: header, Body: body, Responder: rec}
		onMain(func() { fw.H.SchemeRequest(req) })
		select {
		case <-rec.finished:
		case <-time.After(wait.Guard):
			t.Fatal("request did not finish")
		}
		return rec
	}

	const payload = `{"a":1}`
	rec := serve("/proxy", nil, platform.BytesBody([]byte(payload)))
	if length != int64(len(payload)) || rec.status != 200 || rec.body.String() != payload {
		t.Errorf("proxied: ContentLength %d, upstream answered %d %q", length, rec.status, rec.body.String())
	}
	serve("/", nil, platform.BytesBody(nil))
	if length != 0 || !noBody {
		t.Errorf("empty body: ContentLength %d, NoBody %v", length, noBody)
	}
	serve("/", nil, nil)
	if length != 0 || !noBody {
		t.Errorf("no body: ContentLength %d, NoBody %v", length, noBody)
	}
	// A body of another kind: the header tells its length, or it is unknown.
	serve("/", http.Header{"Content-Length": {"7"}}, io.NopCloser(strings.NewReader(payload)))
	if length != 7 {
		t.Errorf("Content-Length header: ContentLength %d", length)
	}
	serve("/", nil, io.NopCloser(strings.NewReader(payload)))
	if length != -1 {
		t.Errorf("unknown length: ContentLength %d", length)
	}
}
