//go:build !android

package mygo

import "testing"

// Only Android has a JVM: elsewhere both are 0, which C takes for none.
func TestJVMOffAndroid(t *testing.T) {
	if vm := JavaVM(); vm != 0 {
		t.Errorf("JavaVM() = %#x, want 0", vm)
	}
	if c := AndroidContext(); c != 0 {
		t.Errorf("AndroidContext() = %#x, want 0", c)
	}
}
