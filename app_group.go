package mygo

import (
	"fmt"
	"strings"
)

// AppGroupDirectory returns an entitled iOS shared container, before or after
// App.Run and from any goroutine. It applies protection until first unlock to
// the container directory; newly created children inherit it. Existing files
// keep their own protection. There is no private-directory fallback on denial.
// Other platforms return ErrUnsupported. Use sharedstore for bounded atomic
// files and cross-process transactions within this directory.
func AppGroupDirectory(identifier string) (string, error) {
	if !strings.HasPrefix(identifier, "group.") || len(identifier) <= 6 || strings.ContainsAny(identifier, "/\\\x00\r\n ") {
		return "", fmt.Errorf("mygo: invalid App Group identifier")
	}
	return hostAppGroupDirectory(identifier)
}
