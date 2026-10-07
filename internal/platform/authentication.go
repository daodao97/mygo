package platform

// AuthenticationErrorCode identifies a native authentication failure.
type AuthenticationErrorCode string

const (
	AuthenticationCanceled       AuthenticationErrorCode = "canceled"
	AuthenticationFailed         AuthenticationErrorCode = "failed"
	AuthenticationUnavailable    AuthenticationErrorCode = "unavailable"
	AuthenticationNotEnrolled    AuthenticationErrorCode = "not-enrolled"
	AuthenticationLockedOut      AuthenticationErrorCode = "locked-out"
	AuthenticationPasscodeNotSet AuthenticationErrorCode = "passcode-not-set"
	AuthenticationFallback       AuthenticationErrorCode = "fallback"
	AuthenticationBusy           AuthenticationErrorCode = "busy"
	AuthenticationInactive       AuthenticationErrorCode = "inactive"
	AuthenticationMissingPurpose AuthenticationErrorCode = "missing-purpose"
	AuthenticationUnknown        AuthenticationErrorCode = "unknown"
)

type AuthenticationError struct {
	Code    AuthenticationErrorCode
	Message string
}

func (e *AuthenticationError) Error() string {
	return "mygo: authentication " + string(e.Code) + ": " + e.Message
}

type BiometricStatus struct {
	Kind                          string
	Available                     bool
	UnavailableReason             AuthenticationErrorCode
	DeviceAuthenticationAvailable bool
}
type AuthenticationOptions struct {
	Reason              string
	AllowDevicePasscode bool
}

// Authenticator delivers callbacks on the main thread exactly once. Its
// cancellation function runs on that thread and remains safe after completion.
type Authenticator interface {
	Query(done func(BiometricStatus, error))
	Authenticate(AuthenticationOptions, func(error)) (cancel func())
}
type UnsupportedAuthenticator struct{}

func (UnsupportedAuthenticator) Query(done func(BiometricStatus, error)) {
	done(BiometricStatus{}, ErrUnsupported)
}
func (UnsupportedAuthenticator) Authenticate(_ AuthenticationOptions, done func(error)) func() {
	done(ErrUnsupported)
	return func() {}
}
