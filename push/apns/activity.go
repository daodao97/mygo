package apns

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"
)

var (
	// ErrActivityLimit means a new registration would exceed MaxActivities.
	ErrActivityLimit = errors.New("apns: live activity registration limit reached")
	// ErrActivityChanged means a target expired, was removed, or was replaced
	// since Targets returned it. Obtain a fresh target before trying again.
	ErrActivityChanged = errors.New("apns: live activity target changed")
)

// ActivitySender is implemented by Client and Provider. Providers fill the
// application's topic; a low-level Client needs ActivityManagerOptions.Topic.
// Implementations must honor context cancellation.
type ActivitySender interface {
	Send(context.Context, Notification) (Response, error)
}

// ActivityManagerOptions configures an opt-in, in-memory subscription registry.
// Zero values select the defaults below. Durations must not be negative.
type ActivityManagerOptions struct {
	Topic           string
	MaxActivities   int           // default 256
	Lifetime        time.Duration // registration lifetime, default 8 hours
	RefreshInterval time.Duration // resend unchanged content after 1 minute
	StaleAfter      time.Duration // APNs expiration and stale date, default 2 minutes
	DismissAfter    time.Duration // dismiss an ended activity after 10 minutes
	SendTimeout     time.Duration // default 8 seconds
}

// ActivityRegistration binds a caller-owned ID to an ActivityKit push token.
// ResourceID is an optional, opaque application identifier for routing updates.
// It is never included in a push payload by the manager.
type ActivityRegistration struct {
	ID, DeviceToken, ResourceID string
}

// ActivityTarget identifies one version of a registration, without exposing
// its device token. Pass the snapshot returned by Targets to Publish.
type ActivityTarget struct {
	ID, ResourceID string
	revision       uint64
}

// ActivityUpdate contains the widget extension's Codable content-state object.
// DeduplicationKey optionally identifies the logical content independently of
// volatile fields such as a display timestamp. Otherwise compact JSON is used.
// End is part of deduplication and sends an end event instead of an update.
type ActivityUpdate struct {
	ContentState     json.RawMessage
	DeduplicationKey string
	End              bool
}

type managedActivity struct {
	registration ActivityRegistration
	revision     uint64
	registered   time.Time
	expires      time.Time
	sent         time.Time
	last         [32]byte
	gate         chan struct{}
}

// ActivityManager handles token rotation, expiry, duplicate content, and
// permanent APNs failures. It is safe from any goroutine. Construction starts
// no workers or polling timers: applications decide when to call Targets and
// Publish. Each send is bounded by SendTimeout and the caller's context.
type ActivityManager struct {
	mu       sync.Mutex
	sender   ActivitySender
	options  ActivityManagerOptions
	entries  map[string]*managedActivity
	revision uint64
	now      func() time.Time
}

// NewActivityManager constructs a headless manager around a Client, Provider,
// or application sender. It does not contact APNs or request device permission.
func NewActivityManager(sender ActivitySender, options ActivityManagerOptions) (*ActivityManager, error) {
	if sender == nil {
		return nil, errors.New("apns: live activity sender is required")
	}
	if options.MaxActivities < 0 || strings.ContainsAny(options.Topic, "\r\n\x00") {
		return nil, errors.New("apns: invalid live activity options")
	}
	if options.MaxActivities == 0 {
		options.MaxActivities = 256
	}
	for _, value := range []struct {
		field *time.Duration
		value time.Duration
	}{
		{&options.Lifetime, 8 * time.Hour},
		{&options.RefreshInterval, time.Minute},
		{&options.StaleAfter, 2 * time.Minute},
		{&options.DismissAfter, 10 * time.Minute},
		{&options.SendTimeout, 8 * time.Second},
	} {
		if *value.field < 0 {
			return nil, errors.New("apns: negative live activity duration")
		}
		if *value.field == 0 {
			*value.field = value.value
		}
	}
	return &ActivityManager{sender: sender, options: options, entries: map[string]*managedActivity{}, now: time.Now}, nil
}

