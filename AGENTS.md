# AGENTS.md

MyGo is a desktop application framework in Go. Windows show web pages in the
system webview (WKWebView on macOS, WebKitGTK on Linux, WebView2 on Windows),
with typed Go ↔ TypeScript IPC, or native UI that MyGo draws itself (package
`ui`).

**Read [docs/architecture.md](docs/architecture.md) before changing code.** It
describes the layers, the threading model, native interop without cgo, the IPC
wire protocol, TypeScript generation, and the checklist for adding features.

## Hard rules

- **Desktop builds have no cgo.** macOS, Linux and Windows must build with
  `CGO_ENABLED=0`, using purego or syscall. Mobile builds may enable cgo;
  keep native bridges in their platform backend and shared UI/logic in Go.
- **Bun is dev tooling only.** The repository is a Bun workspace
  (`packages/bridge`, `packages/runtime`, `packages/cli` with its platform
  packages, the plugins' packages in `plugins/`, and examples with a frontend, which keep it at their root like
  the project template, and `website`); Bun also installs and runs the template's scripts
  (Vite, the mygo-cli package). Nothing in an app may need Bun at run time.
- **Scope:** system webview on macOS, Linux and Windows (WebView2), and
  native UI that MyGo draws itself (`WindowOptions.Content`, package `ui`;
  see "Native UI" in the architecture guide), which also runs in browsers
  (`GOOS=js GOARCH=wasm`, `internal/web`, drawn with WebGL 2 by the GL
  renderer; see "Browsers"). The bundled CEF option is out of scope until
  asked. iOS native UI is also in scope. Other platforms must keep compiling
  through `internal/unsupported`.
- **Great DX over Electron parity.** Keep the familiar feel (app lifecycle,
  windows, menus) but design Go-first APIs: typed IPC via `mygo.Bind` and
  `mygo.NewEvent[T]` with the generated client, not string channels.
- **Threading.** Backend methods and handler callbacks run on the main thread.
  Public methods must be safe from any goroutine (`onMain`/`postMain`/`await`
  in `loop.go`). Never block the main thread waiting on work that needs it.
- **purego callbacks are a scarce, never-freed resource.** Create them once
  per signature at startup and route by user data, never per window or call.
- **Apps pay only for what they use.** A feature must not grow apps that
  don't use it: `examples/counter-native` (native UI) and `examples/hello`
  (webview) are measured on every push. What every app reaches (`App.Run`,
  windows, the default menus, `init` functions, the backends' methods)
  must not call into opt-in features: reach them through functions their
  constructor registers (as `linkedDocuments`), and keep heavy
  dependencies and data out of those paths. Never call
  `reflect.Type.Method`, `Value.Method` or `MethodByName` there: it makes
  the linker keep every exported method of the program (2.5 MB). Measure
  both apps before and after a change to `mygo`, `ui`, a backend or the
  dependencies; explain any growth of an app that doesn't use the feature
  in the PR, and find its cause with `-dumpdep` ("Binary size" in the
  architecture guide).

- **A probe that reads zero is a claim to verify, not a result.** The probes
  (`ui.Probe`, `internal/probe`) name spans on a stack, so a name that never
  appears in a readout means its call site was never reached — not that the
  work is free. Check `calls` before drawing a conclusion from an absent
  name, and when it is 0, find out why: the usual cause is a probe on a
  function nothing calls, because the page goes through a method or a wrapper
  of the same name. Two live examples from one afternoon, both of which cost
  hours because the absence was explained away as "the cache hit":
  `class.ApplyAt` is probed on the package-level function while `u.Row(...)`
  reaches the *method* of the same name on `*Cls`; `leaf.textLayout` runs in
  the `layout` phase while its rasterizer runs in `paint`, so a reading of
  one says nothing about the other.
- **Measure the whole account, not the parts you already suspect.** A phase
  whose parts do not sum to it cannot be diagnosed, and the missing part is
  usually in code outside the package being looked at. `build` read 0.89 ms
  while its probed parts summed to 0.03 ms; the other 97% was class-name
  resolution in `mygo-wind`, reached through a call the probe did not cover.
- **A frame-interval-dependent number is not a floor — it is the pipeline's
  temperature.** Every span that touches the GPU's sync points (`gpu/clear`,
  `gpu/swap`) measures how long the CPU waited for the driver, and that wait
  is a function of how recently the last frame was submitted. It is therefore
  reported differently at 1 Hz and at 85 Hz, and neither reading is "the cost
  of the operation". Measured on Adreno 830, same page, same scene, only the
  driver changed: `total` 5.07 → 1.35 ms, `swap` 1.17 → 0.37 ms, `gpu/clear`
  0.361 → 0.095 ms, between a 1 Hz idle frame and a touched frame at a 23 ms
  interval. The 1 Hz case is the slower one, because a second between swaps
  empties the command queue and the next frame pays a cold pipeline; a steady
  stream of frames keeps it warm. So a still page's "expensive" frame is
  expensive **because** it is still: the HUD panel's own `c.After(1 s)` is
  what creates it, and turning the panel off removes the frames entirely.
  Before calling any GPU-touching span a floor, say at what frame interval
  it was read.
