# Opt-in runtime services

These APIs separate common mechanics from application policy. They do not add
mobile screen components. An application chooses what to fetch, which path to
prefer, what to persist, and how to present it.

## Network paths

On iOS, `Network.PathStatus()` and `Network.OnPathChanged(fn)` expose a lazy
`NWPathMonitor`. Call them after `App.WhenReady`. `Known == false` means the
first OS update has not arrived; it does not mean offline. The interface is
`wifi`, `cellular`, `wired`, `other`, or `none`; `Expensive` and `Constrained`
remain independent. Callbacks run on the UI thread; the returned unsubscribe is
safe from any goroutine. Other platforms return `ErrUnsupported` for these
extended methods; existing `Network.Status/OnChanged` APIs remain available.

A satisfied path is not an Internet reachability test. VPNs can change which
interface appears selected; physical Wi-Fi/cellular policy belongs to the VPN's
own path detector. There is no SSID collection or reachability polling.

```go
off, err := mygo.Network.OnPathChanged(func(p mygo.NetworkPathStatus) {
    if p.Known && p.Online { /* retry eligible work */ }
})
if err == nil { mygo.App.OnQuit(off) }
```

## Async work and polling

`mygo.NewTask[T]` wraps `async.Task[T]` with UI-thread notifications and app
lifecycle cancellation. Use `Start(ctx, work)`, `Cancel`, `Snapshot`, and `Close`.
A replacement cancels the previous generation and suppresses its queued/late
results. Work may publish progress values; published values must be immutable.
`State` reports phase, value availability, error, and generation. Notifications
are ordered within a generation; progress may be coalesced if concurrent
publishers enqueue out of order. Work must honor context cancellation.

```go
task := mygo.NewTask(func(s async.State[Result]) {
    if s.HasValue { /* render s.Value on the UI thread */ }
})
task.Start(context.Background(), func(ctx context.Context, publish func(Result)) error {
    result, err := fetch(ctx)
    if err == nil { publish(result) }
    return err
})
// Leaving the screen:
task.Close()
```

Entering the background cancels tasks; becoming active enables new requests
without rerunning a canceled request. Close detaches observers and suppresses
queued notifications; quit also closes tasks. Construct UI wrappers after app
readiness. For non-UI owners, `async.New` accepts a serialized, asynchronous
queue dispatcher (never an inline callback).

`mygo.NewPoller(interval, work, notify)` also requires `SetVisible(true)`. It
starts an immediate tick, skips overlapping work, and stops its ticker and
cancels work when hidden or not active. Resuming starts a fresh tick. Call
`Close` when its owner is disposed. `async.NewPoller` exposes the same behavior
with an explicit foreground flag for services and tests. These are UI polling
tools; they do not grant iOS background execution time.

## Counter histories and sparklines

`timeseries.NewCounter(options, now, counters...)` converts cumulative numeric
counters to rates with a bounded sample count and time window. `Observe` rejects
too-frequent observations, marks long gaps and counter resets, and resets
ordered history on clock rollback. `Samples` returns a deep copy. It is
single-owner; a shared owner supplies a mutex. Options configure sample count,
window, minimum interval, maximum gap, and tolerated wall/monotonic clock skew.
All counters in a sample share one interval and gap status.

Rendering and persistence are separate. `chart.Sparkline` supplies an axis-free
multi-series plot; `chart.DrawSparkline` draws within a caller-owned plot area.
Supply normalized `X/Y`, colors and one common scale; NaN/Inf breaks paths.
`SparkOptions` controls curve, width, fill opacity and final-point radius.
A gap marker is not a zero-rate sample: convert gaps to NaN before rendering.
Use existing `chart.MinMax` for dense histories. No timer or disk access is
started by either package.

## Multi-file generation cache

`generationcache.Open(directory, options)` provides bounded, immutable file
sets with SHA-256 verification, atomic `current.json` publication, a verified
`previous.json` fallback, and garbage collection that retains two generations.
All readers and writers use the same nonblocking cross-process lock so that
collection cannot delete a reader's files. On contention, `Load` and `Commit`
return `ErrBusy`; retry/backoff belongs to the caller. Close only after all
outstanding operations finish. Calls perform disk I/O: use a worker goroutine.