// Register adds or replaces an activity. Identical registrations keep their
// delivery receipt and original expiry; a rotated token or resource replaces
// the target version and requests delivery of the next submitted state.
func (m *ActivityManager) Register(r ActivityRegistration) error {
	if r.ID == "" || len(r.ID) > 256 || len(r.ResourceID) > 256 || strings.ContainsAny(r.ID+r.ResourceID, "\r\n\x00") ||
		r.DeviceToken == "" || len(r.DeviceToken) > 512 || len(r.DeviceToken)%2 != 0 {
		return errors.New("apns: invalid live activity registration")
	}
	if _, err := hex.DecodeString(r.DeviceToken); err != nil {
		return errors.New("apns: invalid live activity token")
	}
	r.DeviceToken = strings.ToLower(r.DeviceToken)
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	m.prune(now)
	old := m.entries[r.ID]
	if old != nil && old.registration == r {
		old.registered = now
		return nil
	}
	if old == nil && len(m.entries) >= m.options.MaxActivities {
		return ErrActivityLimit
	}
	m.revision++
	m.entries[r.ID] = &managedActivity{registration: r, revision: m.revision,
		registered: now, expires: now.Add(m.options.Lifetime), gate: make(chan struct{}, 1)}
	return nil
}

// Remove unregisters an activity without sending an end event. Use Publish
// with End to also dismiss its UI according to the manager's policy.
func (m *ActivityManager) Remove(id string) {
	m.mu.Lock()
	delete(m.entries, id)
	m.mu.Unlock()
}

// Targets returns current registration versions and prunes expired entries.
// Tokens and delivery receipts stay private to the manager.
func (m *ActivityManager) Targets() []ActivityTarget {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prune(m.now())
	targets := make([]ActivityTarget, 0, len(m.entries))
	for _, e := range m.entries {
		targets = append(targets, ActivityTarget{ID: e.registration.ID, ResourceID: e.registration.ResourceID, revision: e.revision})
	}
	return targets
}

func (m *ActivityManager) prune(now time.Time) {
	for id, e := range m.entries {
		if !now.Before(e.expires) {
			delete(m.entries, id)
		}
	}
}

// Publish serializes sends to this target, skips unchanged content until the
// refresh interval, and records a delivery only after a successful send.
// Ending successfully or receiving a current permanent token error removes
// the registration. A late response cannot alter a replacement registration.
func (m *ActivityManager) Publish(ctx context.Context, target ActivityTarget, update ActivityUpdate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var compact bytes.Buffer
	if len(update.ContentState) > 4096 || json.Compact(&compact, update.ContentState) != nil || compact.Len() == 0 || compact.Bytes()[0] != '{' {
		return errors.New("apns: invalid live activity content state")
	}
	key := compact.Bytes()
	if update.DeduplicationKey != "" {
		key = []byte(update.DeduplicationKey)
	}
	// Keep only a digest of caller data in the registry. End must not be
	// skipped when it shares its final content with the preceding update.
	hash := sha256.New()
	hash.Write(key)
	if update.End {
		hash.Write([]byte{1})
	} else {
		hash.Write([]byte{0})
	}
	var digest [32]byte
	copy(digest[:], hash.Sum(nil))
	m.mu.Lock()
	m.prune(m.now())
	e := m.entries[target.ID]
	current := e != nil && e.revision == target.revision && e.registration.ResourceID == target.ResourceID
	m.mu.Unlock()
	if !current {
		return ErrActivityChanged
	}
	select {
	case e.gate <- struct{}{}:
		defer func() { <-e.gate }()
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	now := m.now()
	m.prune(now)
	if m.entries[target.ID] != e {
		m.mu.Unlock()
		return ErrActivityChanged
	}
	if !e.sent.IsZero() && e.last == digest && now.Sub(e.sent) < m.options.RefreshInterval {
		m.mu.Unlock()
		return nil
	}
	token := e.registration.DeviceToken
	m.mu.Unlock()
	payload := &LiveActivityPayload{Timestamp: now.Unix(), Event: "update", ContentState: compact.Bytes(), StaleDate: now.Add(m.options.StaleAfter).Unix()}
	if update.End {
		payload.Event, payload.DismissalDate = "end", now.Add(m.options.DismissAfter).Unix()
	}
	send, cancel := context.WithTimeout(ctx, m.options.SendTimeout)
	defer cancel()
	_, err := m.sender.Send(send, Notification{DeviceToken: token, Topic: m.options.Topic,
		Expiration: now.Add(m.options.StaleAfter), Payload: Payload{LiveActivity: payload}})
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.entries[target.ID] != e {
		return err
	}
	var failure *Error
	permanent := errors.As(err, &failure) && (failure.Unregistered() || failure.Reason == "BadDeviceToken" || failure.Reason == "DeviceTokenNotForTopic")
	if permanent && failure.Unregistered() && !failure.Timestamp.IsZero() && e.registered.After(failure.Timestamp) {
		permanent = false
	}
	if err == nil && update.End || permanent {
		delete(m.entries, target.ID)
	} else if err == nil {
		e.last, e.sent = digest, m.now()
	}
	return err
}
