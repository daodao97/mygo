//go:build windows && (amd64 || arm64)

package windows

import "testing"

// powerThrottlingIgnoreTimerResolution is PROCESS_POWER_THROTTLING_IGNORE_TIMER_RESOLUTION.
const powerThrottlingIgnoreTimerResolution = 0x4

// withThrottlingPolicy sets the test process's power throttling policy to
// st, and skips the test where Windows can't set it or tell it, and gives
// the policy back to the system when the test ends.
func withThrottlingPolicy(t *testing.T, st powerThrottlingState) {
	t.Helper()
	if !has(procGetProcessInformation) || !has(procSetProcessInformation) {
		t.Skip("no power throttling on this Windows")
	}
	if !setThrottlingPolicy(st) {
		t.Skipf("Windows refuses the power throttling policy %+v here", st)
	}
	t.Cleanup(func() { setThrottlingPolicy(powerThrottlingState{}) })
	if got, ok := throttlingPolicy(); !ok || got.controlMask != st.controlMask || got.stateMask != st.stateMask {
		t.Skipf("the power throttling policy reads back as %+v (%v), not %+v", got, ok, st)
	}
}

func wantPolicy(t *testing.T, when string, control, state uint32) {
	t.Helper()
	got, ok := throttlingPolicy()
	if !ok {
		t.Fatalf("%s: the power throttling policy can't be read", when)
	}
	if got.controlMask != control || got.stateMask != state {
		t.Errorf("%s: policy control %#x state %#x, want control %#x state %#x", when, got.controlMask, got.stateMask, control, state)
	}
}

// A process starts with its throttling the system's to decide: KeepRunning
// opts it out, and the release leaves it to the system again, rather than
// opted out for as long as it runs.
func TestKeepRunningGivesThrottlingBackToTheSystem(t *testing.T) {
	withThrottlingPolicy(t, powerThrottlingState{})
	release := power{&Backend{}}.KeepRunning("Serving the phones")
	wantPolicy(t, "held", powerThrottlingExecutionSpeed, 0)
	release()
	wantPolicy(t, "released", 0, 0)
}

func TestKeepRunningPutsBackThrottlingOptedInto(t *testing.T) {
	withThrottlingPolicy(t, powerThrottlingState{controlMask: powerThrottlingExecutionSpeed, stateMask: powerThrottlingExecutionSpeed})
	release := power{&Backend{}}.KeepRunning("Serving the phones")
	wantPolicy(t, "held", powerThrottlingExecutionSpeed, 0)
	release()
	wantPolicy(t, "released", powerThrottlingExecutionSpeed, powerThrottlingExecutionSpeed)
}

func TestKeepRunningNests(t *testing.T) {
	withThrottlingPolicy(t, powerThrottlingState{})
	p := power{&Backend{}}
	first := p.KeepRunning("Serving the phones")
	second := p.KeepRunning("Exporting the video")
	first()
	wantPolicy(t, "one of two released", powerThrottlingExecutionSpeed, 0)
	second()
	wantPolicy(t, "both released", 0, 0)
}

func TestKeepRunningKeepsTimerResolutionPolicy(t *testing.T) {
	withThrottlingPolicy(t, powerThrottlingState{controlMask: powerThrottlingIgnoreTimerResolution, stateMask: powerThrottlingIgnoreTimerResolution})
	release := power{&Backend{}}.KeepRunning("Serving the phones")
	wantPolicy(t, "held", powerThrottlingIgnoreTimerResolution|powerThrottlingExecutionSpeed, powerThrottlingIgnoreTimerResolution)
	release()
	wantPolicy(t, "released", powerThrottlingIgnoreTimerResolution, powerThrottlingIgnoreTimerResolution)
}