- **A present that waits is a symptom of a late frame, not the cost of
  presenting.** The two are told apart by what the wait does when the frame's
  pacing is removed, and the way to remove it is not to trust the number but
  to change the condition. Measured on Adreno 830: `eglSwapInterval(0)` left
  the wait unchanged — p50 0.38 ms and 9% of frames over 4 ms, against 8%
  with the interval at 1 — so the wait is **not** EGL's vsync wait, whatever
  it looks like. What it tracks is the interval since the previous frame:
  55 ms before a 7.9 ms wait, 19 ms before a 0.4 ms one, and with no input at
  all 23 ms between frames and 0.42 ms in the wait. A frame that arrives late
  leaves buffers queued behind it, and the next submit reads that queue as
  its own waiting. Three conclusions follow, and the third is the one easy to
  get wrong: the 7.9 ms is not work; it is not a floor either; and a spike
  that only appears while a test fixture drives input — `adb shell input
  swipe` costs about 50 ms a round trip — is the fixture's pace until proven
  otherwise, not the app's. Say which of the two a spike belongs to before
  optimizing it, and prefer a fixture that drives input in one call
  (`input motionevent DOWN/MOVE/UP`) over one that shells out per event.
- **A measurement rig is part of the system under test until proven
  otherwise.** Every rig tried on this app was slower than the app, so the
  number read was the rig's: `adb shell input` costs about 50 ms a round trip
  and became the frame interval every time. The way out is not a faster rig
  but no rig — a frame source on the device. `Context.AnimationFrame` asks
  for the next frame as soon as the display can show it, and a page that
  calls it animates with nothing driving it from outside. The lab exposes it
  as `/debug/set?drive=1` (lab.selfDrive).
- **A frame interval that lands only on multiples of the vsync is a pacing
  lock, not a slow frame.** With the device-side source on, the intervals
  came out 144 frames at 1× 8.3 ms and 393 at 2× 16.7 ms, with nothing in
  between — a distribution that says the frame loop is landing on the wrong
  vsync, not that drawing is slow. The same run's `submit` read 13.8 ms
  beside a 48-op scene, which no GPU spends that long on: the wait is the
  loop missing its slot, and `submit` is where it gets charged. Told apart
  from real GPU back-pressure by the shape — back-pressure scales with the
  scene, a pacing lock does not, and its intervals quantize to the vsync.

## Commands

