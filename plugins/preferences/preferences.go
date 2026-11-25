// Package preferences keeps small values of an app between its runs, as
// UserDefaults and SharedPreferences do, on every platform: a JSON file in
// the app's data directory, replaced whole, safe from any goroutine. For
// secrets use mygo.Secrets.
//
//	var theme string
//	preferences.Get("theme", &theme)
//	preferences.Set("theme", "dark")
package preferences

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
)

var (
	mu     sync.Mutex
	loaded bool
	values map[string]json.RawMessage
)

func path() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "preferences.json")
}

// load reads the file, once it can be read: one that can't is not written
// over, and the next call tries it again.
func load() error {
	if loaded {
		return nil
	}
	data, err := os.ReadFile(path())
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	loaded = true
	if err == nil {
		// One a crash cut short starts over: what it held is preferences.
		_ = json.Unmarshal(data, &values)
	}
	if values == nil {
		values = map[string]json.RawMessage{}
	}
	return nil
}

// Get reads the value of key into v, and reports whether there was one.
func Get(key string, v any) bool {
	mu.Lock()
	defer mu.Unlock()
	_ = load()
	raw, ok := values[key]
	if !ok {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

// Set keeps v, as JSON, under key, and writes the file.
func Set(key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	if err := load(); err != nil {
		return err
	}
	values[key] = raw
	return save()
}

// Delete forgets the value of key.
func Delete(key string) error {
	mu.Lock()
	defer mu.Unlock()
	if err := load(); err != nil {
		return err
	}
	delete(values, key)
	return save()
}

// Keys lists the keys with values, in no order.
func Keys() []string {
	mu.Lock()
	defer mu.Unlock()
	_ = load()
	out := make([]string, 0, len(values))
	for k := range values {
		out = append(out, k)
	}
	return out
}

func save() error {
	p := path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".preferences-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), p)
}
