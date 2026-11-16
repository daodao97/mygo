//go:build !js

package mygo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var singleInstance struct {
	sync.Mutex
	// lock is the open lock file, whose lock the system keeps until the
	// process exits, also when it crashes or is killed.
	lock     *os.File
	listener net.Listener
	path     string
	socket   os.FileInfo // identifies our socket file at path
	handlers listeners[func([]string, string)]
}

// singleInstanceWait is how long a launch waits for an instance that holds
// the lock but does not answer (one that is starting and not listening
// yet, or one that has quit, as for Relaunch and updates, and is ending)
// before it logs that it waits. It waits on until the lock is free.
var singleInstanceWait = 10 * time.Second

// singleInstanceWaiting, when set, is called the first time a launch finds
// the lock held and nobody answering (tests).
var singleInstanceWaiting func()

type secondInstanceMessage struct {
	Args       []string `json:"args"`
	WorkingDir string   `json:"workingDir"`
	// ActivationToken lets the first instance focus a window for this
	// launch on Wayland, which only grants the focus with the launcher's
	// token.
	ActivationToken string `json:"activationToken,omitempty"`
}

// RequestSingleInstanceLock makes sure a single instance of the app runs.
// It returns true in the first instance. In any later instance it returns
// false after forwarding its command line arguments and working directory to
// the first one (see OnSecondInstance); that instance should then exit:
//
//	if !mygo.App.RequestSingleInstanceLock() {
//		return
//	}
//
// The lock is identified by the application name and, in a packaged app,
// its identifier. It is held until the process exits, so that the work an
// app does after Run returns never overlaps with a new instance: from quit
// on, the instance takes no more command lines, and a launch meanwhile
// (Relaunch's, an update's, or the user's) waits for it to exit and runs
// then.
func (a *Application) RequestSingleInstanceLock() bool {
	if os.Getenv("MYGO_GENERATE") != "" {
		return true // Run will only write the TypeScript client.
	}
	singleInstance.Lock()
	defer singleInstance.Unlock()
	if singleInstance.lock != nil {
		if singleInstance.listener == nil {
			listenSecondInstances() // asked again after a quit
		}
		return true
	}
	path := singleInstanceSocket(singleInstanceKey(a.Name()))
	wd, _ := os.Getwd()
	// Read before GTK starts in this process: it takes the token from the
	// environment.
	token := os.Getenv("XDG_ACTIVATION_TOKEN")
	if token == "" {
		token = os.Getenv("DESKTOP_STARTUP_ID")
	}
	msg, _ := json.Marshal(secondInstanceMessage{Args: os.Args[1:], WorkingDir: wd, ActivationToken: token})
	start, said := time.Now(), false
	for delay := 5 * time.Millisecond; ; delay = min(2*delay, 100*time.Millisecond) {
		// An instance that answers takes the command line: one holding
		// the lock, or one of an older MyGo, which takes none.
		if forwardToFirstInstance(path, msg) {
			if launchedByDev() {
				// mygo dev would only see the app exit.
				log.Printf("mygo: %s is already running: handed the command line over to it (RequestSingleInstanceLock)", a.Name())
			}
			return false
		}
		// The system decides who holds the lock, before anyone touches
		// the socket, which only carries command lines to the holder.
		lock, err := openSingleInstanceLock(singleInstanceLockPath(path))
		if err != nil {
			// Locking is not possible here (e.g. read-only temp dir):
			// behave as if this were the only instance.
			return true
		}
		if lock != nil {
			singleInstance.lock, singleInstance.path = lock, path
			listenSecondInstances()
			a.OnQuit(stopSecondInstances)
			return true
		}
		if fn := singleInstanceWaiting; fn != nil {
			singleInstanceWaiting = nil
			fn()
		}
		// The holder does not answer: it is starting, or it quit and is
		// ending, as for Relaunch and updates. The wait has no end: only
		// the holder's exit frees the lock, a relaunch must land however
		// long the shutdown takes, and running beside it is what the lock
		// is there to prevent.
		if !said && time.Since(start) > singleInstanceWait {
			log.Printf("mygo: waiting for the other instance of %s to exit: it holds the single instance lock (RequestSingleInstanceLock)", a.Name())
			said = true
		}
		time.Sleep(delay)
	}
}

