package unsupported

import "github.com/egoist/mygo/internal/platform"

func (*Backend) Authenticator() platform.Authenticator { return platform.UnsupportedAuthenticator{} }
