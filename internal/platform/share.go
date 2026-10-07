package platform

type ShareOptions struct {
	Text, URL string
	Files     []string
}

type ShareResult struct {
	Completed bool
	Activity  string
}

// Sharing presents native system activities. The callback runs on the main
// thread exactly once, including cancellation and presentation failure.
type Sharing interface {
	Show(parent Window, options ShareOptions, done func(ShareResult, error))
}

// UnsupportedSharing is used by backends without a system share presenter.
type UnsupportedSharing struct{}

func (UnsupportedSharing) Show(_ Window, _ ShareOptions, done func(ShareResult, error)) {
	done(ShareResult{}, ErrUnsupported)
}
