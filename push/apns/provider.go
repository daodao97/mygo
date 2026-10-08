package apns

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ProviderConfig is sender metadata written by `mygo push setup`. Private keys
// are stored separately in the macOS login Keychain or an owner-only file.
// The file format also accepts existing MyGo application provider settings.
type ProviderConfig struct {
	TeamID          string      `json:"teamID"`
	KeyID           string      `json:"keyID"`
	Topic           string      `json:"topic"`
	Environment     Environment `json:"environment"`
	KeyFile         string      `json:"keyFile,omitempty"`
	KeychainService string      `json:"keychainService,omitempty"`
}

var providerTopic = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]{0,254}$`)

func (c ProviderConfig) validate() error {
	if !validID(c.TeamID) || !validID(c.KeyID) {
		return errors.New("apns: TeamID and KeyID must be ten uppercase letters or digits")
	}
	if !providerTopic.MatchString(c.Topic) {
		return errors.New("apns: provider requires a valid application bundle identifier")
	}
	if c.Environment != Sandbox && c.Environment != Production {
		return errors.New("apns: provider environment must be sandbox or production")
	}
	if c.KeychainService != "" && c.KeychainService != c.Topic+".apns" {
		return errors.New("apns: keychain service must be scoped to the provider topic")
	}
	return nil
}

// ImportProvider validates and stores a provider key, then atomically publishes
// its metadata in dir/push-config.json. It needs no MyGo event loop. Call it on
// the sender host only; iOS cannot store provider credentials with this API.
// Keychain access is bounded and private key bytes never enter command arguments.
func ImportProvider(dir string, cfg ProviderConfig, key []byte) error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return errors.New("apns: managed credential import supports macOS Keychain and Linux private files")
	}
	if cfg.Environment == "" {
		cfg.Environment = Sandbox
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	if len(key) > 8192 {
		return errors.New("apns: provider key exceeds 8 KB")
	}
	if _, err := New(Config{TeamID: cfg.TeamID, KeyID: cfg.KeyID, PrivateKey: key, Environment: cfg.Environment}); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	cfg.KeyFile, cfg.KeychainService = "", ""
	if runtime.GOOS == "darwin" {
		cfg.KeychainService = cfg.Topic + ".apns"
		input := "add-generic-password -U -s '" + cfg.KeychainService + "' -a '" + cfg.KeyID + "' -w '" + base64.StdEncoding.EncodeToString(key) + "'\n"
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "security", "-i")
		cmd.Stdin = strings.NewReader(input)
		if err := cmd.Run(); err != nil {
			return errors.New("apns: unable to save provider key in Keychain")
		}
		stored, err := providerKey(cfg)
		if err != nil || !bytes.Equal(stored, key) {
			return errors.New("apns: Keychain save verification failed")
		}
	} else {
		path, err := filepath.Abs(filepath.Join(dir, "apns.p8"))
		if err != nil {
			return err
		}
		cfg.KeyFile = path
		if err := writeProviderFile(path, key); err != nil {
			return err
		}
	}
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeProviderFile(filepath.Join(dir, "push-config.json"), raw)
}

func writeProviderFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".apns-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func providerKey(cfg ProviderConfig) ([]byte, error) {
	var key []byte
	var err error
	if cfg.KeychainService != "" {
		if runtime.GOOS != "darwin" {
			return nil, errors.New("apns: this provider key requires macOS Keychain")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		raw, e := exec.CommandContext(ctx, "security", "find-generic-password", "-s", cfg.KeychainService, "-a", cfg.KeyID, "-w").Output()
		err = e
		if err == nil {
			key, err = base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		}
	} else {
		f, e := os.Open(cfg.KeyFile)
		if e != nil {
			return nil, errors.New("apns: unable to read provider key")
		}
		defer f.Close()
		st, e := f.Stat()
		if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return nil, errors.New("apns: provider key must be an owner-only regular file")
		}
		key, err = io.ReadAll(io.LimitReader(f, 8193))
	}
	if err != nil || len(key) > 8192 {
		return nil, errors.New("apns: unable to read provider key")
	}
	return key, nil
}

// Provider is a configured sender with a default topic. It is safe for
// concurrent use, independent of the MyGo event loop, and retains its HTTP/2
// connections and JWT when Reload finds unchanged credentials.
type Provider struct {
	mu     sync.Mutex
	dir    string
	client *Client
	topic  string
	digest [32]byte
}

// Topic returns the application's bundle identifier from provider metadata.
func (p *Provider) Topic() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.topic
}

// OpenProvider loads credentials created by `mygo push setup` from an
// application data directory. It never reads mygo.json or embeds a private key.
func OpenProvider(dir string) (*Provider, error) {
	if runtime.GOOS == "ios" {
		return nil, errors.New("apns: provider credentials belong on a desktop or server")
	}
	p := &Provider{dir: dir}
	if err := p.Reload(); err != nil {
		return nil, err
	}
	return p, nil
}

// Reload picks up configuration and key rotation. A failed reload disables
// sending until a later successful reload; it never silently keeps stale keys.
func (p *Provider) Reload() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.reload(); err != nil {
		p.client = nil
		return err
	}
	return nil
}

func (p *Provider) reload() error {
	f, err := os.Open(filepath.Join(p.dir, "push-config.json"))
	if err != nil {
		return errors.New("apns: provider is not configured; run mygo push setup")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 8193))
	var cfg ProviderConfig
	if err != nil || len(raw) > 8192 || json.Unmarshal(raw, &cfg) != nil {
		return errors.New("apns: invalid provider configuration")
	}
	if err := cfg.validate(); err != nil {
		return err
	}
	if p.topic != "" && cfg.Topic != p.topic {
		return errors.New("apns: reloaded provider belongs to another application")
	}
	if (cfg.KeyFile == "") == (cfg.KeychainService == "") {
		return errors.New("apns: provider must specify exactly one credential store")
	}
	key, err := providerKey(cfg)
	if err != nil {
		return err
	}
	h := sha256.New()
	identity, _ := json.Marshal([]string{cfg.TeamID, cfg.KeyID, string(cfg.Environment)})
	h.Write(identity)
	h.Write(key)
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	if p.client != nil && digest == p.digest {
		return nil
	}
	client, err := New(Config{TeamID: cfg.TeamID, KeyID: cfg.KeyID, PrivateKey: key, Environment: cfg.Environment})
	if err != nil {
		return err
	}
	if p.client != nil {
		p.client.http.CloseIdleConnections()
	}
	p.client, p.topic, p.digest = client, cfg.Topic, digest
	return nil
}

// Send fills the topic from provider metadata. An explicitly different topic
// is rejected; use a separate provider for another application.
func (p *Provider) Send(ctx context.Context, n Notification) (Response, error) {
	p.mu.Lock()
	client, topic := p.client, p.topic
	p.mu.Unlock()
	if client == nil {
		return Response{}, errors.New("apns: provider is not configured")
	}
	if n.Topic != "" && n.Topic != topic {
		return Response{}, errors.New("apns: notification topic differs from configured application")
	}
	n.Topic = topic
	return client.Send(ctx, n)
}
