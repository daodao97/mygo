package platform

// Mobile dialogs return exactly once on the main thread. Imported paths are
// private cache copies owned by the caller, never provider-owned temporary URLs.
// Export copies existing files to a user-selected destination.

type ExportDialogOptions struct {
	Title string
	Files []string
}

type PhotoDialogOptions struct {
	Multiple bool
}
