package fake

import "github.com/egoist/mygo/internal/platform"

type secureStorage struct{ b *Backend }

func (b *Backend) SecureStorage() platform.SecureStorage { return secureStorage{b} }

func (s secureStorage) Set(namespace, key string, value []byte, _ string, done func(error)) {
	s.b.mu.Lock()
	err := s.b.SecretError
	if err == nil {
		if s.b.secrets == nil {
			s.b.secrets = map[string][]byte{}
		}
		data := make([]byte, len(value))
		copy(data, value)
		s.b.secrets[namespace+"\x00"+key] = data
	}
	s.b.mu.Unlock()
	done(err)
}
func (s secureStorage) Get(namespace, key string, done func([]byte, error)) {
	s.b.mu.Lock()
	value, exists := s.b.secrets[namespace+"\x00"+key]
	data := make([]byte, len(value))
	copy(data, value)
	err := s.b.SecretError
	if !exists && err == nil {
		err = platform.ErrSecretNotFound
	}
	s.b.mu.Unlock()
	if err != nil {
		data = nil
	}
	done(data, err)
}
func (s secureStorage) Delete(namespace, key string, done func(error)) {
	s.b.mu.Lock()
	err := s.b.SecretError
	if err == nil {
		delete(s.b.secrets, namespace+"\x00"+key)
	}
	s.b.mu.Unlock()
	done(err)
}