```sh
go build ./... && GOOS=linux go build ./... && GOOS=windows go build ./...  # all must pass
go vet ./... && GOOS=linux go vet ./... && GOOS=windows go vet ./...
go vet -tags mygo_noinspector ./...        # production builds, without the inspector of native UI
GOOS=js GOARCH=wasm go vet $(GOOS=js GOARCH=wasm go list ./... | grep -v /cmd/mygo)  # browsers, all but the CLI
go test ./...                              # core (fake backend), tsgen, CLI, accelerator
go build -trimpath -tags mygo_noinspector -ldflags "-s -w -X github.com/egoist/mygo.production=1" -o /tmp/counter-native ./examples/counter-native  # an app's release size, as the benchmarks build it
MYGO_E2E=1 go test ./internal/e2e          # real GUI tests (macOS desktop session)
bun install && bun run test && bun run typecheck   # the workspace, from the root
bun run build                              # build internal/bridge/bridge.js and the npm packages' dist/
bun run --cwd packages/cli binaries        # cross-compile the CLI into the mygo-cli platform packages
go run ./cmd/mygo generate examples/todo   # regenerate an example's TypeScript client
go run ./cmd/mygo dev examples/todo        # live reload (dev bundle in examples/todo/.mygo)
go run ./cmd/mygo dev -web examples/counter-native   # native UI in the browser, with live reload
go run ./cmd/mygo build examples/todo      # .app + .dmg in examples/todo/build
go run ./cmd/mygo build -platform web examples/counter-native   # a static site in examples/counter-native/dist/web
go run ./examples/gallery                  # the native UI toolkit's tour
go run ./examples/code-editor              # the code editor and the diff viewer (ui/editor)
go run ./examples/markdown-chat            # Markdown replies streaming in (ui/markdown)
go run ./examples/notes                    # the rich text editor (ui/richtext), in a notes app
go run ./examples/terminal                 # a terminal (downloads libghostty-vt once)
go generate ./plugins/terminal             # build libghostty-vt for every platform (with Zig), write Ghostty's themes
go generate ./internal/gpu/d3d11           # recompile the Direct3D shader (on Windows)
go generate ./internal/gpu/metal           # recompile the Metal shader (on macOS, with Xcode)
go generate ./internal/l10n                # MyGo's own strings: their functions, and the packages of translations in locales/
go generate ./plugins/glass                # recompile the glass plugin's shaders (on macOS, and on Windows)
bun run --cwd website dev                  # the website, with docs/ at /docs (see website/README.md)
bun scripts/bench.ts run [--e2e]           # benchmarks and app sizes, medians of 6 runs (CI: bench.yml, /benchmarks)
go run ./internal/idlemem app...           # apps' memory once idle, with their webviews' processes
```

Linux GUI tests cross-compile and run in a container with WebKitGTK and Xvfb;
Windows GUI tests need Windows with the WebView2 Runtime (a GitHub Actions
`windows-latest` runner has it). See "Testing" in the architecture guide.

## Conventions

- Never write `[skip ci]` (or `[ci skip]`, `[no ci]`) in a commit message,
  not even quoted in its body: GitHub skips the push's workflows, CI and
  benchmarks included, and Cloudflare the website's deploy. Only the
  benchmarks branch's commits say it.
- New behavior goes into `package mygo` first; backends only translate
  `internal/platform` calls to native ones. Implement every platform method in
  `darwin`, `linux`, `windows`, `web`, `fake` and `unsupported`.
- Objective-C: `alloc`/`init` objects are owned and must be released; wrap
  temporary objects in `withPool`; copy blocks you call later.
- Official plugins live in `plugins/<name>`: the Go package and its npm
  package (`@mygo-plugins/<name>`) side by side, released with
  mygo-runtime's version. The updater plugin is Go only; its `native`
  package draws the update window in native UI. The terminal plugin is Go
  only too, for native UI, with libghostty-vt loaded through purego: its
  `mygo-plugin.json` pins the builds the CLI puts into apps, published as
  release assets (`libghostty-vt-<commit>`); change it with `go generate`. The glass
  plugin is Go only too, an effect of the renderers (`scene.Effect`): its
  shader in Metal, HLSL and GLSL with a CPU twin that must draw the same
  pixels, compiled ahead of time by `go generate` on macOS and Windows.
  The game plugin is Go only too, what games need beyond what apps do,
  opt-in, kept out of core `ui`: a game loop and a camera for native UI,
  and `plugins/game/gamepad`, which reads controllers.
- The `dist/` of npm packages is not committed: run `bun run build` after
  `bun install` (CI and releases do). Commit `internal/bridge/bridge.js`
  whenever its sources change, and keep the generated clients of examples
  (`examples/*/src/mygo.ts`) up to date. Built frontends are not
  committed: `mygo build` embeds them.
- `mygo-runtime` (`packages/runtime`) and `mygo-cli` (`packages/cli`, with
  its per-platform packages in `packages/cli/npm`) are released to npm with
  the same version as the Go module (`mygo.Version`, also the CLI's
  `version`), which `bun scripts/version.ts <version>` sets everywhere; the
  template depends on both. Pushing a `v<version>` tag releases everything
  (`.github/workflows/release.yml`, see "Releasing" in the architecture
  guide); never commit the CLI's binaries.
