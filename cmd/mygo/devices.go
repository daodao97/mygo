package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// What -device takes: iOS's simulators and devices, with simctl and devicectl
// of Xcode, and Android's devices, emulators and the AVDs of emulators that
// are not running, with adb and emulator of the SDK. The parsing functions
// take the tools' output, for tests without them.

// Kinds of a device.
const (
	kindSimulator = "simulator"
	kindDevice    = "device"
	kindEmulator  = "emulator"
)

// States of a device.
const (
	stateBooted      = "booted"      // a simulator or an emulator that runs
	stateShutdown    = "shutdown"    // one that does not
	stateConnected   = "connected"   // a device to run on
	stateUnavailable = "unavailable" // an iOS device out of reach
)

// deviceEntry is a line of mygo devices. ID is what -device takes.
type deviceEntry struct {
	Platform string `json:"platform"` // ios or android
	Kind     string `json:"kind"`     // simulator, device or emulator
	Name     string `json:"name"`
	ID       string `json:"id,omitempty"` // an emulator that is not running has only its name
	Runtime  string `json:"runtime,omitempty"`
	State    string `json:"state"` // as above, or what adb says of a device
}

// toolTimeout bounds what asks the tools for their devices, which hang when
// their daemons do.
const toolTimeout = 15 * time.Second

// toolOutput runs a tool and returns its standard output.
func toolOutput(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), toolTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	// What it started may hold its output open past the timeout.
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

