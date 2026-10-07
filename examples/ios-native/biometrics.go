package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (s *demo) biometricControls(c *ui.Context) {
	ui.Text(c, "Device Owner Authentication").FontSize(24).Bold()
	ui.Text(c, "Check availability or authenticate with Face ID / Touch ID. Passcode fallback uses the system prompt. This demo does not read biometric data or unlock Keychain items.").FontSize(15).LineHeight(1.4)
	ui.Text(c, s.biometricResult).FontSize(14).LineHeight(1.4)
	if ui.Button(c, "Check biometrics").Height(44).FillWidth().Disabled(s.biometricPending).Clicked() {
		s.biometricPending = true
		go func() {
			status, err := mygo.Biometrics.Query()
			s.window.Update(func() {
				s.biometricPending = false
				if err != nil {
					s.biometricResult = err.Error()
					return
				}
				s.biometricResult = fmt.Sprintf("Biometrics: %s · available: %v · device authentication: %v", status.Kind, status.Available, status.DeviceAuthenticationAvailable)
				if status.UnavailableReason != "" {
					s.biometricResult += " · " + string(status.UnavailableReason)
				}
			})
		}()
	}
	for _, mode := range []string{"Authenticate with biometrics", "Authenticate with passcode fallback", "Cancel authentication after 2 seconds"} {
		if ui.Button(c, mode).Height(44).FillWidth().Disabled(s.biometricPending).Clicked() {
			var ctx context.Context
			var cancel context.CancelFunc
			if mode == "Cancel authentication after 2 seconds" {
				ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			s.biometricCancel = cancel
			s.biometricPending = true
			s.biometricResult = "Authentication pending"
			go func() {
				defer cancel()
				err := mygo.Biometrics.Authenticate(ctx, mygo.AuthenticationOptions{Reason: "Verify the device owner for the MyGo demo.", AllowDevicePasscode: mode == "Authenticate with passcode fallback"})
				s.window.Update(func() {
					s.biometricPending, s.biometricCancel = false, nil
					s.biometricResult = "Authentication succeeded"
					if errors.Is(err, context.Canceled) {
						s.biometricResult = "Authentication canceled by app"
					} else if errors.Is(err, context.DeadlineExceeded) {
						s.biometricResult = "Authentication timed out"
					} else if err != nil {
						var native *mygo.AuthenticationError
						if errors.As(err, &native) {
							s.biometricResult = "Authentication: " + string(native.Code)
						} else {
							s.biometricResult = err.Error()
						}
					}
				})
			}()
		}
	}
	if ui.Button(c, "Cancel authentication").Height(44).FillWidth().Disabled(s.biometricCancel == nil).Clicked() {
		s.biometricCancel()
	}
}
