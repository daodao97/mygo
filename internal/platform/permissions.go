package platform

// Permissions manages OS-level authorization, not web origin policy.
// Each callback is delivered exactly once on the main thread.
type Permissions interface {
	Query(kind string, done func(status string, err error))
	Request(kind string, done func(status string, err error))
	OpenSettings(done func(error))
}

type UnsupportedPermissions struct{}

func (UnsupportedPermissions) Query(_ string, done func(string, error))   { done("", ErrUnsupported) }
func (UnsupportedPermissions) Request(_ string, done func(string, error)) { done("", ErrUnsupported) }
func (UnsupportedPermissions) OpenSettings(done func(error))              { done(ErrUnsupported) }