// openSingleInstanceLock opens the lock file at path and takes its lock,
// returning nil while another instance holds it. The file stays: removing
// it would let two instances lock two files. Its handle is not inherited
// (os.OpenFile opens with O_CLOEXEC, a handle Windows does not inherit),
// so a process the app started cannot keep the lock after the app exits.
func openSingleInstanceLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(f); err != nil {
		f.Close()
		if lockBusy(err) {
			return nil, nil
		}
		return nil, err
	}
	return f, nil
}

// forwardToFirstInstance hands msg to the instance listening at path,
// reporting whether one took it.
func forwardToFirstInstance(path string, msg []byte) bool {
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return false
	}
	// On Windows the first instance, in the background, could not bring
	// its windows to the front from OnSecondInstance: this launch, which
	// the user just started, passes its right to the foreground on before
	// it exits.
	backend().App().AllowForeground()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	_, _ = conn.Write(msg)
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}
	_, _ = io.Copy(io.Discard, conn) // wait until it was read
	conn.Close()
	return true
}

// listenSecondInstances takes the command lines of later instances on the
// socket. The lock is held, so a socket file there is a stale one, left
// by an instance that crashed. singleInstance is locked.
func listenSecondInstances() {
	path := singleInstance.path
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		// Later instances wait for this one to exit.
		log.Printf("mygo: later instances cannot hand their command line over: %v (RequestSingleInstanceLock)", err)
		return
	}
	// The socket file is removed on quit, and only if it is still ours:
	// an instance of an older MyGo, which takes no lock, may have
	// replaced it.
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	singleInstance.socket, _ = os.Stat(path)
	singleInstance.listener = ln
	go acceptSecondInstances(ln)
}

// OnSecondInstance is called in the first instance when another instance
// was started and called RequestSingleInstanceLock. Apps usually focus
// their main window here.
func (a *Application) OnSecondInstance(fn func(args []string, workingDir string)) (off func()) {
	return singleInstance.handlers.add(fn, false)
}

func acceptSecondInstances(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
			data, err := io.ReadAll(io.LimitReader(conn, 1<<20))
			if err != nil {
				return
			}
			var msg secondInstanceMessage
			if json.Unmarshal(data, &msg) != nil {
				return
			}
			postMain(func() {
				backend().App().WithActivationToken(msg.ActivationToken, func() {
					for _, fn := range singleInstance.handlers.snapshot() {
						fn(msg.Args, msg.WorkingDir)
					}
					deliverArgs(msg.Args, msg.WorkingDir)
				})
			})
		}()
	}
}

// stopSecondInstances stops taking the command lines of later instances,
// on quit, which would reach an instance that is ending. The lock stays
// until the process exits: later instances wait for it.
func stopSecondInstances() {
	singleInstance.Lock()
	defer singleInstance.Unlock()
	if singleInstance.listener != nil {
		singleInstance.listener.Close()
		if fi, err := os.Stat(singleInstance.path); err == nil && singleInstance.socket != nil && os.SameFile(fi, singleInstance.socket) {
			_ = os.Remove(singleInstance.path)
		}
		singleInstance.listener, singleInstance.socket = nil, nil
	}
}

// singleInstanceKey identifies the lock of the app named name. The
// identifier keeps the development app of mygo dev ("<identifier>.dev")
// apart from the installed app when both call SetName with the same name.
func singleInstanceKey(name string) string {
	if info, ok := packageInfo(); ok && info.Identifier != "" {
		return info.Identifier + "\n" + name
	}
	return name
}

// singleInstanceSocket returns a short per-user socket path; Unix socket
// paths are limited to about 100 bytes.
func singleInstanceSocket(name string) string {
	sum := sha256.Sum256([]byte(name))
	file := "mygo-" + strconv.Itoa(os.Getuid()) + "-" + hex.EncodeToString(sum[:6]) + ".sock"
	for _, dir := range []string{os.Getenv("XDG_RUNTIME_DIR"), os.TempDir(), "/tmp"} {
		if dir != "" && len(filepath.Join(dir, file)) < 100 {
			return filepath.Join(dir, file)
		}
	}
	return filepath.Join("/tmp", file)
}

// singleInstanceLockPath returns the path of the lock file next to the
// socket at socket.
func singleInstanceLockPath(socket string) string {
	return strings.TrimSuffix(socket, ".sock") + ".lock"
}
