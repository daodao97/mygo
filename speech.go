package mygo

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

// SpeechOptions configures live speech-to-text. Language is a BCP-47 tag;
// empty uses the system locale. OnDevice forbids Apple's network recognition.
// ContextualStrings are hints for names and vocabulary, not text to insert.
type SpeechOptions struct {
	Provider          string
	Language          string
	OnDevice          bool
	ContextualStrings []string
	// MaxDuration defaults to 60 seconds. On expiry capture finishes normally.
	MaxDuration time.Duration
}

// SpeechResult replaces the complete transcript. Final ends the session;
// an error also ends it and may retain a provisional transcript in Text.
type SpeechResult struct {
	Text  string
	Final bool
	Err   error
}

// SpeechRecording controls an active provider. Methods must be goroutine-safe,
// idempotent, and safe after completion. Cancel releases all capture resources.
type SpeechRecording interface {
	Stop()
	Cancel()
}

// SpeechProvider implements a speech-to-text service. Start honors ctx while
// preparing capture. emit may run on any goroutine, including during Start;
// after Final or Err the provider releases its recording resources.
type SpeechProvider interface {
	Start(context.Context, SpeechOptions, func(SpeechResult)) (SpeechRecording, error)
}

type SpeechModule struct{}

var Speech SpeechModule
var speechProviders = struct {
	sync.RWMutex
	m map[string]SpeechProvider
}{m: make(map[string]SpeechProvider)}

// RegisterProvider adds a provider, e.g. a cloud service implemented in Go.
// The built-in "apple" provider uses iOS Speech and AVAudioEngine; other
// backends return ErrUnsupported. Providers own authentication and capture.
//
// Deprecated: apps call speech.RegisterProvider (plugins/speech), which calls this.
func (SpeechModule) RegisterProvider(name string, provider SpeechProvider) error {
	if name == "" || strings.TrimSpace(name) != name || name == "apple" || provider == nil {
		return errors.New("mygo: invalid speech provider")
	}
	speechProviders.Lock()
	defer speechProviders.Unlock()
	if speechProviders.m[name] != nil {
		return fmt.Errorf("mygo: speech provider %q already registered", name)
	}
	speechProviders.m[name] = provider
	return nil
}

// Start requests authorization and begins capture. It can wait for permission:
// start it from a worker goroutine for press-and-hold UI. Cancel ctx when the
// user releases before authorization completes, navigates away or backgrounds.
// No audio is saved. Default Provider is "apple"; no cloud fallback is implied.
//
// Deprecated: apps call speech.Start (plugins/speech), which calls this.
func (SpeechModule) Start(ctx context.Context, o SpeechOptions) (*SpeechSession, error) {
	if ctx == nil {
		return nil, errors.New("mygo: speech needs a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if o.MaxDuration == 0 {
		o.MaxDuration = 60 * time.Second
	}
	if o.MaxDuration < 0 || o.MaxDuration > 10*time.Minute || len(o.ContextualStrings) > 100 {
		return nil, errors.New("mygo: invalid speech options")
	}
	o.ContextualStrings = append([]string(nil), o.ContextualStrings...)
	var provider SpeechProvider = appleSpeechProvider{}
	if o.Provider != "" && o.Provider != "apple" {
		speechProviders.RLock()
		provider = speechProviders.m[o.Provider]
		speechProviders.RUnlock()
		if provider == nil {
			return nil, fmt.Errorf("mygo: unknown speech provider %q", o.Provider)
		}
	}
	s := &SpeechSession{results: make(chan SpeechResult, 1), done: make(chan struct{})}
	recording, err := provider.Start(ctx, o, s.emit)
	if err != nil {
		if recording != nil {
			recording.Cancel()
		}
		s.Cancel()
		return nil, err
	}
	if recording == nil {
		s.Cancel()
		return nil, errors.New("mygo: speech provider returned no recording")
	}
	s.mu.Lock()
	s.recording = recording
	finished := s.finished
	s.mu.Unlock()
	if finished {
		recording.Cancel()
	}
	go func() {
		timer := time.NewTimer(o.MaxDuration)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			s.Cancel()
			return
		case <-s.done:
			return
		case <-timer.C:
			s.Stop()
		}
		select {
		case <-ctx.Done():
			s.Cancel()
		case <-s.done:
		}
	}()
	return s, nil
}

// SpeechSession owns a single capture. Results is a coalescing stream: slow
// readers get the newest complete transcript; the final result is never lost.
type SpeechSession struct {
	mu                sync.Mutex
	results           chan SpeechResult
	done              chan struct{}
	recording         SpeechRecording
	finished, stopped bool
}

func (s *SpeechSession) Results() <-chan SpeechResult { return s.results }
func (s *SpeechSession) emit(r SpeechResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	if r.Err != nil {
		r.Final = true
	}
	select {
	case <-s.results:
	default:
	}
	s.results <- r
	if r.Final {
		s.finished = true
		close(s.results)
		close(s.done)
	}
}
func (s *SpeechSession) Stop() {
	s.mu.Lock()
	if s.finished || s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	r := s.recording
	s.mu.Unlock()
	if r != nil {
		r.Stop()
	}
}
func (s *SpeechSession) Cancel() {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	select {
	case <-s.results:
	default:
	}
	s.results <- SpeechResult{Final: true, Err: context.Canceled}
	close(s.results)
	close(s.done)
	r := s.recording
	s.mu.Unlock()
	if r != nil {
		r.Cancel()
	}
}

// Authorize requests Apple's microphone and speech permissions without starting
// capture. Call it when opening a voice input panel so authorization dialogs do
// not interrupt a press-and-hold gesture. Custom providers own their permissions.
//
// Deprecated: apps call speech.Authorize (plugins/speech), which calls this.
func (SpeechModule) Authorize(ctx context.Context) error {
	if ctx == nil {
		return errors.New("mygo: speech needs a context")
	}
	needsApp("Speech.Authorize")
	for _, permission := range []Permission{PermissionMicrophone, PermissionSpeech} {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, err := Permissions.Request(permission)
		if err != nil {
			return err
		}
		if status != PermissionGranted {
			return fmt.Errorf("mygo: %s permission %s", permission, status)
		}
	}
	return ctx.Err()
}

type appleSpeechProvider struct{}

func (appleSpeechProvider) Start(ctx context.Context, o SpeechOptions, emit func(SpeechResult)) (SpeechRecording, error) {
	if err := Speech.Authorize(ctx); err != nil {
		return nil, err
	}
	var recording platform.SpeechRecording
	var err error
	onMain(func() {
		if err = ctx.Err(); err != nil {
			return
		}
		recording, err = speechBackend().Start(platform.SpeechOptions{Language: o.Language, OnDevice: o.OnDevice, ContextualStrings: o.ContextualStrings}, func(r platform.SpeechResult) {
			emit(SpeechResult{Text: r.Text, Final: r.Final, Err: r.Err})
		})
	})
	if err != nil {
		if recording != nil {
			recording.Cancel()
		}
		return nil, err
	}
	if recording == nil {
		return nil, errors.New("mygo: native speech returned no recording")
	}
	return nativeSpeechRecording{recording}, nil
}

type nativeSpeechRecording struct{ recording platform.SpeechRecording }

func (r nativeSpeechRecording) Stop()   { onMain(r.recording.Stop) }
func (r nativeSpeechRecording) Cancel() { onMain(r.recording.Cancel) }