- The configuration types of `packages/cli/index.d.ts` (for `mygo.config.ts`)
  mirror `Config` in `cmd/mygo/config.go`; `TestConfigTypes` checks them.
- Tests: unit tests through `internal/fake`; GUI behavior in `internal/e2e`.
- Tests never pass or fail on wall-clock time ("Waiting in tests" in the
  architecture guide): wait for the condition or event with
  `internal/wait`, whose one-minute guard only turns a hang into a
  failure and is never tuned; advance a `ui.Tester`'s clock instead of
  sleeping; measure work by counts, not durations. A test that can time
  out while the code is right has a race to fix, in the test or the
  product, not a timeout to raise or a rerun to hide behind.

## Runbook

Android build/install/EGL checks live in `scripts/android-*.sh` (bash; run them from
anywhere, they find the repo themselves). The scripts are **platform-adaptive**:
`HOST_TAG` and the executable suffix come from `uname`, and the default SDK/JDK paths
differ per platform (Windows `C:/Users/hzy/AppData/Local/Android/Sdk`, macOS
`~/Library/Android/sdk`, Linux `~/Android/Sdk`). Everything is overridable by env
(`ANDROID_HOME`/`ANDROID_SDK_ROOT`, `ANDROID_NDK_VERSION=28.2.13676358`, `JAVA_HOME`,
`MYGO_ANDROID_CC`, `MYGO_ANDROID_PKG`, `MYGO_ANDROID_SO`, `MYGO_PYTHON`):

```sh
bash scripts/android-doctor.sh          # toolchain self-check: SDK≥31 / NDK r28 / JDK 17+ / CC / adb / emulator
bash scripts/android-build.sh           # Go .so (c-shared) → 16KB align check → Gradle if android/ exists
bash scripts/android-build.sh --so-only # the .so and its page-align check only
bash scripts/android-run.sh             # adb install + am start + logcat
bash scripts/android-egl-stress.sh      # 20x foreground/background + 10x rotation, blank-frame report
```

**JDK: AGP 9.0.1 / Gradle 9.1.0 need JDK 17+; JDK 21 is what the Mac mini uses.**
`brew install openjdk@21` is keg-only — export
`JAVA_HOME=/opt/homebrew/opt/openjdk@21` (or the equivalent) before running Gradle.

The `.so` is `GOOS=android GOARCH=arm64 CGO_ENABLED=1 go build -buildmode=c-shared`
with the NDK r28 `aarch64-linux-android29-clang`; it lands in
`android/app/src/main/jniLibs/arm64-v8a/` when that exists, else in `.mygo/android/`
(gitignored). **NDK r28 already links 16KB-aligned (`align 2**14`), so never set
`CGO_LDFLAGS`; the build script only *verifies* alignment with `llvm-objdump -p`.**
`android-run.sh` and `android-egl-stress.sh` read the package/activity from
`android/app/src/main/AndroidManifest.xml` and say exactly what is missing instead of
guessing; the stress test reports "无设备，未跑" when no device is attached.

### HarfBuzz static libraries (Android text shaping)

Android shapes text through C HarfBuzz (`shaping/chb.go` in
`github.com/Heyana/typesetting-go`), which links two static libraries. They are
**not in git** — `third_party/hb-build/.gitignore` keeps `src/` and `lib/` out —
because they are 12 MB of build product. `android-build.sh` looks for them at
`third_party/hb-build/src/lib/android/` and exports the `CGO_CFLAGS`/`CGO_LDFLAGS`
itself; `chb.go`'s own `#cgo` paths point at `${SRCDIR}/../hb-build`, which does not
resolve from the module cache, so those flags are the only thing that makes the link
work. Missing libraries is an error with the download URL, not a link failure later.

```sh
# download (same for local builds and CI)
curl -fsSL https://github.com/Heyana/mygo-upstream/releases/download/hb-android-arm64/hb-android-arm64.tar.gz \
  | tar xz -C third_party/hb-build/src/lib

# or build them yourself — auto-downloads the freetype/harfbuzz sources on first run
bash third_party/hb-build/build.sh android
```

`build.sh` reads the NDK from `$NDK`, then `ANDROID_HOME`/`ANDROID_SDK_ROOT`, and
derives the host tag from `uname`. It fills `android-arm64.ini` from
`android-arm64.ini.in` (that generated file names one machine's toolchain, so it is
ignored rather than tracked).
