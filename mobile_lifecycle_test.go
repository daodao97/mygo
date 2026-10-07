package mygo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/internal/platform"
)

func TestPersistStateConcurrentCancellation(t *testing.T) {
	// A regressed cancellation deadlocks the fake UI loop, including test
	// cleanup. Isolate it so the regression fails within a bounded timeout.
	if os.Getenv("MYGO_TEST_STATE_CANCEL") != "1" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestPersistStateConcurrentCancellation$", "-test.timeout=5s")
		cmd.Env = append(os.Environ(), "MYGO_TEST_STATE_CANCEL=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("concurrent cancellation: %v\n%s", err, out)
		}
		return
	}
	isolateAppState(t)
	value := 1
	off, err := PersistState("concurrent", &value)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan struct{})
	onMain(func() {
		go func() { off(); close(finished) }()
		deadline := time.Now().Add(time.Second)
		for {
			loop.mu.Lock()
			queued := len(loop.queue) > 0
			loop.mu.Unlock()
			if queued {
				break
			}
			if time.Now().After(deadline) {
				t.Error("background cancellation did not queue its UI operation")
				return
			}
			runtime.Gosched()
		}
		// The UI thread can finish cancellation even while another caller
		// waits for its queued call. Neither call may remove a later binding.
		off()
		if _, err := PersistState("concurrent", &value); err != nil {
			t.Error(err)
		}
	})
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("background cancellation did not finish")
	}
	if live := onMainValue(func() bool { return App.state.bindings["concurrent"] != nil }); !live {
		t.Fatal("old cancellation removed a newly registered value")
	}
}

func isolateAppState(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldDir, _ := App.Path(PathUserData)
	old := onMainValue(func() appStateStore { s := App.state; App.state = appStateStore{}; return s })
	App.SetPath(PathUserData, dir)
	t.Cleanup(func() { onMain(func() { App.state = old }); App.SetPath(PathUserData, oldDir) })
	return filepath.Join(dir, appStateFile)
}

func TestPersistStateRestorationAndRegistration(t *testing.T) {
	file := isolateAppState(t)
	type draft struct {
		Text   string
		Route  string
		Scroll float64
	}
	value := draft{"你好 👋", "/draft", 123.5}
	off, err := PersistState("draft", &value)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := PersistState("draft", &value); err == nil {
		t.Fatal("duplicate registration accepted")
	}
	if err := App.SaveState(); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(file)
	if err := App.SaveState(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(file)
	if !os.SameFile(before, after) {
		t.Error("unchanged state replaced its file")
	}
	off()
	// Re-registering restores the saved value. An old cancellation must not
	// remove the new registration when called a second time.
	restored := draft{}
	off2, err := PersistState("draft", &restored)
	if err != nil {
		t.Fatal(err)
	}
	defer off2()
	if restored != value {
		t.Fatalf("restored = %+v, want %+v", restored, value)
	}
	off()
	onMain(func() { restored.Text = "updated" })
	if err := App.SaveState(); err != nil {
		t.Fatal(err)
	}
	onMain(func() { App.state = appStateStore{} }) // simulate a new process
	final := draft{}
	if _, err := PersistState("draft", &final); err != nil {
		t.Fatal(err)
	}
	if final.Text != "updated" || final.Scroll != 123.5 {
		t.Fatalf("new launch = %+v", final)
	}
	info, _ := os.Stat(file)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("snapshot permissions = %v", info.Mode())
	}
}

