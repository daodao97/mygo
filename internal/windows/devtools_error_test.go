//go:build windows && (amd64 || arm64)

package windows

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/internal/platform"
)

// TestDevtoolsFailure keeps the protocol's message in the error, and tells
// an evaluation whose document went away from the other failures.
func TestDevtoolsFailure(t *testing.T) {
	const navigated = `{"error":{"code":-32000,"message":"Inspected target navigated or closed"},"id":4}` // as WebView2 answers
	for _, c := range []struct {
		method, answer, want string
	}{
		{"Runtime.evaluate", navigated, platform.ErrNavigated.Error()},
		{"Page.printToPDF", navigated, "mygo: Page.printToPDF failed (HRESULT 0x80070057): Inspected target navigated or closed"},
		{"Runtime.evaluate", `{"error":{"code":-32000,"message":"Execution context was destroyed."},"id":5}`, "mygo: Runtime.evaluate failed (HRESULT 0x80070057): Execution context was destroyed."},
		{"Runtime.evaluate", `{"error":{"code":-32602,"message":"Inspected target navigated or closed"},"id":6}`, "mygo: Runtime.evaluate failed (HRESULT 0x80070057): Inspected target navigated or closed"},
		{"Runtime.evaluate", "", "mygo: Runtime.evaluate failed (HRESULT 0x80070057)"},
		{"Runtime.evaluate", "not json", "mygo: Runtime.evaluate failed (HRESULT 0x80070057)"},
		{"Runtime.evaluate", "null", "mygo: Runtime.evaluate failed (HRESULT 0x80070057)"},
		{"Runtime.evaluate", `[{"error":{}}]`, "mygo: Runtime.evaluate failed (HRESULT 0x80070057)"},
		{"Runtime.evaluate", `{"error":"Inspected target navigated or closed"}`, "mygo: Runtime.evaluate failed (HRESULT 0x80070057)"},
	} {
		failure := hresultError(c.method, 0x80070057)
		err := devtoolsFailure(c.method, c.answer, failure)
		if err.Error() != c.want {
			t.Errorf("devtoolsFailure(%s, %q) = %q, want %q", c.method, c.answer, err, c.want)
		}
		left := c.want == platform.ErrNavigated.Error()
		if errors.Is(err, platform.ErrNavigated) != left || errors.Is(err, failure) == left {
			t.Errorf("devtoolsFailure(%s, %q) = %v: ErrNavigated %v, wraps the failure %v", c.method, c.answer, err, errors.Is(err, platform.ErrNavigated), errors.Is(err, failure))
		}
	}
}
