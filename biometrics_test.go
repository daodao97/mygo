package mygo

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestBiometricsValidationAndNativeResults(t *testing.T) {
	t.Cleanup(func() {
		onMain(func() {
			fb.AuthenticationError = nil
			fb.AuthenticationPending = false
			fb.BiometricStatus = platform.BiometricStatus{}
		})
	})
	for _, reason := range []string{"", "  ", "a\x00b"} {
		if err := Biometrics.Authenticate(context.Background(), AuthenticationOptions{Reason: reason}); err == nil {
			t.Fatalf("accepted reason %q", reason)
		}
	}
	if err := Biometrics.Authenticate(nil, AuthenticationOptions{Reason: "Test"}); err == nil {
		t.Fatal("accepted nil context")
	}
	onMain(func() {
		fb.BiometricStatus = platform.BiometricStatus{Kind: "face-id", Available: false, UnavailableReason: AuthenticationLockedOut, DeviceAuthenticationAvailable: true}
	})
	status, err := Biometrics.Query()
	if err != nil || status.Kind != BiometricFaceID || status.Available || status.UnavailableReason != AuthenticationLockedOut || !status.DeviceAuthenticationAvailable {
		t.Fatalf("status: %+v %v", status, err)
	}
	o := AuthenticationOptions{Reason: "Verify device owner", AllowDevicePasscode: true}
	onMain(func() {
		if err := Biometrics.Authenticate(context.Background(), o); err != nil {
			t.Errorf("main thread auth: %v", err)
		}
		if fb.LastAuthentication.Reason != o.Reason || !fb.LastAuthentication.AllowDevicePasscode {
			t.Error("options were lost")
		}
	})
	for _, code := range []AuthenticationErrorCode{AuthenticationCanceled, AuthenticationFailed, AuthenticationNotEnrolled, AuthenticationLockedOut, AuthenticationMissingPurpose} {
		onMain(func() { fb.AuthenticationError = &AuthenticationError{Code: code, Message: "native"} })
		var native *AuthenticationError
		if err := Biometrics.Authenticate(context.Background(), o); !errors.As(err, &native) || native.Code != code {
			t.Fatalf("native error %q: %v", code, err)
		}
	}
	onMain(func() { fb.AuthenticationError = ErrUnsupported })
	if _, err := Biometrics.Query(); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	if err := Biometrics.Authenticate(context.Background(), o); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}

func TestAuthenticationContextCancellationAndLateReply(t *testing.T) {
	t.Cleanup(func() { onMain(func() { fb.CompleteAuthentication(nil); fb.AuthenticationPending = false }) })
	onMain(func() { fb.AuthenticationPending = true })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := Biometrics.Authenticate(ctx, AuthenticationOptions{Reason: "Deadline"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
	onMain(func() {
		// The completed request's late native reply must be ignored.
		fb.CompleteAuthentication(nil)
		callbacks := 0
		oldCancel := fb.Authenticator().Authenticate(platform.AuthenticationOptions{Reason: "First"}, func(error) { callbacks++ })
		var busy *AuthenticationError
		if err := Biometrics.Authenticate(context.Background(), AuthenticationOptions{Reason: "Concurrent"}); !errors.As(err, &busy) || busy.Code != AuthenticationBusy {
			t.Fatalf("concurrent authentication: %v", err)
		}
		fb.CompleteAuthentication(nil)
		newCancel := fb.Authenticator().Authenticate(platform.AuthenticationOptions{Reason: "Second"}, func(error) { callbacks++ })
		oldCancel()
		if callbacks != 1 {
			t.Fatal("old cancellation completed the next request")
		}
		newCancel()
		newCancel()
		fb.CompleteAuthentication(nil)
		if callbacks != 2 {
			t.Fatalf("completion count %d", callbacks)
		}
	})
	canceled, stop := context.WithCancel(context.Background())
	stop()
	if err := Biometrics.Authenticate(canceled, AuthenticationOptions{Reason: "Canceled"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
