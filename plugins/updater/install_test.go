package updater

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstallUpdateWithoutWindowChecksSelectedVersion(t *testing.T) {
	for _, mode := range []string{"success", "changed", "check failure", "install failure"} {
		t.Run(mode, func(t *testing.T) {
			h := newHarness(t, Options{})
			var installs atomic.Int32
			h.result = func(context.Context) (*release, error) {
				if mode == "check failure" {
					return nil, errors.New("offline")
				}
				version := "2.0.0"
				if mode == "changed" {
					version = "3.0.0"
				}
				return newRelease(version, func(context.Context, func(int64, int64)) error {
					installs.Add(1)
					if mode == "install failure" {
						return errors.New("signature mismatch")
					}
					return nil
				}), nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			err := h.u.installUpdate(ctx, "2.0.0")
			if mode == "success" {
				if err != nil || installs.Load() != 1 {
					t.Fatal(installs.Load(), err)
				}
			} else if err == nil || errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("missing immediate update failure", err)
			}
			if mode == "changed" || mode == "check failure" {
				if installs.Load() != 0 {
					t.Fatal("unchecked release installed")
				}
			}
			h.noWindow()
			if h.relaunches.Load() != 0 {
				t.Fatal("installer relaunched before remote response")
			}
		})
	}
}

func TestInstallUpdateJoinsNativePromptAndDownload(t *testing.T) {
	h := newHarness(t, Options{})
	proceed := make(chan struct{})
	var installs atomic.Int32
	h.result = func(context.Context) (*release, error) {
		return newRelease("2.0.0", func(ctx context.Context, progress func(int64, int64)) error {
			installs.Add(1)
			progress(1, 2)
			select {
			case <-proceed:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}), nil
	}
	done := h.begin(true)
	s := h.shown()
	waitView(t, s, "A new version of")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	one, two := make(chan error, 1), make(chan error, 1)
	go func() { one <- h.u.installUpdate(ctx, "2.0.0") }()
	waitView(t, s, "Downloading")
	go func() { two <- h.u.installUpdate(ctx, "2.0.0") }()
	close(proceed)
	if err := <-one; err != nil {
		t.Fatal(err)
	}
	if err := <-two; err != nil {
		t.Fatal(err)
	}
	if installs.Load() != 1 {
		t.Fatal("native and remote requests duplicated install", installs.Load())
	}
	v := waitView(t, s, "Ready to Relaunch")
	s.Respond(v.Prompt, actionLater, false)
	waitDone(t, done)
}

func TestInstallUpdateCancelledWaiterKeepsLocalDownload(t *testing.T) {
	h := newHarness(t, Options{})
	proceed := make(chan struct{})
	h.result = func(context.Context) (*release, error) {
		return newRelease("2.0.0", func(ctx context.Context, progress func(int64, int64)) error {
			progress(1, 2)
			select {
			case <-proceed:
				return nil
			case <-ctx.Done():
				t.Error("remote waiter canceled native install")
				return ctx.Err()
			}
		}), nil
	}
	done := h.begin(true)
	s := h.shown()
	v := waitView(t, s, "A new version of")
	s.Respond(v.Prompt, actionInstall, false)
	waitView(t, s, "Downloading")
	ctx, cancel := context.WithCancel(context.Background())
	answer := make(chan error, 1)
	go func() { answer <- h.u.installUpdate(ctx, "2.0.0") }()
	cancel()
	if err := <-answer; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	close(proceed)
	v = waitView(t, s, "Ready to Relaunch")
	s.Respond(v.Prompt, actionLater, false)
	waitDone(t, done)
	if strings.Contains(v.Detail, "canceled") {
		t.Fatal(v.Detail)
	}
}

func TestTwoControllersCannotAnswerUnchangedPromptTwice(t *testing.T) {
	h := newHarness(t, Options{})
	s := newSession(h.u, h.u.text(), false)
	s.set(h.u.text().availableView(newRelease("2.0.0", nil), false))
	v, _ := s.Current()
	s.Respond(v.Prompt, actionInstall, false)
	fresh, _ := s.Current()
	s.Respond(fresh.Prompt, actionInstall, false)
	if len(s.responses) != 1 {
		t.Fatal("two controllers accepted the same available prompt", len(s.responses))
	}
	s.close()
}
