package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// The development builds of phones: mygo dev -platform ios builds the app
// for the simulator, installs it on the one booted (or -device, which may
// name a device, then signed with a development profile) and launches it,
// its output in the console; -platform android does the same on the device
// or emulator adb reaches. The app loads devUrl, which the simulator reaches
// as the Mac does, an iOS device over the network, and Android through adb
// reverse; without devUrl it embeds frontendDist, as the device cannot read
// the Mac's disk.

// mobileDevice is the device a development build runs on.
type mobileDevice struct {
	platform string // "ios" or "android"
	id       string // the simulator's or device's UDID, or adb's serial
	name     string
	physical bool // an iOS device, not a simulator
}

// iosRunTarget finds what to run on: what id names (a UDID or a name) among
// simulators, which it boots, and devices, else the simulator booted.
func iosRunTarget(id string) (*mobileDevice, error) {
	sims, simErr := iosSimulators()
	if id == "" {
		if simErr != nil {
			return nil, fmt.Errorf("no simulators (install Xcode): %v", simErr)
		}
		for _, d := range sims {
			if d.State == stateBooted {
				return &mobileDevice{platform: "ios", id: d.ID, name: d.Name}, nil
			}
		}
		return nil, errors.New("no simulator is booted: open one (open -a Simulator), or pass -device with its name or UDID (mygo devices lists them)")
	}
	// Devices are for Xcode 15 and later, which has devicectl.
	phones, phoneErr := iosPhysicalDevices()
	d, err := chooseIOSDevice(id, sims, phones)
	if err != nil {
		if simErr != nil {
			return nil, fmt.Errorf("no simulators (install Xcode): %v", simErr)
		}
		if phoneErr != nil {
			return nil, fmt.Errorf("%v; to run on a device, update to Xcode 15 or later, whose devicectl reaches it", err)
		}
		return nil, err
	}
	if d.Kind == kindDevice {
		if d.State == stateUnavailable {
			return nil, fmt.Errorf("%s is out of reach: connect it, unlock it and turn Developer Mode on (Settings › Privacy & Security)", d.Name)
		}
		return &mobileDevice{platform: "ios", id: d.ID, name: d.Name, physical: true}, nil
	}
	if d.State != stateBooted {
		if err := exec.Command("xcrun", "simctl", "boot", d.ID).Run(); err != nil {
			return nil, fmt.Errorf("could not boot %s: %v", d.Name, err)
		}
	}
	return &mobileDevice{platform: "ios", id: d.ID, name: d.Name}, nil
}

// buildAndLaunchMobile is buildAndLaunch for phones.
func (s *devSession) buildAndLaunchMobile(ctx context.Context, t *task, running [32]byte) (p *devProcess, sum [32]byte, err error) {
	c, err := loadConfig(s.root)
	if err != nil {
		return nil, sum, err
	}
	dc := devConfig(c)
	dir := filepath.Join(s.root, ".mygo", "dev", s.device.platform)
	work := filepath.Join(dir, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		return nil, sum, err
	}
	if err := s.updateI18n(c); err != nil {
		return nil, sum, err
	}
	opts := buildOptions{debug: true, work: work}
	if opts.pkg, err = packageDir(c); err != nil {
		return nil, sum, err
	}
	if c.DevURL == "" {
		if opts.overlay, err = frontendFiles(c, opts.pkg, work); err != nil {
			return nil, sum, err
		}
	}
	stage, err := os.MkdirTemp(dir, ".staging-")
	if err != nil {
		return nil, sum, err
	}
	defer os.RemoveAll(stage)

	var app string
	var prof *profile
	switch s.device.platform {
	case "ios":
		tc, err := newIOSToolchain(c, !s.device.physical)
		if err != nil {
			return nil, sum, err
		}
		if s.device.physical {
			// Before building: no point building for a profile that isn't there.
			if prof, err = pickProfile(dc, "development", s.device.id); err != nil {
				return nil, sum, fmt.Errorf("%v (the development app's identifier is %s: a wildcard profile fits it)", err, dc.Identifier)
			}
			if len(dc.IOS.AssociatedDomains) > 0 && !prof.allowsAssociatedDomains() {
				warnf("%q has no Associated Domains, as profiles of a wildcard app ID: this build has no universal links", prof.name)
				d := *dc
				d.IOS.AssociatedDomains = nil
				dc = &d
			}
		}
		if app, err = tc.buildApp(ctx, c, dc, t, opts, stage); err != nil {
			return nil, sum, err
		}
	case "android":
		if app, err = s.buildAndroidDev(ctx, c, dc, t, opts, stage); err != nil {
			return nil, sum, err
		}
	}
	h := sha256.New()
	if err := hashTree(h, app); err != nil {
		return nil, sum, err
	}
	copy(sum[:], h.Sum(nil))
	if sum == running {
		return nil, sum, errUnchanged
	}
	if c.Bindings != "" {
		t.set("generating the TypeScript client")
		if err := writeClient(ctx, c, false); err != nil {
			return nil, sum, err
		}
	}
	final := filepath.Join(dir, filepath.Base(app))
	if s.device.platform == "ios" {
		t.set("signing")
		if prof != nil {
			err = prof.sign(work, app, dc)
		} else {
			err = signForSimulator(app)
		}
		if err != nil {
			return nil, sum, err
		}
	}
	if err := replacePath(app, final); err != nil {
		return nil, sum, err
	}
	t.set("installing on " + s.device.name)
	s.stopLive()
	if p, err = s.launchMobile(dc, final); err != nil {
		return nil, sum, err
	}
	return p, sum, nil
}

