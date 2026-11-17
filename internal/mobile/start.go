package mobile

import (
	"bufio"
	"bytes"
	"os"
	"runtime"
	_ "unsafe" // go:linkname
)

// mainMain is the app's main function, which the build modes of phones
// (c-archive, c-shared) compile but don't run: start runs it.
//
//go:linkname mainMain main.main
func mainMain()

// start runs the app's main function on a thread of its own, Go's main
// thread, once the host is there.
func start() {
	redirectOutput()
	if h, ok := theHost.(interface{ callHere(msg []byte) []byte }); ok {
		askTimeZone(h)
	}
	go func() {
		runtime.LockOSThread()
		mainMain()
	}()
}

// drain runs the calls Go made of the host, on the UI thread.
func drain() {
	if b := current.Load(); b != nil && b.t != nil {
		b.t.drain()
	}
}

// redirectOutput sends what the app writes to its standard output and
// error to the system's log, line by line, where phones show nothing of
// them.
func redirectOutput() {
	for _, f := range []struct {
		file **os.File
		err  bool
	}{{&os.Stdout, false}, {&os.Stderr, true}} {
		r, w, err := os.Pipe()
		if err != nil {
			continue
		}
		if dupOutput(w, *f.file) != nil {
			r.Close()
			w.Close()
			continue
		}
		w.Close() // the standard descriptor writes to the pipe now
		isErr := f.err
		go func() {
			s := bufio.NewScanner(r)
			s.Buffer(make([]byte, 4096), 1<<20)
			for s.Scan() {
				if !chromiumLine(s.Bytes()) {
					hostLog(isErr, s.Bytes())
				}
			}
		}()
	}
}

// chromiumLine reports whether line is a log line of Android's web view,
// which writes to the process's standard error too, as
// "[1031/124014.395252:INFO:file.cc:67] ...": it logs to the system's log
// itself.
func chromiumLine(line []byte) bool {
	if len(line) < 2 || line[0] != '[' {
		return false
	}
	end := bytes.IndexByte(line, ']')
	if end < 0 {
		return false
	}
	head := line[1:end]
	for _, level := range []string{":INFO:", ":WARNING:", ":ERROR:", ":VERBOSE"} {
		if bytes.Contains(head, []byte(level)) || bytes.HasPrefix(head, []byte(level[1:])) {
			return true
		}
	}
	return false
}
