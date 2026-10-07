package mygo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const appStateFile = "app-state.json"

type appStateStore struct {
	loaded   bool
	values   map[string]json.RawMessage
	bindings map[string]func() (json.RawMessage, error)
	last     []byte
}

// PersistState restores a JSON-compatible value and registers it for automatic
// saving on background entry and orderly shutdown. Call it in App.WhenReady
// before constructing content. Export the fields to persist; include route,
// draft and scroll state explicitly. Mutate the value only on the UI thread.
// off unregisters the live value; its last saved snapshot is retained.
// It is safe to call repeatedly or concurrently from any goroutine.
// Corrupt or incompatible snapshots return an error without changing value.
func PersistState[T any](key string, value *T) (off func(), err error) {
	if strings.TrimSpace(key) == "" || value == nil {
		return nil, errors.New("mygo: PersistState requires a key and a non-nil value")
	}
	needsApp("PersistState")
	onMain(func() {
		s := &App.state
		if err = App.loadState(); err != nil {
			return
		}
		if s.bindings[key] != nil {
			err = fmt.Errorf("mygo: state key %q is already registered", key)
			return
		}
		if data, ok := s.values[key]; ok {
			var restored T
			if err = json.Unmarshal(data, &restored); err != nil {
				err = fmt.Errorf("mygo: restoring state %q: %w", key, err)
				return
			}
			*value = restored
		}
		s.bindings[key] = func() (json.RawMessage, error) { return json.Marshal(value) }
		// Serialize cancellation on the UI thread. Holding a sync.Once lock
		// while waiting for that thread deadlocks if it also calls off.
		registered := true
		off = func() {
			onMain(func() {
				if registered {
					registered = false
					delete(s.bindings, key)
				}
			})
		}
	})
	return
}

func (a *Application) loadState() error {
	s := &a.state
	if s.loaded {
		return nil
	}
	dir, err := a.Path(PathUserData)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(dir, appStateFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var snapshot struct {
		Version int
		Values  map[string]json.RawMessage
	}
	if err == nil {
		if err := json.Unmarshal(data, &snapshot); err != nil {
			return fmt.Errorf("mygo: reading app state: %w", err)
		}
		if snapshot.Version != 1 {
			return fmt.Errorf("mygo: unsupported app state version %d", snapshot.Version)
		}
	}
	s.values = snapshot.Values
	if s.values == nil {
		s.values = map[string]json.RawMessage{}
	}
	s.bindings = map[string]func() (json.RawMessage, error){}
	s.last, s.loaded = data, true
	return nil
}

// SaveState checkpoints all registered values atomically. It can also be
// called after important edits; a force kill may have no background callback.
func (a *Application) SaveState() error {
	needsApp("App.SaveState")
	return onMainValue(a.saveState)
}

func (a *Application) OnStateSaveError(fn func(error)) func() {
	return a.onStateSaveError.add(fn, false)
}

func (a *Application) saveState() error {
	s := &a.state
	if len(s.bindings) == 0 {
		return nil
	}
	values := make(map[string]json.RawMessage, len(s.values))
	for key, value := range s.values {
		values[key] = value
	}
	for key, capture := range s.bindings {
		value, err := capture()
		if err != nil {
			return fmt.Errorf("mygo: saving state %q: %w", key, err)
		}
		values[key] = value
	}
	data, err := json.Marshal(struct {
		Version int
		Values  map[string]json.RawMessage
	}{1, values})
	if err != nil || bytes.Equal(data, s.last) {
		return err
	}
	dir, err := a.Path(PathUserData)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".app-state-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(dir, appStateFile)); err != nil {
		return err
	}
	s.values, s.last = values, data
	return nil
}
