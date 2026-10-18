package updater

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

// ErrUpdateBusy means the current update window is checking or handling a
// different release. The caller can check again without canceling that window.
var ErrUpdateBusy = errors.New("updater: another update operation is in progress")

// InstallUpdate installs the selected version using the same signed installer
// and session as the native/web update window. The version must match a fresh
// check; no URL, archive or installation path can be supplied. A local install
// of the same release is joined, and an available prompt can be accepted without
// changing automatic-download preferences. Local cancellation remains effective.
//
// The call blocks until the update is installed, fails or ctx is canceled; call
// it off the UI thread. It does not relaunch the app. The caller may use
// mygo.App.Relaunch after reporting success to an authenticated remote controller.
func InstallUpdate(ctx context.Context, version string) error {
	u := active.Load()
	if u == nil {
		return errors.New("updater: the updater plugin is not enabled")
	}
	return u.installUpdate(ctx, version)
}

func (u *updater) installUpdate(ctx context.Context, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if version == "" {
		return errors.New("updater: select a version before installing")
	}
	if !enabled() {
		return errors.New("updater: updates are disabled for this installation")
	}
	// Resolve the session language before locking: App.Locale may need the main thread.
	t := u.text()
	u.load()
	u.mu.Lock()
	if r := u.installed; r != nil {
		u.mu.Unlock()
		if r.version != version {
			return ErrUpdateBusy
		}
		return nil
	}
	s := u.session
	automatic := u.state.AutomaticDownloads
	if s == nil {
		s = newSession(u, t, false)
		u.session = s
		u.launchChecked = true
		if u.timer != nil {
			u.timer.Stop()
			u.timer = nil
		}
		u.mu.Unlock()
		go func() {
			defer u.end(s)
			stop := context.AfterFunc(ctx, s.cancel)
			defer stop()
			s.set(s.t.checkingView())
			r, err := check(s.ctx)
			u.checked(err)
			if err == nil && (r == nil || r.version != version) {
				err = errors.New("updater: release changed; check updates again")
			}
			if err != nil {
				s.set(s.t.errorView(s.t.CheckError, err))
				return
			}
			s.mu.Lock()
			s.candidate = r
			s.mu.Unlock()
			_ = u.install(s, r)
		}()
	} else {
		u.mu.Unlock()
		s.mu.Lock()
		r := s.candidate
		s.mu.Unlock()
		if r == nil || r.version != version {
			return ErrUpdateBusy
		}
		v, _ := s.Current()
		if slices.ContainsFunc(v.Buttons, func(b button) bool { return b.Action == actionInstall && !b.Disabled }) {
			s.Respond(v.Prompt, actionInstall, automatic)
		} else if !v.Bar {
			u.mu.Lock()
			installed := u.installed
			u.mu.Unlock()
			if installed != nil && installed.version == version {
				return nil
			}
			return ErrUpdateBusy
		}
	}
	for {
		u.mu.Lock()
		installed := u.installed
		u.mu.Unlock()
		if installed != nil && installed.version == version {
			return nil
		}
		v, changed := s.Current()
		if v.Detail != "" {
			return fmt.Errorf("updater: %s", v.Detail)
		}
		if slices.ContainsFunc(v.Buttons, func(b button) bool { return b.Action == actionInstall && !b.Disabled }) {
			s.Respond(v.Prompt, actionInstall, automatic)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.Done():
			u.mu.Lock()
			installed = u.installed
			u.mu.Unlock()
			if installed != nil && installed.version == version {
				return nil
			}
			last, _ := s.Current()
			if last.Detail != "" {
				return fmt.Errorf("updater: %s", last.Detail)
			}
			return errors.New("updater: update canceled")
		case <-changed:
			if v.Detail != "" {
				return fmt.Errorf("updater: %s", v.Detail)
			}
		}
	}
}
