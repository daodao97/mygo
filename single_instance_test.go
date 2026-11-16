//go:build !js

package mygo

import (
	"bufio"
	"encoding/json/v2"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/egoist/mygo/internal/fake"
	"github.com/egoist/mygo/internal/wait"
)

// singleInstanceTestHelper is the instance that the helper process of the
// single instance tests runs (MYGO_TEST_SECOND_INSTANCE). It prints what
// RequestSingleInstanceLock did; MYGO_TEST_INSTANCE_MODE keeps it running
// until its standard input ends: "hold" as it is, "quit" after its quit
// (OnQuit), "child" with a process it started (MYGO_TEST_INSTANCE_CHILD),
// which reads the same input.
func singleInstanceTestHelper(helper *fake.Backend) {
	mode := os.Getenv("MYGO_TEST_INSTANCE_MODE")
	if os.Getenv("MYGO_TEST_INSTANCE_WAIT") == "1" {
		singleInstanceWait = wait.Guard
		singleInstanceWaiting = func() { fmt.Println("waiting") }
	}
	if !App.RequestSingleInstanceLock() {
		fmt.Println("forwarded")
		if helper.ForegroundAllowed > 0 {
			fmt.Println("passed the foreground on")
		}
		return
	}
	switch mode {
	case "quit":
		fire(&App.onQuit) // as finish does
		fmt.Println("quit")
	case "child":
		child := exec.Command(os.Args[0], "-test.run=^$")
		child.Env = append(os.Environ(), "MYGO_TEST_SECOND_INSTANCE=", "MYGO_TEST_INSTANCE_CHILD=1")
		child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("locked")
	default:
		fmt.Println("locked")
	}
	if mode != "" {
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
}

// testInstance is a helper process running singleInstanceTestHelper.
type testInstance struct {
	cmd   *exec.Cmd
	in    *os.File // its standard input, which it reads until closed
	lines chan string
}

// startInstance starts an instance of the app named name in mode (see
// singleInstanceTestHelper), with env added to its environment.
func startInstance(t *testing.T, name, mode string, env ...string) *testInstance {
	t.Helper()
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), append([]string{
		"MYGO_TEST_SECOND_INSTANCE=1",
		"MYGO_TEST_IDENTIFIER=" + packageIdentifier,
		"MYGO_TEST_INSTANCE_NAME=" + name,
		"MYGO_TEST_INSTANCE_MODE=" + mode,
	}, env...)...)
	// Files rather than pipes that exec copies: a child of the instance
	// keeps them open after the instance exits.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inR, outW, os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	inR.Close()
	outW.Close()
	p := &testInstance{cmd: cmd, in: inW, lines: make(chan string, 16)}
	go func() {
		s := bufio.NewScanner(outR)
		for s.Scan() {
			p.lines <- s.Text()
		}
		outR.Close()
	}()
	t.Cleanup(func() {
		p.in.Close()
		_ = p.cmd.Process.Kill()
		_ = p.cmd.Wait()
	})
	return p
}

// expect waits for the next line the instance prints, which must be want.
func (p *testInstance) expect(t *testing.T, want string) {
	t.Helper()
	if got := wait.For(t, fmt.Sprintf("the instance to print %q", want), p.lines); got != want {
		t.Fatalf("the instance printed %q, want %q", got, want)
	}
}

// exit closes the instance's input, which it waits for, and waits for it
// to exit.
func (p *testInstance) exit(t *testing.T) {
	t.Helper()
	p.in.Close()
	if err := p.cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

// testInstancePaths returns the socket and the lock file of the app
// named name, which the test removes when it ends.
func testInstancePaths(t *testing.T, name string) (socket, lock string) {
	socket = singleInstanceSocket(singleInstanceKey(name))
	lock = singleInstanceLockPath(socket)
	t.Cleanup(func() {
		_ = os.Remove(socket)
		_ = os.Remove(lock)
	})
	return socket, lock
}

// lockHeld reports whether another holds the lock file at path.
func lockHeld(t *testing.T, path string) bool {
	t.Helper()
	f, err := openSingleInstanceLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if f != nil {
		f.Close()
	}
	return f == nil
}

// TestSingleInstanceLockRace checks that of the launches taking the lock
// at the same moment only one gets it, and the next one once it is
// released.
func TestSingleInstanceLockRace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.lock")
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	var won []*os.File
	for range 8 {
		wg.Go(func() {
			<-start
			f, err := openSingleInstanceLock(path)
			if err != nil {
				t.Error(err)
				return
			}
			if f != nil {
				mu.Lock()
				won = append(won, f)
				mu.Unlock()
			}
		})
	}
	close(start)
	wg.Wait()
	if len(won) != 1 {
		t.Fatalf("%d launches took the lock, want 1", len(won))
	}
	if !lockHeld(t, path) {
		t.Fatal("the lock was taken while held")
	}
	won[0].Close()
	if lockHeld(t, path) {
		t.Fatal("the lock was not free once released")
	}
}

