//go:build windows && (amd64 || arm64)

package windows

import (
	"syscall"
	"time"
	"unsafe"

	"github.com/egoist/mygo/internal/platform"
)

var (
	wtsapi32 = systemDLL("wtsapi32.dll")

	procWTSRegisterSessionNotification = wtsapi32.NewProc("WTSRegisterSessionNotification")
	procPowerCreateRequest             = kernel32.NewProc("PowerCreateRequest")
	procPowerSetRequest                = kernel32.NewProc("PowerSetRequest")
	procPowerClearRequest              = kernel32.NewProc("PowerClearRequest")
	procCloseHandle                    = kernel32.NewProc("CloseHandle")
	procGetSystemPowerStatus           = kernel32.NewProc("GetSystemPowerStatus")
	procGetTickCount                   = kernel32.NewProc("GetTickCount")
	procGetLastInputInfo               = user32.NewProc("GetLastInputInfo")
	procGetProcessInformation          = kernel32.NewProc("GetProcessInformation")
	procSetProcessInformation          = kernel32.NewProc("SetProcessInformation")
)

// Messages about power and the session, which the application window gets.
const (
	wmPowerBroadcast      = 0x0218
	wmWTSSessionChange    = 0x02B1
	pbtAPMSuspend         = 0x4
	pbtAPMResumeAutomatic = 0x12
	wtsSessionLock        = 0x7
	wtsSessionUnlock      = 0x8
)

// Power throttling (ProcessPowerThrottling), which Windows applies to the
// processes it doesn't see, EcoQoS: their threads run slower, and on hybrid
// processors on the efficiency cores.
const (
	processPowerThrottling        = 4 // ProcessPowerThrottling
	powerThrottlingVersion        = 1 // PROCESS_POWER_THROTTLING_CURRENT_VERSION
	powerThrottlingExecutionSpeed = 0x1
)

func (b *Backend) Power() platform.Power     { return power{b} }
func (b *Backend) Secrets() platform.Secrets { return secrets{} }

type power struct{ b *Backend }

func (p power) Watch() {
	p.b.watchingPower = true
	const notifyForThisSession = 0
	procWTSRegisterSessionNotification.Call(p.b.appHwnd, notifyForThisSession)
}

// powerMessage reports the power and session messages of the application
// window.
func (b *Backend) powerMessage(m uint32, wp uintptr) {
	if !b.watchingPower {
		return
	}
	switch {
	case m == wmPowerBroadcast && wp == pbtAPMSuspend:
		b.h.PowerEvent("suspend")
	case m == wmPowerBroadcast && wp == pbtAPMResumeAutomatic:
		b.h.PowerEvent("resume")
	case m == wmWTSSessionChange && wp == wtsSessionLock:
		b.h.PowerEvent("lock-screen")
	case m == wmWTSSessionChange && wp == wtsSessionUnlock:
		b.h.PowerEvent("unlock-screen")
	}
}

// reasonContext mirrors REASON_CONTEXT with a simple reason string.
type reasonContext struct {
	version, flags uint32
	reason         *uint16
	_              [2]uintptr // the rest of the union
}

func (p power) KeepAwake(display bool, reason string) func() {
	const (
		contextSimpleString = 0x1
		requestDisplay      = 0 // PowerRequestDisplayRequired
		requestSystem       = 1 // PowerRequestSystemRequired
	)
	ctx := reasonContext{flags: contextSimpleString, reason: u16(reason)}
	h, _, _ := procPowerCreateRequest.Call(uintptr(unsafe.Pointer(&ctx)))
	if h == 0 || h == ^uintptr(0) {
		return nil
	}
	requests := []uintptr{requestSystem}
	if display {
		requests = append(requests, requestDisplay)
	}
	for _, r := range requests {
		procPowerSetRequest.Call(h, r)
	}
	return func() {
		for _, r := range requests {
			procPowerClearRequest.Call(h, r)
		}
		procCloseHandle.Call(h)
	}
}

// powerThrottlingState is PROCESS_POWER_THROTTLING_STATE: the mechanisms
// the process decides itself, in controlMask, and what it decided for them,
// in stateMask. A mechanism outside controlMask is the system's to decide,
// which is how a process starts: GetProcessInformation reports this policy,
// not whether Windows throttles the process at the moment.
type powerThrottlingState struct {
	version, controlMask, stateMask uint32
}

// throttlingPolicy returns the process's power throttling policy, and
// whether Windows told it.
func throttlingPolicy() (powerThrottlingState, bool) {
	st := powerThrottlingState{version: powerThrottlingVersion}
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return st, false
	}
	r, _, _ := procGetProcessInformation.Call(uintptr(process), processPowerThrottling, uintptr(unsafe.Pointer(&st)), unsafe.Sizeof(st))
	return st, r != 0
}

// setThrottlingPolicy sets the process's power throttling policy, and
// reports whether Windows took it.
func setThrottlingPolicy(st powerThrottlingState) bool {
	st.version = powerThrottlingVersion
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return false
	}
	r, _, _ := procSetProcessInformation.Call(uintptr(process), processPowerThrottling, uintptr(unsafe.Pointer(&st)), unsafe.Sizeof(st))
	return r != 0
}

// KeepRunning turns off the power throttling Windows applies to a process
// it doesn't see, as one whose windows are hidden, minimized or behind
// another is, in favor of the ones it shows: EcoQoS slows its threads and
// moves them to the efficiency cores of hybrid processors, which an app
// that serves the network, streams or syncs out of sight pays for. It
// doesn't keep the computer awake, as KeepAwake does.
//
// The first holder opts the process out of execution speed throttling;
// the last release puts back the policy the process had before, which
// usually leaves it to the system again, so an app that shows a window
// again is treated as Windows treats it. The other mechanisms (timer
// resolution) keep their policy throughout.
func (p power) KeepRunning(string) func() {
	b := p.b
	if b.keepRunning == 0 {
		b.throttlingBefore, b.throttlingKnown = throttlingPolicy()
		st := b.throttlingBefore
		if !b.throttlingKnown {
			st = powerThrottlingState{}
		}
		st.controlMask |= powerThrottlingExecutionSpeed
		st.stateMask &^= powerThrottlingExecutionSpeed
		setThrottlingPolicy(st)
	}
	b.keepRunning++
	return func() {
		b.keepRunning--
		if b.keepRunning > 0 {
			return
		}
		if b.throttlingKnown {
			setThrottlingPolicy(b.throttlingBefore)
		} else {
			// Windows didn't tell the policy before: MyGo sets it nowhere
			// else, so it was the system's to decide.
			setThrottlingPolicy(powerThrottlingState{})
		}
	}
}

func (p power) OnBattery() bool {
	var status struct {
		acLineStatus, batteryFlag, batteryLifePercent, systemStatusFlag uint8
		batteryLifeTime, batteryFullLifeTime                            uint32
	}
	if r, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status))); r == 0 {
		return false
	}
	return status.acLineStatus == 0
}

func (p power) IdleTime() time.Duration {
	info := struct{ size, time uint32 }{size: 8}
	if r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0
	}
	now, _, _ := procGetTickCount.Call()
	return time.Duration(uint32(now)-info.time) * time.Millisecond
}
