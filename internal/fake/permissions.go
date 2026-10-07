package fake

import "github.com/egoist/mygo/internal/platform"

type permissions struct{ b *Backend }

func (b *Backend) Permissions() platform.Permissions { return permissions{b} }

func (p permissions) Query(kind string, done func(string, error)) {
	p.b.mu.Lock()
	status, err := p.b.PermissionStatuses[kind], p.b.PermissionError
	p.b.mu.Unlock()
	if status == "" {
		status = "not-determined"
	}
	done(status, err)
}
func (p permissions) Request(kind string, done func(string, error)) {
	p.b.mu.Lock()
	status, err := p.b.PermissionDecision, p.b.PermissionError
	if status == "" {
		status = "denied"
	}
	if err == nil {
		if p.b.PermissionStatuses == nil {
			p.b.PermissionStatuses = map[string]string{}
		}
		p.b.PermissionStatuses[kind] = status
	}
	p.b.mu.Unlock()
	done(status, err)
}
func (p permissions) OpenSettings(done func(error)) {
	p.b.mu.Lock()
	p.b.SettingsOpened++
	err := p.b.PermissionError
	p.b.mu.Unlock()
	done(err)
}
