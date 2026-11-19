package android

import (
	"bytes"
	"io/fs"
	"testing"
)

// TestGradleWrapper checks that the host ships Gradle's wrapper for both
// kinds of host: mygo build runs gradlew on macOS and Linux and
// gradlew.bat on Windows, which needs CRLF line endings.
func TestGradleWrapper(t *testing.T) {
	for _, name := range []string{"gradlew", "gradlew.bat", "gradle/wrapper/gradle-wrapper.jar", "gradle/wrapper/gradle-wrapper.properties"} {
		if _, err := fs.Stat(Files, name); err != nil {
			t.Errorf("Files has no %s: %v", name, err)
		}
	}
	sh, err := fs.ReadFile(Files, "gradlew")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sh, []byte("\r")) {
		t.Error("gradlew has CR line endings, which sh cannot run")
	}
	bat, err := fs.ReadFile(Files, "gradlew.bat")
	if err != nil {
		t.Fatal(err)
	}
	if n, crlf := bytes.Count(bat, []byte("\n")), bytes.Count(bat, []byte("\r\n")); n == 0 || n != crlf {
		t.Errorf("gradlew.bat has %d of %d lines ending in CRLF, want all", crlf, n)
	}
}
