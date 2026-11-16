//go:build windows && (amd64 || arm64)

package windows

import "testing"

func TestExternalShellTarget(t *testing.T) {
	for _, tt := range []struct {
		target string
		want   string
	}{
		{`file://F:\Example Work\项目`, `F:\Example Work\项目`},
		{`FILE://c:\100%\#notes`, `c:\100%\#notes`},
		{`file://C:\report.html?view=1`, `file://C:\report.html?view=1`},
		{`file:///F:/Example%20Work/%E9%A1%B9%E7%9B%AE`, `F:\Example Work\项目`},
		{`file://LOCALHOST/C:/notes%23one`, `C:\notes#one`},
		{`file:///C:/`, `C:\`},
		{`https://example.com/a%20b`, `https://example.com/a%20b`},
		{`file://server/share/project`, `file://server/share/project`},
		{`file://C:relative`, `file://C:relative`},
		{`file:///1:/example`, `file:///1:/example`},
		{`file://1:\example`, `file://1:\example`},
		{`file://user@localhost/C:/project`, `file://user@localhost/C:/project`},
		{`file:///C:/index.html#section`, `file:///C:/index.html#section`},
		{`file:///C:/index.html?mode=view`, `file:///C:/index.html?mode=view`},
		{`file:///C:/bad%3Fname`, `file:///C:/bad%3Fname`},
		{`file:///C:/bad%00name`, `file:///C:/bad%00name`},
	} {
		if got := externalShellTarget(tt.target); got != tt.want {
			t.Errorf("externalShellTarget(%q) = %q, want %q", tt.target, got, tt.want)
		}
	}
}