func runDevices(args []string) error {
	flags := newFlags("devices", "[flags]", `Lists what mygo dev -device takes: iOS simulators and devices, Android
devices and emulators, with those of emulators that are not running, which
mygo dev starts.`)
	asJSON := flags.Bool("json", false, "print the list as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	list := listDevices()
	if *asJSON {
		if list == nil {
			list = []deviceEntry{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	}
	if len(list) == 0 {
		fmt.Println("No devices: mygo doctor tells what is missing.")
		return nil
	}
	fmt.Print(formatDevices(list))
	return nil
}

// listDevices lists the devices of the tools there are.
func listDevices() []deviceEntry {
	var list []deviceEntry
	if runtime.GOOS == "darwin" {
		sims, _ := iosSimulators()
		phones, _ := iosPhysicalDevices()
		list = append(append(list, sims...), phones...)
	}
	return append(list, androidEntries()...)
}

// formatDevices lays the list out in columns.
func formatDevices(list []deviceEntry) string {
	var b bytes.Buffer
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tPLATFORM\tKIND\tSTATE")
	for _, d := range list {
		id := d.ID
		if id == "" {
			id = d.Name
		}
		platform := d.Runtime
		if platform == "" {
			platform = map[string]string{"ios": "iOS", "android": "Android"}[d.Platform]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", d.Name, id, platform, d.Kind, d.State)
	}
	w.Flush()
	return b.String()
}

// iOS simulators.

// iosSimulators lists the simulators of Xcode's iOS runtimes.
func iosSimulators() ([]deviceEntry, error) {
	out, err := toolOutput("xcrun", "simctl", "list", "-j", "devices", "available")
	if err != nil {
		return nil, err
	}
	return parseSimctlDevices(out)
}

// parseSimctlDevices reads simctl list -j devices: the newest runtime first,
// then by name.
func parseSimctlDevices(out []byte) ([]deviceEntry, error) {
	var list struct {
		Devices map[string][]struct {
			UDID, Name, State string
		}
	}
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, fmt.Errorf("simctl: %w", err)
	}
	versions := map[string]string{}
	var entries []deviceEntry
	for key, devs := range list.Devices {
		name, version := simulatorRuntime(key)
		if !strings.HasPrefix(name, "iOS") {
			continue
		}
		versions[name] = version
		for _, d := range devs {
			state := stateShutdown
			if d.State == "Booted" {
				state = stateBooted
			}
			entries = append(entries, deviceEntry{Platform: "ios", Kind: kindSimulator, Name: d.Name, ID: d.UDID, Runtime: name, State: state})
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Runtime != b.Runtime {
			return versionLess(versions[b.Runtime], versions[a.Runtime])
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	return entries, nil
}

// simulatorRuntime reads the key of a runtime of simctl,
// com.apple.CoreSimulator.SimRuntime.iOS-27-0, as "iOS 27.0" and "27.0".
func simulatorRuntime(key string) (name, version string) {
	s := key[strings.LastIndexByte(key, '.')+1:]
	if strings.Contains(key, " ") { // Xcodes long gone said "iOS 13.0"
		s = key
	}
	system, v, ok := strings.Cut(strings.ReplaceAll(s, " ", "-"), "-")
	if !ok {
		return s, ""
	}
	v = strings.ReplaceAll(v, "-", ".")
	return system + " " + v, v
}

// iOS devices.

// iosPhysicalDevices lists the devices Xcode knows, through devicectl of
// Xcode 15 and later. Without it there are none.
func iosPhysicalDevices() ([]deviceEntry, error) {
	f, err := os.CreateTemp("", "mygo-devices-*.json")
	if err != nil {
		return nil, err
	}
	f.Close()
	defer os.Remove(f.Name())
	if _, err := toolOutput("xcrun", "devicectl", "list", "devices", "--quiet", "--timeout", "10", "--json-output", f.Name()); err != nil {
		return nil, err
	}
	out, err := os.ReadFile(f.Name())
	if err != nil {
		return nil, err
	}
	return parseDevicectlDevices(out)
}

// parseDevicectlDevices reads devicectl list devices --json-output, without
// the simulators it may list too and Apple's other systems. Its keys moved
// from hardwareProperties and the like, which Xcode 15 has, to properties.
func parseDevicectlDevices(out []byte) ([]deviceEntry, error) {
	type hardware struct{ UDID, Platform, Reality string }
	type connection struct {
		TunnelState, TransportType string
		State                      string
	}
	var doc struct {
		Result struct {
			Devices []struct {
				Identifier           string
				VisibilityClass      string
				HardwareProperties   hardware
				DeviceProperties     struct{ Name, OSVersionNumber string }
				ConnectionProperties connection
				Properties           struct {
					Hardware   hardware
					Connection connection
					State      struct{ Name string }
					Software   struct {
						OSVersionNumber struct{ StringValue string }
					}
				}
			}
		}
	}
	if err := json.Unmarshal(out, &doc); err != nil {
		return nil, fmt.Errorf("devicectl: %w", err)
	}
	var entries []deviceEntry
	for _, d := range doc.Result.Devices {
		p := d.Properties
		hw, conn := p.Hardware, p.Connection
		if hw.Platform == "" && hw.UDID == "" {
			hw = d.HardwareProperties
		}
		if conn.TransportType == "" && conn.TunnelState == "" {
			conn = d.ConnectionProperties
		}
		if hw.Reality == "simulated" || d.VisibilityClass == "simulators" || hw.Platform != "iOS" {
			continue
		}
		name := or(p.State.Name, d.DeviceProperties.Name)
		version := or(p.Software.OSVersionNumber.StringValue, d.DeviceProperties.OSVersionNumber)
		e := deviceEntry{Platform: "ios", Kind: kindDevice, Name: name, ID: or(hw.UDID, d.Identifier), State: stateConnected}
		if version != "" {
			e.Runtime = "iOS " + version
		}
		// A device out of reach has no way to reach it.
		if conn.TransportType == "" || conn.TunnelState == "unavailable" {
			e.State = stateUnavailable
		}
		entries = append(entries, e)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

// chooseIOSDevice finds what -device names among simulators and devices: a
// UDID, or a name, which among simulators may be that of several runtimes'.
// The booted one wins, then the newest runtime's, which comes first.
func chooseIOSDevice(id string, sims, phones []deviceEntry) (*deviceEntry, error) {
	var named []deviceEntry
	for _, d := range slices.Concat(sims, phones) {
		if d.ID == id {
			return &d, nil
		}
		if d.Name == id {
			named = append(named, d)
		}
	}
	var sim, phone *deviceEntry
	for _, d := range named {
		switch {
		case d.Kind == kindDevice && phone == nil:
			phone = &d
		case d.Kind == kindSimulator && (sim == nil || d.State == stateBooted && sim.State != stateBooted):
			sim = &d
		}
	}
	switch {
	case sim != nil && phone != nil:
		return nil, fmt.Errorf("%q names a simulator (%s) and a device (%s): pass its UDID", id, sim.ID, phone.ID)
	case phone != nil:
		return phone, nil
	case sim != nil:
		return sim, nil
	}
	return nil, fmt.Errorf("no simulator or device %q (mygo devices lists them)", id)
}

// Android.

// adbDevice is a line of adb devices -l.
type adbDevice struct {
	serial, state, model string
	avd                  string // an emulator's AVD, once asked
}

func (d adbDevice) displayName() string {
	if d.avd != "" {
		return d.avd
	}
	return or(d.model, d.serial)
}

func (d adbDevice) isEmulator() bool { return strings.HasPrefix(d.serial, "emulator-") }

// adbLine reads a line of adb devices -l, whose serials may hold spaces, as
// those of wireless debugging, and its state on Linux without udev's rules
// too: "no permissions (…)".
var adbLine = regexp.MustCompile(`^(\S.*?)\s+(device|offline|unauthorized|authorizing|connecting|bootloader|recovery|sideload|rescue|host|no permissions)(\s.*)?$`)

// parseADBDevices reads adb devices -l. The lines of its daemon starting,
// which come before the list, are not devices.
func parseADBDevices(out string) []adbDevice {
	_, list, ok := strings.Cut(out, "List of devices attached")
	if !ok {
		return nil
	}
	var devs []adbDevice
	for _, l := range strings.Split(list, "\n") {
		m := adbLine.FindStringSubmatch(strings.TrimRight(l, "\r "))
		if m == nil {
			continue
		}
		d := adbDevice{serial: m[1], state: m[2]}
		for _, x := range strings.Fields(m[3]) {
			if v, ok := strings.CutPrefix(x, "model:"); ok {
				d.model = strings.ReplaceAll(v, "_", " ")
			}
		}
		devs = append(devs, d)
	}
	return devs
}

// parseAVDName reads adb emu avd name: the name, then the console's OK.
func parseAVDName(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" && l != "OK" && !strings.HasPrefix(l, "KO") {
			return l
		}
	}
	return ""
}

// parseAVDs reads emulator -list-avds, which may say what it thinks of the
// system on lines that name no AVD, as an AVD's name cannot hold what they do.
func parseAVDs(out string) []string {
	var names []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.ContainsAny(l, " :|") {
			continue
		}
		names = append(names, l)
	}
	return names
}

// adbDevices lists what adb reaches, with the AVDs of its emulators.
func adbDevices() ([]adbDevice, error) {
	out, err := toolOutput(sdkTool("platform-tools", "adb"), "devices", "-l")
	if err != nil {
		return nil, err
	}
	devs := parseADBDevices(string(out))
	for i, d := range devs {
		if d.isEmulator() {
			devs[i].avd = runningAVD(d.serial)
		}
	}
	return devs, nil
}

// runningAVD asks an emulator its AVD's name, "" while it cannot tell.
func runningAVD(serial string) string {
	out, err := toolOutput(sdkTool("platform-tools", "adb"), "-s", serial, "emu", "avd", "name")
	if err != nil {
		return ""
	}
	return parseAVDName(string(out))
}

// listAVDs lists the emulators' virtual devices.
func listAVDs() []string {
	out, err := toolOutput(sdkTool("emulator", "emulator"), "-list-avds")
	if err != nil {
		return nil
	}
	return parseAVDs(string(out))
}

// androidEntries lists what adb reaches and the AVDs that are not running.
func androidEntries() []deviceEntry {
	devs, err := adbDevices()
	if err != nil {
		devs = nil
	}
	versions := map[string]string{}
	for _, d := range devs {
		if d.state == "device" {
			out, err := toolOutput(sdkTool("platform-tools", "adb"), "-s", d.serial, "shell", "getprop", "ro.build.version.release")
			if err == nil {
				versions[d.serial] = strings.TrimSpace(string(out))
			}
		}
	}
	return androidDeviceEntries(devs, listAVDs(), versions)
}

// androidDeviceEntries makes the lines of Android: what adb reaches, then
// the AVDs that are not running. versions are the systems of the devices,
// by serial.
func androidDeviceEntries(devs []adbDevice, avds []string, versions map[string]string) []deviceEntry {
	var entries []deviceEntry
	running := map[string]bool{}
	for _, d := range devs {
		e := deviceEntry{Platform: "android", Kind: kindDevice, Name: d.displayName(), ID: d.serial, State: d.state}
		if d.isEmulator() {
			e.Kind = kindEmulator
			running[d.avd] = true
			if d.state == "device" {
				e.State = stateBooted
			}
		} else if d.state == "device" {
			e.State = stateConnected
		}
		if v := versions[d.serial]; v != "" {
			e.Runtime = "Android " + v
		}
		entries = append(entries, e)
	}
	for _, a := range avds {
		if !running[a] {
			entries = append(entries, deviceEntry{Platform: "android", Kind: kindEmulator, Name: a, State: stateShutdown})
		}
	}
	return entries
}

// pickAndroidDevice finds the device to run on among what adb reaches: id if
// set, else the first ready. It says why a device it names cannot run.
func pickAndroidDevice(devs []adbDevice, id string) (*adbDevice, error) {
	var refusing *adbDevice
	for _, d := range devs {
		if id != "" && d.serial != id && d.avd != id && d.model != id {
			continue
		}
		switch d.state {
		case "device":
			return &d, nil
		case "unauthorized":
			refusing = &d
		default:
			if id != "" {
				return nil, fmt.Errorf("%s is %s", d.displayName(), d.state)
			}
		}
	}
	switch {
	case refusing != nil:
		return nil, fmt.Errorf("%s has not allowed this computer: accept USB debugging on it", refusing.displayName())
	case id != "":
		return nil, fmt.Errorf("adb reaches no device %q, and no emulator is named so (mygo devices lists them)", id)
	}
	return nil, errors.New("adb reaches no device: start an emulator, or connect a device with USB debugging on")
}

// Starting emulators.

// avdBoot is what startAVD does through the tools, for tests without them.
type avdBoot struct {
	// start starts the emulator, whose end the channel tells.
	start func(name string) (<-chan error, error)
	// devices lists what adb reaches, with the AVDs of its emulators.
	devices func() ([]adbDevice, error)
	// waitForDevice waits until the device is online.
	waitForDevice func(ctx context.Context, serial string) error
	// prop reads a system property, "" while it cannot.
	prop func(serial, name string) string
	// pause waits before asking again.
	pause func(ctx context.Context) error
}

// run starts the emulator and waits until adb reaches it and its system
// has finished booting.
func (b avdBoot) run(ctx context.Context, name string) (*adbDevice, error) {
	exited, err := b.start(name)
	if err != nil {
		return nil, fmt.Errorf("could not start the emulator %s: %v", name, err)
	}
	// Before it serves: adb lists it for a while before it knows its name.
	var dev *adbDevice
	for dev == nil {
		select {
		case err := <-exited:
			return nil, emulatorExited(name, err)
		default:
		}
		devs, err := b.devices()
		if err == nil {
			for _, d := range devs {
				if d.avd == name {
					dev = &d
				}
			}
		}
		if dev == nil {
			if err := b.pause(ctx); err != nil {
				return nil, err
			}
		}
	}
	if err := b.waitForDevice(ctx, dev.serial); err != nil {
		return nil, err
	}
	for b.prop(dev.serial, "sys.boot_completed") != "1" {
		select {
		case err := <-exited:
			return nil, emulatorExited(name, err)
		default:
		}
		if err := b.pause(ctx); err != nil {
			return nil, err
		}
	}
	return dev, nil
}

func emulatorExited(name string, err error) error {
	if err == nil {
		return fmt.Errorf("the emulator %s exited as it started", name)
	}
	return fmt.Errorf("the emulator %s exited as it started: %v", name, err)
}

// emulatorBootTimeout bounds a cold start: slow machines take minutes.
const emulatorBootTimeout = 5 * time.Minute

// startAVD starts an emulator apart from mygo, which it outlives as it is
// slow to start again, and waits for it to boot.
func startAVD(ctx context.Context, name string) (*adbDevice, error) {
	t := con.start("Starting the emulator " + name)
	ctx, cancel := context.WithTimeout(ctx, emulatorBootTimeout)
	defer cancel()
	var logPath string
	boot := avdBoot{
		start: func(name string) (<-chan error, error) {
			log, err := os.CreateTemp("", "mygo-emulator-*.log")
			if err != nil {
				return nil, err
			}
			logPath = log.Name()
			cmd := exec.Command(sdkTool("emulator", "emulator"), "-avd", name)
			cmd.Stdout, cmd.Stderr = log, log
			detachProcess(cmd)
			if err := cmd.Start(); err != nil {
				log.Close()
				return nil, err
			}
			log.Close()
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			return exited, nil
		},
		devices: adbDevices,
		waitForDevice: func(ctx context.Context, serial string) error {
			t.set("waiting for its system")
			cmd := adb(serial, "wait-for-device")
			return runWithContext(ctx, cmd)
		},
		prop: func(serial, name string) string {
			out, err := toolOutput(sdkTool("platform-tools", "adb"), "-s", serial, "shell", "getprop", name)
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(out))
		},
		pause: func(ctx context.Context) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
				return nil
			}
		},
	}
	d, err := boot.run(ctx, name)
	if errors.Is(err, context.DeadlineExceeded) {
		err = fmt.Errorf("the emulator %s did not boot in %s", name, emulatorBootTimeout)
	}
	if err != nil && logPath != "" {
		err = fmt.Errorf("%w (its output is in %s)", err, logPath)
	} else if logPath != "" {
		os.Remove(logPath)
	}
	if err := t.end(err, "Started the emulator "+name); err != nil {
		return nil, err
	}
	return d, nil
}

// runWithContext runs a command until it ends or ctx is done, which kills it.
func runWithContext(ctx context.Context, cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		cmd.Process.Kill()
		<-done
		return ctx.Err()
	}
}

// Doctor's facts, from the same tools.

// parseNDKVersion reads Pkg.Revision in an NDK's source.properties.
func parseNDKVersion(properties string) string {
	for _, l := range strings.Split(properties, "\n") {
		if k, v, ok := strings.Cut(l, "="); ok && strings.TrimSpace(k) == "Pkg.Revision" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// parseJavaVersion reads java -version, whose first line is as
// openjdk version "21.0.8" 2025-07-15, and returns the version and its major
// number: 8 of the older "1.8.0_392".
func parseJavaVersion(out string) (string, int) {
	m := regexp.MustCompile(`version "([^"]+)"`).FindStringSubmatch(out)
	if m == nil {
		return "", 0
	}
	parts := strings.Split(m[1], ".")
	major, _ := strconv.Atoi(parts[0])
	if major == 1 && len(parts) > 1 {
		major, _ = strconv.Atoi(parts[1])
	}
	return m[1], major
}

// androidPlatforms lists the SDK's platforms, newest last.
func androidPlatforms(sdk string) []string {
	dirs, _ := filepath.Glob(filepath.Join(sdk, "platforms", "android-*"))
	names := make([]string, 0, len(dirs))
	for _, d := range dirs {
		names = append(names, strings.TrimPrefix(filepath.Base(d), "android-"))
	}
	sort.Slice(names, func(i, j int) bool { return versionLess(names[i], names[j]) })
	return names
}
