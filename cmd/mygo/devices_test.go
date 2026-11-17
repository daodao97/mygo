package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

const simctlJSON = `{"devices": {
  "com.apple.CoreSimulator.SimRuntime.iOS-26-5": [
    {"udid": "SIM-OLD", "isAvailable": true, "state": "Shutdown", "name": "iPhone 17"}
  ],
  "com.apple.CoreSimulator.SimRuntime.iOS-27-0": [
    {"udid": "SIM-B", "isAvailable": true, "state": "Booted", "name": "iPhone 18"},
    {"udid": "SIM-A", "isAvailable": true, "state": "Shutdown", "name": "iPhone 17"}
  ],
  "com.apple.CoreSimulator.SimRuntime.watchOS-11-0": [
    {"udid": "WATCH", "isAvailable": true, "state": "Booted", "name": "Apple Watch"}
  ],
  "com.apple.CoreSimulator.SimRuntime.tvOS-18-0": [
    {"udid": "TV", "isAvailable": true, "state": "Shutdown", "name": "Apple TV"}
  ]
}}`

// The newest runtime comes first, and only iOS's.
func TestParseSimctlDevices(t *testing.T) {
	got, err := parseSimctlDevices([]byte(simctlJSON))
	if err != nil {
		t.Fatal(err)
	}
	want := []deviceEntry{
		{"ios", kindSimulator, "iPhone 17", "SIM-A", "iOS 27.0", stateShutdown},
		{"ios", kindSimulator, "iPhone 18", "SIM-B", "iOS 27.0", stateBooted},
		{"ios", kindSimulator, "iPhone 17", "SIM-OLD", "iOS 26.5", stateShutdown},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	if _, err := parseSimctlDevices([]byte("simctl fell over")); err == nil {
		t.Error("output that is not JSON was read")
	}
}

func TestSimulatorRuntime(t *testing.T) {
	for key, want := range map[string]string{
		"com.apple.CoreSimulator.SimRuntime.iOS-27-0":   "iOS 27.0",
		"com.apple.CoreSimulator.SimRuntime.iOS-26-0-1": "iOS 26.0.1",
		"iOS 13.0": "iOS 13.0",
	} {
		if got, _ := simulatorRuntime(key); got != want {
			t.Errorf("%s: %q, want %q", key, got, want)
		}
	}
}

// Both generations of devicectl's output: Xcode 15's keys, and Xcode 26's
// properties, with the simulators it lists, Apple's other systems and a
// device out of reach.
func TestParseDevicectlDevices(t *testing.T) {
	out := `{"info": {"outcome": "success"}, "result": {"devices": [
	  {"identifier": "CORE-1",
	   "connectionProperties": {"transportType": "wired", "tunnelState": "disconnected"},
	   "deviceProperties": {"name": "Jane’s iPhone", "osVersionNumber": "17.5"},
	   "hardwareProperties": {"platform": "iOS", "udid": "UDID-1", "reality": "physical"}},
	  {"identifier": "CORE-2",
	   "connectionProperties": {"tunnelState": "unavailable"},
	   "deviceProperties": {"name": "Old iPad", "osVersionNumber": "16.0"},
	   "hardwareProperties": {"platform": "iOS", "udid": "UDID-2"}},
	  {"identifier": "CORE-3", "visibilityClass": "default",
	   "properties": {
	     "connection": {"state": "disconnected", "transportType": "localNetwork"},
	     "hardware": {"platform": "iOS", "udid": "UDID-3"},
	     "software": {"osVersionNumber": {"stringValue": "27.0.1"}},
	     "state": {"name": "Sam’s iPhone"}}},
	  {"identifier": "CORE-4", "visibilityClass": "simulators",
	   "properties": {
	     "connection": {"transportType": "sameMachine"},
	     "hardware": {"platform": "iOS", "udid": "SIM", "reality": "simulated"},
	     "state": {"name": "iPhone 18 Pro"}}},
	  {"identifier": "CORE-5",
	   "connectionProperties": {"transportType": "localNetwork"},
	   "deviceProperties": {"name": "Watch"},
	   "hardwareProperties": {"platform": "watchOS", "udid": "WATCH"}}
	]}}`
	got, err := parseDevicectlDevices([]byte(out))
	if err != nil {
		t.Fatal(err)
	}
	want := []deviceEntry{
		{"ios", kindDevice, "Jane’s iPhone", "UDID-1", "iOS 17.5", stateConnected},
		{"ios", kindDevice, "Old iPad", "UDID-2", "iOS 16.0", stateUnavailable},
		{"ios", kindDevice, "Sam’s iPhone", "UDID-3", "iOS 27.0.1", stateConnected},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
}

func TestChooseIOSDevice(t *testing.T) {
	sims := []deviceEntry{
		{Kind: kindSimulator, Name: "iPhone 17", ID: "SIM-NEW", State: stateShutdown},
		{Kind: kindSimulator, Name: "iPhone 17", ID: "SIM-OLD", State: stateBooted},
		{Kind: kindSimulator, Name: "iPad", ID: "SIM-IPAD", State: stateShutdown},
		{Kind: kindSimulator, Name: "Twin", ID: "SIM-TWIN", State: stateShutdown},
	}
	phones := []deviceEntry{
		{Kind: kindDevice, Name: "Jane’s iPhone", ID: "UDID-1", State: stateConnected},
		{Kind: kindDevice, Name: "Twin", ID: "UDID-2", State: stateConnected},
	}
	for id, want := range map[string]string{
		"SIM-NEW":       "SIM-NEW",
		"iPhone 17":     "SIM-OLD", // the booted one of two runtimes'
		"iPad":          "SIM-IPAD",
		"Jane’s iPhone": "UDID-1",
		"UDID-1":        "UDID-1",
		"UDID-2":        "UDID-2", // a UDID is never ambiguous
	} {
		d, err := chooseIOSDevice(id, sims, phones)
		if err != nil {
			t.Errorf("%s: %v", id, err)
		} else if d.ID != want {
			t.Errorf("%s: chose %s, want %s", id, d.ID, want)
		}
	}
	if _, err := chooseIOSDevice("Twin", sims, phones); err == nil || !strings.Contains(err.Error(), "UDID") {
		t.Errorf("a name of both: %v", err)
	}
	if _, err := chooseIOSDevice("Nothing", sims, phones); err == nil {
		t.Error("a name of neither was found")
	}
}

const adbOutput = `* daemon not running; starting now at tcp:5037
* daemon started successfully
List of devices attached
emulator-5554          device product:sdk_gphone64_arm64 model:sdk_gphone64_arm64 device:emu64a transport_id:1
R58M12ABC              device usb:1-1 product:o1s model:SM_G991B device:o1s transport_id:2
HT7A1B                 unauthorized usb:1-2 transport_id:3
0123                   no permissions (user in plugdev group; are your udev rules wrong?); see [http://developer.android.com/tools/device.html] usb:1-3 transport_id:4
emulator-5556          offline transport_id:5
adb-R3CN (2)._adb-tls-connect._tcp device product:p model:Pixel_9 device:d transport_id:6

`

func TestParseADBDevices(t *testing.T) {
	got := parseADBDevices(adbOutput)
	want := []adbDevice{
		{serial: "emulator-5554", state: "device", model: "sdk gphone64 arm64"},
		{serial: "R58M12ABC", state: "device", model: "SM G991B"},
		{serial: "HT7A1B", state: "unauthorized"},
		{serial: "0123", state: "no permissions"},
		{serial: "emulator-5556", state: "offline"},
		{serial: "adb-R3CN (2)._adb-tls-connect._tcp", state: "device", model: "Pixel 9"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	if got := parseADBDevices("adb: cannot connect\n"); got != nil {
		t.Errorf("no list: %+v", got)
	}
}

func TestParseAVDs(t *testing.T) {
	if got, want := parseAVDName("ryui_i18n\nOK\n"), "ryui_i18n"; got != want {
		t.Errorf("name: %q", got)
	}
	if got := parseAVDName("KO: unknown command\n"); got != "" {
		t.Errorf("no name: %q", got)
	}
	got := parseAVDs("INFO    | Storing crashdata in: /tmp/x\nHAX is working and emulator runs\nPixel_9\nMyGo_Pixel\n\n")
	if !reflect.DeepEqual(got, []string{"Pixel_9", "MyGo_Pixel"}) {
		t.Errorf("avds: %v", got)
	}
}

// What runs is listed by its serial, with its AVD's name; AVDs that do not
// run have no serial.
func TestAndroidDeviceEntries(t *testing.T) {
	devs := []adbDevice{
		{serial: "emulator-5554", state: "device", model: "sdk", avd: "Pixel_9"},
		{serial: "R58", state: "device", model: "SM G991B"},
		{serial: "HT7", state: "unauthorized"},
	}
	got := androidDeviceEntries(devs, []string{"Pixel_9", "Tablet"}, map[string]string{"emulator-5554": "17", "R58": "14"})
	want := []deviceEntry{
		{"android", kindEmulator, "Pixel_9", "emulator-5554", "Android 17", stateBooted},
		{"android", kindDevice, "SM G991B", "R58", "Android 14", stateConnected},
		{"android", kindDevice, "HT7", "HT7", "", "unauthorized"},
		{"android", kindEmulator, "Tablet", "", "", stateShutdown},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v", got)
	}
	// As --json prints them: what is not known is left out.
	js, _ := json.Marshal(got[3])
	if string(js) != `{"platform":"android","kind":"emulator","name":"Tablet","state":"shutdown"}` {
		t.Errorf("json: %s", js)
	}
	table := formatDevices(got)
	if !strings.Contains(table, "Tablet") || !strings.Contains(table, "emulator-5554") || !strings.HasPrefix(table, "NAME") {
		t.Errorf("table:\n%s", table)
	}
}

func TestPickAndroidDevice(t *testing.T) {
	devs := []adbDevice{
		{serial: "HT7", state: "unauthorized"},
		{serial: "emulator-5556", state: "offline", avd: "Tablet"},
		{serial: "emulator-5554", state: "device", avd: "Pixel_9"},
		{serial: "R58", state: "device", model: "SM G991B"},
	}
	for id, want := range map[string]string{"": "emulator-5554", "R58": "R58", "Pixel_9": "emulator-5554", "SM G991B": "R58"} {
		d, err := pickAndroidDevice(devs, id)
		if err != nil {
			t.Errorf("%q: %v", id, err)
		} else if d.serial != want {
			t.Errorf("%q: chose %s", id, d.serial)
		}
	}
	for id, want := range map[string]string{"HT7": "USB debugging", "Tablet": "offline", "Nothing": "no device"} {
		if _, err := pickAndroidDevice(devs, id); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v", id, err)
		}
	}
	if _, err := pickAndroidDevice(nil, ""); err == nil {
		t.Error("nothing to run on was found")
	}
	if _, err := pickAndroidDevice(devs[:1], ""); err == nil || !strings.Contains(err.Error(), "USB debugging") {
		t.Errorf("only a device refusing: %v", err)
	}
}

// fakeAVD plays an emulator through its steps, each pause one more.
type fakeAVD struct {
	pauses, waited int
	// Pauses before adb lists it and it boots.
	listedAfter, bootedAfter int
	exited                   chan error
	started                  string
}

func (f *fakeAVD) boot() avdBoot {
	return avdBoot{
		start: func(name string) (<-chan error, error) {
			f.started = name
			f.exited = make(chan error, 1)
			return f.exited, nil
		},
		devices: func() ([]adbDevice, error) {
			devs := []adbDevice{{serial: "emulator-5554", state: "device", avd: "Other"}}
			if f.pauses >= f.listedAfter {
				devs = append(devs, adbDevice{serial: "emulator-5556", state: "offline", avd: "Pixel_9"})
			}
			return devs, nil
		},
		waitForDevice: func(ctx context.Context, serial string) error {
			if serial != "emulator-5556" {
				return errors.New("waited for another device: " + serial)
			}
			f.waited++
			return nil
		},
		prop: func(serial, name string) string {
			if serial != "emulator-5556" || name != "sys.boot_completed" {
				return "wrong"
			}
			if f.pauses >= f.bootedAfter {
				return "1"
			}
			return ""
		},
		pause: func(ctx context.Context) error {
			f.pauses++
			if f.pauses > 100 {
				return errors.New("never ends")
			}
			return ctx.Err()
		},
	}
}

// An emulator starts, adb finds it by its AVD's name, among others, and it
// counts once its system has booted.
func TestAVDBoot(t *testing.T) {
	f := &fakeAVD{listedAfter: 3, bootedAfter: 7}
	d, err := f.boot().run(context.Background(), "Pixel_9")
	if err != nil {
		t.Fatal(err)
	}
	if d.serial != "emulator-5556" || f.started != "Pixel_9" || f.waited != 1 || f.pauses != 7 {
		t.Errorf("device %+v, %+v", d, f)
	}
}

func TestAVDBootEnds(t *testing.T) {
	// The emulator exits before adb sees it.
	f := &fakeAVD{listedAfter: 1000, bootedAfter: 1000}
	b := f.boot()
	start := b.start
	b.start = func(name string) (<-chan error, error) {
		exited, err := start(name)
		f.exited <- errors.New("exit status 1")
		return exited, err
	}
	if _, err := b.run(context.Background(), "Pixel_9"); err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("exited: %v", err)
	}
	// And while it boots.
	f = &fakeAVD{listedAfter: 0, bootedAfter: 1000}
	b = f.boot()
	pause := b.pause
	b.pause = func(ctx context.Context) error {
		f.exited <- nil
		return pause(ctx)
	}
	if _, err := b.run(context.Background(), "Pixel_9"); err == nil || !strings.Contains(err.Error(), "exited") {
		t.Errorf("exited booting: %v", err)
	}
	// Ctrl+C.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f = &fakeAVD{listedAfter: 1000, bootedAfter: 1000}
	if _, err := f.boot().run(ctx, "Pixel_9"); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
	// It does not start.
	f = &fakeAVD{}
	b = f.boot()
	b.start = func(string) (<-chan error, error) { return nil, errors.New("no emulator") }
	if _, err := b.run(context.Background(), "Pixel_9"); err == nil || !strings.Contains(err.Error(), "no emulator") {
		t.Errorf("not started: %v", err)
	}
}

func TestLaunchArgsOfIOSDevice(t *testing.T) {
	if got, want := devicectlInstallArgs("UDID", "/a/App Dev.app"), []string{"devicectl", "device", "install", "app", "--device", "UDID", "/a/App Dev.app"}; !reflect.DeepEqual(got, want) {
		t.Errorf("install: %q", got)
	}
	env := []string{"MYGO_ENV=development", "MYGO_DEV_URL=http://192.168.1.2:5173/?a=b", "MYGO_DEV=1"}
	got, err := devicectlLaunchArgs("UDID", "com.example.app.dev", env, true, "")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"devicectl", "device", "process", "launch", "--device", "UDID", "--terminate-existing",
		"--environment-variables", `{"MYGO_DEV":"1","MYGO_DEV_URL":"http://192.168.1.2:5173/?a=b","MYGO_ENV":"development"}`,
		"--console", "com.example.app.dev"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("console: %q", got)
	}
	// An Xcode without --console writes the process it launched.
	got, err = devicectlLaunchArgs("UDID", "com.example.app.dev", nil, false, "/tmp/r.json")
	if err != nil {
		t.Fatal(err)
	}
	want = []string{"devicectl", "device", "process", "launch", "--device", "UDID", "--terminate-existing",
		"--environment-variables", `{}`, "--json-output", "/tmp/r.json", "com.example.app.dev"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("no console: %q", got)
	}
	if !devicectlHasConsole("  --console   Attaches the application") || devicectlHasConsole("  --start-stopped") {
		t.Error("devicectlHasConsole")
	}
	pid, err := parseDevicectlProcess([]byte(`{"result": {"process": {"processIdentifier": 321, "executable": "file:///a"}}}`))
	if err != nil || pid != 321 {
		t.Errorf("pid %d, %v", pid, err)
	}
	if _, err := parseDevicectlProcess([]byte(`{"result": {}}`)); err == nil {
		t.Error("no process was read")
	}
}

func TestLanDevURL(t *testing.T) {
	for u, want := range map[string]string{
		"http://localhost:5173":         "http://192.168.1.2:5173",
		"http://127.0.0.1:5173/app?x=1": "http://192.168.1.2:5173/app?x=1",
		"http://localhost":              "http://192.168.1.2",
		"https://dev.example.com:8443/": "https://dev.example.com:8443/",
	} {
		got, err := lanDevURL(u, "192.168.1.2")
		if err != nil || got != want {
			t.Errorf("%s: %s, %v", u, got, err)
		}
	}
}

// What devicectl says of itself is not the app's output.
func TestMobileNoiseOfDevicectl(t *testing.T) {
	for _, l := range []string{
		"05:21:30  Acquired tunnel connection to device.",
		"05:21:30  Enabling developer disk image services.",
		"Launched application with com.example.app bundle identifier.",
		"Waiting for the application to terminate…",
	} {
		if !mobileNoise(l, "com.example.app") {
			t.Errorf("%q is the app's", l)
		}
	}
	if mobileNoise("listening on :8080", "com.example.app") {
		t.Error("the app's line is noise")
	}
}