// TestSingleInstanceRace checks that of instances started at once one
// runs and the others hand their command line over to it.
func TestSingleInstanceRace(t *testing.T) {
	name := "MyGoTest-" + t.Name()
	testInstancePaths(t, name)
	var instances []*testInstance
	for range 4 {
		instances = append(instances, startInstance(t, name, "hold"))
	}
	var got []string
	for _, p := range instances {
		got = append(got, wait.For(t, "an instance to print what it did", p.lines))
	}
	slices.Sort(got)
	if want := []string{"forwarded", "forwarded", "forwarded", "locked"}; !slices.Equal(got, want) {
		t.Fatalf("the instances printed %q, want %q", got, want)
	}
	for _, p := range instances {
		p.exit(t)
	}
}

// TestSingleInstanceWaitsForQuit checks that the lock is held after quit,
// until the process exits, and that a launch meanwhile, as Relaunch's,
// waits for it instead of handing its command line to an ending instance.
func TestSingleInstanceWaitsForQuit(t *testing.T) {
	name := "MyGoTest-" + t.Name()
	socket, lock := testInstancePaths(t, name)
	first := startInstance(t, name, "quit")
	first.expect(t, "quit")
	if !lockHeld(t, lock) {
		t.Fatal("the lock was released on quit")
	}
	if conn, err := net.Dial("unix", socket); err == nil {
		conn.Close()
		t.Fatal("an instance that quit still takes command lines")
	}
	next := startInstance(t, name, "", "MYGO_TEST_INSTANCE_WAIT=1")
	next.expect(t, "waiting")
	first.exit(t)
	next.expect(t, "locked")
	next.exit(t)
}

// TestSingleInstanceWaitsForListener checks that a launch that finds the
// lock held by an instance not listening yet hands its command line over
// once it listens.
func TestSingleInstanceWaitsForListener(t *testing.T) {
	name := "MyGoTest-" + t.Name()
	socket, lockPath := testInstancePaths(t, name)
	lock, err := openSingleInstanceLock(lockPath)
	if err != nil || lock == nil {
		t.Fatalf("lock: %v, %v", lock, err)
	}
	defer lock.Close()
	next := startInstance(t, name, "", "MYGO_TEST_INSTANCE_WAIT=1")
	next.expect(t, "waiting")
	_ = os.Remove(socket)
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	conn, err := ln.Accept()
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(conn)
	conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	var msg secondInstanceMessage
	if err := json.Unmarshal(data, &msg); err != nil || msg.WorkingDir == "" {
		t.Fatalf("handed over %q: %v", data, err)
	}
	next.expect(t, "forwarded")
	next.exit(t)
}

// TestSingleInstanceReleasedOnExit checks that the lock of an instance
// that is killed is released, although a process it started still runs.
func TestSingleInstanceReleasedOnExit(t *testing.T) {
	name := "MyGoTest-" + t.Name()
	_, lock := testInstancePaths(t, name)
	first := startInstance(t, name, "child")
	var got []string
	for range 2 {
		got = append(got, wait.For(t, "the instance and its child to start", first.lines))
	}
	slices.Sort(got)
	if want := []string{"child started", "locked"}; !slices.Equal(got, want) {
		t.Fatalf("printed %q, want %q", got, want)
	}
	if !lockHeld(t, lock) {
		t.Fatal("the lock is not held")
	}
	if err := first.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = first.cmd.Wait()
	// The child reads the input, which is still open: it runs.
	// Windows releases the locks of a process that ended soon after.
	wait.Until(t, "the lock to be released", func() bool { return !lockHeld(t, lock) })
	first.in.Close()
}
