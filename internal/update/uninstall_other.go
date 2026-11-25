//go:build !windows

package update

// RefreshUninstallEntry sets the version of a Windows app's entry in
// Settings > Apps; other systems have no such entry.
func RefreshUninstallEntry(dir, identifier, version string) error { return nil }