// devicectlInstallArgs are the arguments of xcrun that install an app on a
// device.
func devicectlInstallArgs(device, app string) []string {
	return []string{"devicectl", "device", "install", "app", "--device", device, app}
}

// devicectlLaunchArgs are the arguments of xcrun that launch an app on a
// device, with env (as KEY=value), replacing what runs of it. With console,
// xcrun lasts as long as the app and shows its output; without, it returns,
// and writes the process in jsonOut.
func devicectlLaunchArgs(device, bundleID string, env []string, console bool, jsonOut string) ([]string, error) {
	vars := make(map[string]string, len(env))
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		vars[k] = v
	}
	js, err := json.Marshal(vars)
	if err != nil {
		return nil, err
	}
	args := []string{"devicectl", "device", "process", "launch", "--device", device, "--terminate-existing", "--environment-variables", string(js)}
	if console {
		args = append(args, "--console")
	} else {
		args = append(args, "--json-output", jsonOut)
	}
	return append(args, bundleID), nil
}

// devicectlHasConsole reports whether the help of devicectl device process
// launch has --console, which Xcode 15 has not.
func devicectlHasConsole(help string) bool { return strings.Contains(help, "--console") }

// parseDevicectlProcess reads the process of launch --json-output.
func parseDevicectlProcess(out []byte) (int, error) {
	var doc struct {
		Result struct {
			Process struct {
				ProcessIdentifier int
			}
		}
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return 0, fmt.Errorf("devicectl: %w", err)
	}
	if doc.Result.Process.ProcessIdentifier == 0 {
		return 0, errors.New("devicectl did not say what it launched")
	}
	return doc.Result.Process.ProcessIdentifier, nil
}

// launchOnIOSDevice installs the app on a device and starts it. The process
// it returns lasts as long as the app, with its output when Xcode can show
// it, else as long as mygo dev's build: stop then ends the app.
func launchOnIOSDevice(device, bundleID, app string, env []string) (*exec.Cmd, func(), error) {
	help, err := exec.Command("xcrun", "devicectl", "device", "process", "launch", "--help").CombinedOutput()
	if err != nil {
		return nil, nil, errors.New("running on a device needs Xcode 15 or later, whose devicectl reaches it")
	}
	if out, err := exec.Command("xcrun", devicectlInstallArgs(device, app)...).CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("devicectl install: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	if devicectlHasConsole(string(help)) {
		args, err := devicectlLaunchArgs(device, bundleID, env, true, "")
		if err != nil {
			return nil, nil, err
		}
		return exec.Command("xcrun", args...), nil, nil
	}
	warnf("This Xcode's devicectl cannot show the app's output: update Xcode to see it here")
	result, err := os.CreateTemp("", "mygo-launch-*.json")
	if err != nil {
		return nil, nil, err
	}
	result.Close()
	defer os.Remove(result.Name())
	args, err := devicectlLaunchArgs(device, bundleID, env, false, result.Name())
	if err != nil {
		return nil, nil, err
	}
	if out, err := exec.Command("xcrun", args...).CombinedOutput(); err != nil {
		return nil, nil, fmt.Errorf("devicectl launch: %v\n%s", err, strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(result.Name())
	if err != nil {
		return nil, nil, err
	}
	pid, err := parseDevicectlProcess(data)
	if err != nil {
		return nil, nil, err
	}
	stop := func() {
		exec.Command("xcrun", "devicectl", "device", "process", "terminate", "--device", device, "--pid", strconv.Itoa(pid)).Run()
	}
	// Nothing to follow: this stands for the app until it is stopped.
	return exec.Command("tail", "-f", "/dev/null"), stop, nil
}

// hashTree hashes the files under root, by path and content.
func hashTree(h io.Writer, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == filepath.Join("_CodeSignature", "CodeResources") {
			return nil
		}
		io.WriteString(h, rel+"\x00")
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(h, f)
		return err
	})
}

