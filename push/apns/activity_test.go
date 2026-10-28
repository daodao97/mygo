package apns

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/egoist/mygo/internal/wait"
)

type activitySenderFunc func(context.Context, Notification) (Response, error)

func (f activitySenderFunc) Send(ctx context.Context, n Notification) (Response, error) {
	return f(ctx, n)
}

func activityRegistration(id string) ActivityRegistration {
	return ActivityRegistration{ID: id, DeviceToken: strings.Repeat("a", 64), ResourceID: "resource"}
}

func activityManager(t *testing.T, sender ActivitySender, options ActivityManagerOptions) *ActivityManager {
	t.Helper()
	m, err := NewActivityManager(sender, options)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func activityTarget(t *testing.T, m *ActivityManager) ActivityTarget {
	t.Helper()
	targets := m.Targets()
	if len(targets) != 1 {
		t.Fatalf("targets=%v, want one", targets)
	}
	return targets[0]
}

func TestActivityManagerDeduplicatesRefreshesAndEnds(t *testing.T) {
	var notices []Notification
	m := activityManager(t, activitySenderFunc(func(_ context.Context, n Notification) (Response, error) {
		notices = append(notices, n)
		return Response{}, nil
	}), ActivityManagerOptions{Topic: "dev.example.app"})
	now := time.Unix(1_700_000_000, 0)
	m.now = func() time.Time { return now }
	if err := m.Register(activityRegistration("activity")); err != nil {
		t.Fatal(err)
	}
	target := activityTarget(t, m)
	update := ActivityUpdate{ContentState: json.RawMessage(`{"status":"working","updatedAt":1}`), DeduplicationKey: "working"}
	publish := func() {
		t.Helper()
		if err := m.Publish(t.Context(), target, update); err != nil {
			t.Fatal(err)
		}
	}
	publish()
	now = now.Add(time.Second)
	// A repeated registration and a changed display timestamp are the same event.
	if err := m.Register(activityRegistration("activity")); err != nil {
		t.Fatal(err)
	}
	update.ContentState = json.RawMessage(`{"status":"working","updatedAt":2}`)
	publish()
	if len(notices) != 1 {
		t.Fatal("duplicate state was sent", len(notices))
	}
	now = now.Add(time.Minute - time.Second)
	publish()
	if len(notices) != 2 {
		t.Fatal("unchanged activity did not refresh")
	}
	last := notices[1]
	if last.Topic != "dev.example.app" || last.Payload.LiveActivity.Timestamp != now.Unix() ||
		last.Payload.LiveActivity.StaleDate != now.Add(2*time.Minute).Unix() || !last.Expiration.Equal(now.Add(2*time.Minute)) {
		t.Fatal("delivery policy not applied", last)
	}
	// The same content must still be sent when its lifecycle changes to end.
	update.End = true
	publish()
	if len(notices) != 3 || notices[2].Payload.LiveActivity.Event != "end" ||
		notices[2].Payload.LiveActivity.DismissalDate != now.Add(10*time.Minute).Unix() || len(m.Targets()) != 0 {
		t.Fatal("end did not remove the activity", notices)
	}
	if err := m.Publish(t.Context(), target, update); !errors.Is(err, ErrActivityChanged) {
		t.Fatal(err)
	}
}

func TestActivityManagerCompactsContentAndRetriesFailures(t *testing.T) {
	var sends int
	failure := errors.New("offline")
	m := activityManager(t, activitySenderFunc(func(_ context.Context, _ Notification) (Response, error) {
		sends++
		return Response{}, failure
	}), ActivityManagerOptions{})
	m.Register(activityRegistration("activity"))
	target := activityTarget(t, m)
	update := ActivityUpdate{ContentState: json.RawMessage(`{ "value": 1 }`)}
	if err := m.Publish(t.Context(), target, update); err == nil {
		t.Fatal("lost transport failure")
	}
	failure = nil
	if err := m.Publish(t.Context(), target, update); err != nil {
		t.Fatal(err)
	}
	update.ContentState = json.RawMessage(`{"value":1}`)
	if err := m.Publish(t.Context(), target, update); err != nil {
		t.Fatal(err)
	}
	if sends != 2 {
		t.Fatal("failed send was acknowledged or whitespace was treated as new state", sends)
	}
	update.ContentState = json.RawMessage(`{"value":2}`)
	if err := m.Publish(t.Context(), target, update); err != nil {
		t.Fatal(err)
	}
	if sends != 3 {
		t.Fatal("changed content was suppressed")
	}
	update.End, failure = true, errors.New("offline")
	if err := m.Publish(t.Context(), target, update); err == nil || len(m.Targets()) != 1 {
		t.Fatal("failed end discarded a retryable registration", err)
	}
	failure = nil
	if err := m.Publish(t.Context(), target, update); err != nil || len(m.Targets()) != 0 {
		t.Fatal("end retry did not remove the registration", err)
	}
}

func TestActivityManagerRegistrationBoundsAndExpiry(t *testing.T) {
	m := activityManager(t, activitySenderFunc(func(context.Context, Notification) (Response, error) { return Response{}, nil }), ActivityManagerOptions{MaxActivities: 1, Lifetime: time.Hour})
	now := time.Unix(1_700_000_000, 0)
	m.now = func() time.Time { return now }
	first := activityRegistration("first")
	if err := m.Register(first); err != nil {
		t.Fatal(err)
	}
	old := activityTarget(t, m)
	if err := m.Register(activityRegistration("second")); !errors.Is(err, ErrActivityLimit) {
		t.Fatal(err)
	}
	first.DeviceToken = strings.Repeat("b", 64)
	if err := m.Register(first); err != nil {
		t.Fatal("rotation at capacity rejected", err)
	}
	if err := m.Publish(t.Context(), old, ActivityUpdate{ContentState: json.RawMessage(`{}`)}); !errors.Is(err, ErrActivityChanged) {
		t.Fatal("old token target remained valid", err)
	}
	rotated := activityTarget(t, m)
	now = now.Add(30 * time.Minute)
	m.Register(first)
	now = now.Add(30 * time.Minute)
	if len(m.Targets()) != 0 {
		t.Fatal("duplicate registration extended original expiry")
	}
	if err := m.Register(activityRegistration("second")); err != nil {
		t.Fatal("expired registration still consumed capacity", err)
	}
	if err := m.Publish(t.Context(), rotated, ActivityUpdate{ContentState: json.RawMessage(`{}`)}); !errors.Is(err, ErrActivityChanged) {
		t.Fatal(err)
	}
	m.Remove("second")
	m.Register(first)
	current := activityTarget(t, m)
	if current.revision == rotated.revision {
		t.Fatal("removed ID reused an old target version")
	}
	first.ResourceID = "different-resource"
	m.Register(first)
	if err := m.Publish(t.Context(), current, ActivityUpdate{ContentState: json.RawMessage(`{}`)}); !errors.Is(err, ErrActivityChanged) {
		t.Fatal("old routing snapshot sent to replacement resource", err)
	}
}

func TestActivityManagerPermanentErrorsRespectTokenRefresh(t *testing.T) {
	for _, reason := range []string{"Unregistered", "BadDeviceToken", "DeviceTokenNotForTopic", "TooManyRequests"} {
		t.Run(reason, func(t *testing.T) {
			failure := &Error{Status: 410, Reason: reason}
			m := activityManager(t, activitySenderFunc(func(context.Context, Notification) (Response, error) { return Response{}, failure }), ActivityManagerOptions{})
			m.Register(activityRegistration("activity"))
			if err := m.Publish(t.Context(), activityTarget(t, m), ActivityUpdate{ContentState: json.RawMessage(`{}`)}); !errors.Is(err, failure) {
				t.Fatal(err)
			}
			want := 0
			if reason == "TooManyRequests" {
				want = 1
			}
			if len(m.Targets()) != want {
				t.Fatal("wrong failure retention policy", m.Targets())
			}
		})
	}
	now := time.Unix(1_700_000_000, 0)
	failure := &Error{Status: 410, Reason: "Unregistered", Timestamp: now.Add(-time.Second)}
	m := activityManager(t, activitySenderFunc(func(context.Context, Notification) (Response, error) { return Response{}, failure }), ActivityManagerOptions{})
	m.now = func() time.Time { return now }
	m.Register(activityRegistration("activity"))
	m.Publish(t.Context(), activityTarget(t, m), ActivityUpdate{ContentState: json.RawMessage(`{}`)})
	if len(m.Targets()) != 1 {
		t.Fatal("old unregistration removed a freshly registered token")
	}
}

func TestActivityManagerRotationDuringSendKeepsNewReceipt(t *testing.T) {
	for _, oldFailure := range []error{nil, &Error{Status: 410, Reason: "Unregistered"}} {
		t.Run(map[bool]string{true: "old failure", false: "old success"}[oldFailure != nil], func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var sends atomic.Int32
			m := activityManager(t, activitySenderFunc(func(_ context.Context, n Notification) (Response, error) {
				sends.Add(1)
				if n.DeviceToken == strings.Repeat("a", 64) {
					close(entered)
					<-release
					return Response{}, oldFailure
				}
				return Response{}, nil
			}), ActivityManagerOptions{})
			m.Register(activityRegistration("activity"))
			old := activityTarget(t, m)
			update := ActivityUpdate{ContentState: json.RawMessage(`{"status":"working"}`)}
			finished := make(chan error, 1)
			go func() { finished <- m.Publish(wait.Context(t), old, update) }()
			wait.For(t, "old send started", entered)
			rotated := activityRegistration("activity")
			rotated.DeviceToken = strings.Repeat("b", 64)
			m.Register(rotated)
			current := activityTarget(t, m)
			if err := m.Publish(t.Context(), current, update); err != nil {
				t.Fatal(err)
			}
			close(release)
			wait.For(t, "old send finished", finished)
			if err := m.Publish(t.Context(), current, update); err != nil {
				t.Fatal(err)
			}
			if sends.Load() != 2 || len(m.Targets()) != 1 {
				t.Fatal("old reply removed new token or overwrote its receipt", sends.Load())
			}
		})
	}
}

func TestActivityManagerConcurrentDuplicatesAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, release := make(chan struct{}), make(chan struct{})
		var sends atomic.Int32
		m := activityManager(t, activitySenderFunc(func(context.Context, Notification) (Response, error) {
			if sends.Add(1) == 1 {
				close(entered)
				<-release
			}
			return Response{}, nil
		}), ActivityManagerOptions{})
		m.Register(activityRegistration("activity"))
		target, update := activityTarget(t, m), ActivityUpdate{ContentState: json.RawMessage(`{}`)}
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				if err := m.Publish(t.Context(), target, update); err != nil {
					t.Error(err)
				}
			})
		}
		<-entered
		ctx, cancel := context.WithCancel(t.Context())
		finished := make(chan error, 1)
		go func() { finished <- m.Publish(ctx, target, update) }()
		synctest.Wait()
		cancel()
		synctest.Wait()
		if err := <-finished; !errors.Is(err, context.Canceled) {
			t.Fatal("queued publication ignored cancellation", err)
		}
		close(release)
		wg.Wait()
		if sends.Load() != 1 {
			t.Fatal("concurrent identical state was sent more than once", sends.Load())
		}
	})
}

func TestActivityManagerValidatesBeforeSending(t *testing.T) {
	var sends int
	sender := activitySenderFunc(func(context.Context, Notification) (Response, error) { sends++; return Response{}, nil })
	if _, err := NewActivityManager(nil, ActivityManagerOptions{}); err == nil {
		t.Fatal("nil sender accepted")
	}
	for _, options := range []ActivityManagerOptions{{MaxActivities: -1}, {Lifetime: -1}, {SendTimeout: -1}, {Topic: "invalid\ntopic"}} {
		if _, err := NewActivityManager(sender, options); err == nil {
			t.Fatal("invalid policy accepted", options)
		}
	}
	m := activityManager(t, sender, ActivityManagerOptions{})
	for _, r := range []ActivityRegistration{{}, {ID: "id", DeviceToken: "no token"}, {ID: strings.Repeat("x", 257), DeviceToken: "aa"}} {
		if err := m.Register(r); err == nil {
			t.Fatal("invalid registration accepted")
		}
	}
	m.Register(activityRegistration("activity"))
	target := activityTarget(t, m)
	for _, content := range []string{"", "null", "[]", "{", `{"text":"` + strings.Repeat("x", 4096) + `"}`} {
		if err := m.Publish(t.Context(), target, ActivityUpdate{ContentState: json.RawMessage(content)}); err == nil {
			t.Fatal("invalid content accepted")
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := m.Publish(ctx, target, ActivityUpdate{ContentState: json.RawMessage(`{}`)}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if sends != 0 {
		t.Fatal("invalid or canceled updates reached the sender")
	}
}

func TestActivityManagerBoundsAnInFlightSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := activityManager(t, activitySenderFunc(func(ctx context.Context, _ Notification) (Response, error) {
			<-ctx.Done()
			return Response{}, ctx.Err()
		}), ActivityManagerOptions{SendTimeout: time.Second})
		m.Register(activityRegistration("activity"))
		err := m.Publish(t.Context(), activityTarget(t, m), ActivityUpdate{ContentState: json.RawMessage(`{}`)})
		if !errors.Is(err, context.DeadlineExceeded) || len(m.Targets()) != 1 {
			t.Fatal("send was unbounded or lost its retryable registration", err)
		}
	})
}

func TestAPNsProductionDependenciesAreStandardLibraryOnly(t *testing.T) {
	cmd := exec.CommandContext(wait.Context(t), "go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect APNs dependencies: %v\n%s", err, output)
	}
	if got := strings.Fields(string(output)); len(got) != 1 || got[0] != "github.com/egoist/mygo/push/apns" {
		t.Fatalf("APNs pulled in non-standard dependencies: %s", output)
	}
}
