package mygo

import "github.com/egoist/mygo/internal/platform"

// LifecycleState is the application's activity, independent of readiness.
// A temporary interruption makes it inactive without entering the background.
type LifecycleState string

const (
	LifecycleStarting   LifecycleState = "starting"
	LifecycleInactive   LifecycleState = "inactive"
	LifecycleActive     LifecycleState = "active"
	LifecycleBackground LifecycleState = "background"
	LifecycleStopped    LifecycleState = "stopped"
)

// Lifecycle returns the latest activity state. Mobile backgrounding never
// means Quit; the OS may later suspend or terminate the process without a callback.
func (a *Application) Lifecycle() LifecycleState {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lifecycle == "" {
		return LifecycleStarting
	}
	return a.lifecycle
}

func (a *Application) setLifecycle(s LifecycleState) bool {
	a.mu.Lock()
	if a.lifecycle == s || a.lifecycle == LifecycleStopped {
		a.mu.Unlock()
		return false
	}
	a.lifecycle = s
	a.mu.Unlock()
	fire1(&a.onLifecycleChanged, s)
	return true
}

func (a *Application) OnLifecycleChanged(fn func(LifecycleState)) func() {
	return a.onLifecycleChanged.add(fn, false)
}

// OnDidEnterBackground runs before registered state is saved. Keep callbacks
// short; this is not an unrestricted background execution grant.
func (a *Application) OnDidEnterBackground(fn func()) func() {
	return a.onDidEnterBackground.add(fn, false)
}

// OnWillEnterForeground runs before the app becomes active again.
func (a *Application) OnWillEnterForeground(fn func()) func() {
	return a.onWillEnterForeground.add(fn, false)
}

// IsSystemManaged reports whether closing/quitting belongs to the OS (iOS).
func (a *Application) IsSystemManaged() bool { return backend().SystemManagedLifetime() }

// TryQuit follows the cancelable desktop quit sequence. iOS returns
// ErrUnsupported without closing content or stopping the Go dispatch loop.
func (a *Application) TryQuit() error {
	return onMainValue(func() error {
		if a.IsSystemManaged() {
			return platform.ErrUnsupported
		}
		if a.prepareQuit() {
			backend().Quit()
		}
		return nil
	})
}

func (a *Application) checkpoint() {
	for _, w := range Windows() {
		w.captureState()
	}
	saveWindowStates()
	if err := a.saveState(); err != nil {
		fire1(&a.onStateSaveError, err)
	}
}