// launchMobile installs the app on the device and starts it, its output
// in the console. The process it returns ends when the app does.
func (s *devSession) launchMobile(dc *Config, app string) (*devProcess, error) {
	env := append(append([]string{}, s.env...), "MYGO_DEV=1")
	var cmd *exec.Cmd
	var stop func()
	switch {
	case s.device.physical:
		var err error
		if cmd, stop, err = launchOnIOSDevice(s.device.id, dc.Identifier, app, env); err != nil {
			return nil, err
		}
	case s.device.platform == "ios":
		if !s.replaceInstalled(dc, app) {
			if out, err := exec.Command("xcrun", "simctl", "install", s.device.id, app).CombinedOutput(); err != nil {
				return nil, fmt.Errorf("simctl install: %v\n%s", err, strings.TrimSpace(string(out)))
			}
		}
		cmd = exec.Command("xcrun", "simctl", "launch", "--console-pty", "--terminate-running-process", s.device.id, dc.Identifier)
		cmd.Env = os.Environ()
		for _, e := range env {
			// simctl passes the variables named so to the app.
			cmd.Env = append(cmd.Env, "SIMCTL_CHILD_"+e)
		}
		id := s.device.id
		stop = func() { exec.Command("xcrun", "simctl", "terminate", id, dc.Identifier).Run() }
	case s.device.platform == "android":
		var err error
		if cmd, stop, err = s.launchAndroid(dc, app, env); err != nil {
			return nil, err
		}
	}
	out, outDone, err := pipe(con.output("app", os.Stdout))
	if err != nil {
		return nil, err
	}
	r, w, err := os.Pipe()
	if err != nil {
		out.Close()
		return nil, err
	}
	go func() {
		// The lines of the app's, without the system's noise about it.
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 4096), 1<<20)
		for sc.Scan() {
			if l := sc.Text(); !mobileNoise(l, dc.Identifier) {
				io.WriteString(out, l+"\n")
			}
		}
		r.Close()
		out.Close()
	}()
	cmd.Stdout, cmd.Stderr = w, w
	setProcessGroup(cmd)
	err = cmd.Start()
	w.Close()
	if err != nil {
		return nil, err
	}
	p := &devProcess{exe: app, cmd: cmd, done: make(chan struct{}), onStop: stop}
	go func() {
		p.err = cmd.Wait()
		<-outDone
		close(p.done)
	}()
	return p, nil
}

// replaceInstalled puts the new executable in the app the simulator has
// installed, and reports whether that is all it takes: nothing else of the
// bundle changed since it was installed. Installing takes the system seconds;
// a change of the app's Go code changes only the executable.
func (s *devSession) replaceInstalled(dc *Config, app string) bool {
	h := sha256.New()
	exe := dc.executableName()
	err := filepath.WalkDir(app, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(app, path)
		if rel == exe || rel == filepath.Join("_CodeSignature", "CodeResources") {
			return nil
		}
		io.WriteString(h, rel+"\x00")
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(h, f)
		return err
	})
	if err != nil {
		return false
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	same := s.installedBundle == sum && s.installedOn == s.device.id
	s.installedBundle, s.installedOn = sum, s.device.id
	if !same {
		return false
	}
	out, err := exec.Command("xcrun", "simctl", "get_app_container", s.device.id, dc.Identifier, "app").Output()
	if err != nil {
		return false
	}
	installed := strings.TrimSpace(string(out))
	data, err := os.ReadFile(filepath.Join(app, exe))
	if err != nil {
		return false
	}
	// A new file: the system maps the old one's while the app runs.
	tmp := filepath.Join(installed, "."+exe+".new")
	if os.WriteFile(tmp, data, 0o755) != nil {
		return false
	}
	if os.Rename(tmp, filepath.Join(installed, exe)) != nil {
		os.Remove(tmp)
		return false
	}
	return true
}

// swiftStamp hashes the sources of the host, for what target it compiled.
func swiftStamp(host, target string) string {
	h := sha256.New()
	io.WriteString(h, target+"\x00")
	if hashTree(h, host) != nil {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// swiftCompiled reports whether object is what the host's sources compile to.
func swiftCompiled(host, target, object string) bool {
	if _, err := os.Stat(object); err != nil {
		return false
	}
	stamp, err := os.ReadFile(object + ".stamp")
	return err == nil && string(stamp) == swiftStamp(host, target)
}

func noteSwiftCompiled(host, target, object string) {
	os.WriteFile(object+".stamp", []byte(swiftStamp(host, target)), 0o644)
}

// mobileNoise reports whether a line of a development build's output is
// the system's rather than the app's: the simulator's warnings about its
// own frameworks, simctl's line of the app's process ID.
func mobileNoise(line, id string) bool {
	if strings.HasPrefix(line, "--------- beginning of ") { // logcat's
		return true
	}
	if strings.Contains(line, "is implemented in both") && strings.Contains(line, ".simruntime") {
		return true
	}
	if rest, ok := strings.CutPrefix(line, id+": "); ok && strings.Trim(rest, "0123456789") == "" {
		return true
	}
	// devicectl's own, on its way to a device.
	for _, s := range []string{"Acquired tunnel connection to device", "Enabling developer disk image services",
		"Acquired usage assertion", "Launched application with " + id, "Waiting for the application to terminate"} {
		if strings.Contains(line, s) {
			return true
		}
	}
	return false
}
