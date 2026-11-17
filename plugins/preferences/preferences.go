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

func load() {
	if loaded {
		return
	}
	loaded = true
	values = map[string]json.RawMessage{}
	data, err := os.ReadFile(path())
	if err == nil {
		// One a crash cut short starts over: what it held is preferences.
		_ = json.Unmarshal(data, &values)
	}
}

// Get reads the value of key into v, and reports whether there was one.
func Get(key string, v any) bool {
	mu.Lock()
	defer mu.Unlock()
	load()
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
	load()
	values[key] = raw
	return save()
}

// Delete forgets the value of key.
func Delete(key string) error {
	mu.Lock()
	defer mu.Unlock()
	load()
	delete(values, key)
	return save()
}

// Keys lists the keys with values, in no order.
func Keys() []string {
	mu.Lock()
	defer mu.Unlock()
	load()
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