func TestPersistStateFailedSnapshotPreservesFile(t *testing.T) {
	file := isolateAppState(t)
	initial := []byte(`{"Version":1,"Values":{"retired":{"keep":true},"draft":"original"}}`)
	if err := os.WriteFile(file, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	var draft string
	if _, err := PersistState("draft", &draft); err != nil {
		t.Fatal(err)
	}
	if draft != "original" {
		t.Fatalf("draft = %q", draft)
	}
	onMain(func() { draft = "saved" })
	if err := App.SaveState(); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(file)
	if !bytes.Contains(saved, []byte(`"retired":{"keep":true}`)) {
		t.Fatal("unregistered saved key lost")
	}
	bad := make(chan int)
	off, err := PersistState("bad", &bad)
	if err != nil {
		t.Fatal(err)
	}
	onMain(func() { draft = "must not replace previous snapshot" })
	if err := App.SaveState(); err == nil {
		t.Fatal("non-JSON value accepted")
	}
	got, _ := os.ReadFile(file)
	if !bytes.Equal(got, saved) {
		t.Fatal("failed snapshot overwrote the saved file")
	}
	off()
	if err := App.SaveState(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(file))
	if len(entries) != 1 {
		t.Fatalf("leftover temporary files: %v", entries)
	}
}

func TestPersistStateCorruption(t *testing.T) {
	file := isolateAppState(t)
	for _, data := range []string{"", "{broken", `{"Version":2,"Values":{}}`, `{"Version":1,"Values":{"draft":{"Count":"bad"}}}`} {
		t.Run(data, func(t *testing.T) {
			onMain(func() { App.state = appStateStore{} })
			if err := os.WriteFile(file, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			value := struct{ Count int }{7}
			if _, err := PersistState("draft", &value); err == nil {
				t.Fatal("invalid state accepted")
			}
			if value.Count != 7 {
				t.Fatal("failed restoration mutated live value")
			}
			if err := App.SaveState(); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(file)
			if string(got) != data {
				t.Fatal("damaged state overwritten")
			}
		})
	}
}

func TestBackgroundCheckpointAndLifecycle(t *testing.T) {
	file := isolateAppState(t)
	old := App.Lifecycle()
	t.Cleanup(func() { onMain(func() { App.mu.Lock(); App.lifecycle = old; App.mu.Unlock() }) })
	var states []LifecycleState
	off := App.OnLifecycleChanged(func(s LifecycleState) { states = append(states, s) })
	defer off()
	value := struct{ Draft string }{"before"}
	if _, err := PersistState("draft", &value); err != nil {
		t.Fatal(err)
	}
	backgrounds, foregrounds := 0, 0
	offBG := App.OnDidEnterBackground(func() { backgrounds++; value.Draft = "from background handler" })
	defer offBG()
	offFG := App.OnWillEnterForeground(func() { foregrounds++ })
	defer offFG()
	onMain(func() {
		h := appHandler{}
		h.DidBecomeActive()
		states = nil
		h.DidResignActive()
		h.DidResignActive() // duplicate OS callbacks are harmless
		h.DidEnterBackground()
		h.DidEnterBackground()
	})
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct{ Values map[string]json.RawMessage }
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(snapshot.Values["draft"], []byte("from background handler")) {
		t.Fatal("checkpoint preceded background handlers")
	}
	onMain(func() { appHandler{}.WillEnterForeground(); appHandler{}.DidBecomeActive() })
	onMain(func() {
		want := []LifecycleState{LifecycleInactive, LifecycleBackground, LifecycleInactive, LifecycleActive}
		if !reflect.DeepEqual(states, want) || backgrounds != 1 || foregrounds != 1 {
			t.Errorf("lifecycle = %v, background %d, foreground %d", states, backgrounds, foregrounds)
		}
	})
	before, _ := os.ReadFile(file)
	bad := make(chan int)
	if _, err := PersistState("bad", &bad); err != nil {
		t.Fatal(err)
	}
	failures := make(chan error, 1)
	offError := App.OnStateSaveError(func(err error) { failures <- err })
	defer offError()
	onMain(func() { appHandler{}.DidEnterBackground() })
	select {
	case err := <-failures:
		if err == nil {
			t.Fatal("background save reported nil error")
		}
	case <-time.After(time.Second):
		t.Fatal("background save failure was not reported")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) {
		t.Fatal("failed background save changed the snapshot")
	}
}

func TestSystemManagedCloseAndQuit(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	var closeEvents, quitEvents atomic.Int32
	offClose := w.OnClose(func(*CloseEvent) { closeEvents.Add(1) })
	defer offClose()
	offQuit := App.OnBeforeQuit(func(*QuitEvent) { quitEvents.Add(1) })
	defer offQuit()
	fb.SystemManaged.Store(true)
	t.Cleanup(func() { fb.SystemManaged.Store(false) })
	if err := w.TryClose(); !errors.Is(err, ErrUnsupported) {
		t.Errorf("TryClose = %v", err)
	}
	if err := App.TryQuit(); !errors.Is(err, ErrUnsupported) {
		t.Errorf("TryQuit = %v", err)
	}
	w.Close()
	w.Destroy()
	App.Quit()
	App.Relaunch()
	App.Exit(23)
	onMain(func() {
		if (appHandler{}).QuitRequested() {
			t.Error("native quit request accepted")
		}
		if fw.H.ShouldClose() {
			t.Error("native window close request accepted")
		}
		if w.native == nil || App.quitting || App.relaunch {
			t.Error("system-managed app was closed")
		}
	})
	if w.IsDestroyed() || closeEvents.Load() != 0 || quitEvents.Load() != 0 {
		t.Fatal("unsupported close/quit emitted desktop events")
	}
	// This synchronous UI call also proves the Go dispatch loop is still alive.
	w.SetTitle("still running")
	if w.Title() != "still running" {
		t.Fatal("dispatch loop stopped")
	}
}

func TestFirstFrameNotification(t *testing.T) {
	w, fw := testWindow(t, WindowOptions{})
	var calls atomic.Int32
	off := w.OnFirstFrame(func() { calls.Add(1) })
	defer off()
	canceled := w.OnFirstFrame(func() { t.Error("canceled first-frame callback ran") })
	canceled()
	onMain(func() {
		fw.H.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfacePresented})
		fw.H.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfacePresented})
	})
	if calls.Load() != 1 {
		t.Fatalf("first-frame calls = %d", calls.Load())
	}
	late := make(chan struct{}, 2)
	w.OnFirstFrame(func() { late <- struct{}{} })
	select {
	case <-late:
	case <-time.After(time.Second):
		t.Fatal("late subscriber was not called")
	}
	onMain(func() { fw.H.SurfaceEvent(platform.SurfaceEvent{Kind: platform.SurfacePresented}) })
	select {
	case <-late:
		t.Fatal("late subscriber called twice")
	default:
	}
	onMain(func() {
		off := w.OnFirstFrame(func() { t.Error("canceled late callback ran") })
		off()
	})
	onMain(func() {})
}