```go
cache, err := generationcache.Open(directory, generationcache.Options{})
if err != nil { return err }
defer cache.Close()
err = cache.Commit(ctx, revision, time.Now(), files, validate)
meta, completeFiles, err := cache.Load()
```

Commit clones input, checks count/individual/total limits, runs an optional
validator, writes/syncs files, then switches the pointer. Cancellation before
the pointer switch leaves the preceding set available. The validator must not
retain or concurrently mutate its input. Load verifies the entire generation;
an invalid current set falls back to the preceding complete set. An application
must additionally check its required file names, revision and semantic validity.
Only flat file names are accepted and I/O is directory-confined. Defaults are
16 MiB/file, 64 MiB/set, 64 files. Files are 0600, directories 0700, inheriting
the container's iOS protection policy. The pointer JSON retains uppercase
`Directory/UpdatedAt/Revision/Files` fields for early consumer compatibility.

The protocol coordinates cooperating framework users. Older writers using a
different lock need an application-level shared transaction lock during a
rolling migration. Atomic visibility is guaranteed; filesystem/power-loss
recovery guarantees are those of the underlying filesystem. Failed writes and
validation do not publish partially downloaded sets.

## iOS scheduled refresh

Declare background refresh callbacks in `mygo.json`:

```json
{
  "identifier": "org.example.app",
  "ios": {
    "backgroundTasks": [{
      "identifier": "org.example.app.refresh",
      "prepare": "PrepareRefresh",
      "run": "RunRefresh",
      "cancel": "CancelRefresh",
      "nextDelay": "NextRefreshDelay"
    }]
  }
}
```

Callbacks are C-exported Go functions in an iOS cgo file: `void prepare(void)`,
`int run(void)`, `void cancel(void)`, `double nextDelay(void)`. Nonzero run means
success. A negative nextDelay disables scheduling; otherwise it is an earliest
start delay in seconds, not a promised wakeup time. Keep any minimum retry
interval and resource/URL policy in the app.

The CLI adds BackgroundTasks.framework, `fetch`, permitted identifiers, and a
portable native helper; the generated entry point registers tasks before
`UIApplicationMain`. `ios.native.entryPoint` cannot be combined with this
feature. Identifiers must be unique descendants of the application identifier;
callback names must be C identifiers with consistent signatures.

Use `backgroundtask.New(timeout, work)` for independent, cancellable workers:
prepare calls `Worker.Prepare`, run calls `Worker.Run`, and cancel calls
`Worker.Expire`. Expiration before Run is remembered; overlapping runs are
rejected. Work must honor context cancellation. Prepare/cancel run on main and
must be short; run and nextDelay use utility queues and cannot depend on the UI
loop being ready (including a cold background launch).

Call `mygo.ScheduleBackgroundRefresh(identifier)` after updating the schedule
or entering background; it is safe before UI readiness and returns
`ErrUnsupported` when the native helper/platform is unavailable. Unknown task
identifiers are rejected. `mygo.LastBackgroundRefreshError(identifier)` reads
the latest submission/registration error text (submission is asynchronous).
Native callers can also use `MyGoScheduleBackgroundRefresh` from the generated
`MyGoBackgroundRefresh.h`. Scheduling
for each identifier is serialized; nextDelay may disable/cancel a pending
request. The runtime records registration/submission errors through
`MyGoBackgroundRefreshLastError(identifier)` (caller frees its UTF-8 string).
Expiration calls cancel and completion reports failure after the worker exits;
the runtime completes each accepted launch once and schedules the next request.
The OS decides whether/when to launch; force-quitting, Background App Refresh
settings and power policy affect eligibility. The simulator does not submit
system refresh requests. Unit cancellation and SDK/build checks cannot prove
that iOS will grant a future wakeup.
