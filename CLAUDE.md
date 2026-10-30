# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@AGENTS.md

AGENTS.md (imported above) holds the hard rules, the full command list, the
conventions and the Android runbook; it is authoritative. This file adds what
it leaves to `docs/architecture.md` (about 5,000 lines: read the section you
need, by its heading, rather than the whole file).

## Running one test

```sh
go test -run 'TestName' .                  # core, against internal/fake
go test -run 'TestName' ./ui               # native UI, through ui.Tester
go test -short ./cmd/mygo                  # CLI without its builds and tools
go test -short ./plugins/terminal          # without downloading libghostty-vt
MYGO_E2E=1 go test -run 'TestName' ./internal/e2e   # real GUI, macOS desktop session
bun test packages/runtime                  # one workspace package's tests
go test -run '^$' -bench . ./ui            # time a frame (benchmarks only, never tests)
```

The core's unit tests run `App.Run` on the main goroutine from `TestMain`, as
a real program does; `internal/fake` records the scripts it evaluates, so
tests assert on exactly what the page would receive.

## Generated code

Do not edit these by hand; change the source and regenerate.

- `ui/api_gen.go`: the checked public facade of native UI, from
  `internal/uigen` (`go generate ./ui`). The private render tree uses `node`
  and `context`; the facade brackets every constructor with
  `beginBuild`/`endBuild`.
- `internal/bridge/bridge.js`: built from `packages/bridge` by `bun run build`,
  and committed.
- `examples/*/src/mygo.ts`: generated clients (`go run ./cmd/mygo generate
  examples/<name>`); the generator is `internal/tsgen`.
- Shaders, libghostty-vt builds, l10n functions and pinyin data: the
  `go generate` lines in AGENTS.md.

## Architecture

```
 user code ──► package mygo ──► platform.Backend ──► darwin | linux | windows | web | ios | fake | unsupported
                 ▲    │               ▲
                 │    └─ handlers ────┘ (AppHandler, WindowHandler)
 page (JS) ◄── bridge.js ◄── Eval (batched)
     └──── postMessage(JSON) ──► WindowHandler.Message
```

- **`package mygo` (repo root)** owns all policy: option defaults, the window
  registry, event listeners, the quit sequence, IPC routing and encoding,
  menus, protocols, trust checks. It never calls a native API directly.
  `backend_<goos>.go` picks the backend.
- **`internal/platform`** is a small synchronous contract: `Backend`,
  `Window`, `Surface`, and the handler interfaces the core implements.
  Options arrive fully defaulted; backends never invent policy, so almost all
  behavior is testable through `internal/fake`.
- **Backends** (`internal/darwin`, `linux`, `windows`, `web`, `ios`, `fake`,
  `unsupported`) only translate: AppKit/WKWebView through the Objective-C
  runtime, GTK 3/WebKitGTK through dlopen, Win32/WebView2 through syscall and
  COM, browsers through `syscall/js`.
- **Threading** (`loop.go`): on desktop the main goroutine is locked to the
  main thread in `mygo.go`'s `init`, and `App.Run` blocks in the native loop.
  `postMain` queues work and signals the backend; `onMain` runs directly or
  posts and waits; `await` pumps native events with `Backend.Step` when on
  the main thread, which is what lets `Page.Eval` and dialogs run inside
  event listeners. Calls that need the running app start with `needsApp`;
  settings made before `Run` are kept in the core until it applies them.
  Event listeners run on the main thread synchronously; IPC calls run on
  their own goroutines. Validate input in the caller's goroutine before
  hopping, so panics point at the user's call.
- **IPC**: `mygo.Bind` services, `Event[T]`, `Channel[T]` (`ipc.go`,
  `channel.go`); `internal/tsgen` generates the TypeScript client;
  `packages/runtime` (`mygo-runtime`) is what pages and generated clients
  import.
- **Native UI** (`ui`, `WindowOptions.Content`):

  ```
  view (Go) ──► ui: build, layout, paint ──► internal/scene ──► internal/gpu: d3d11 | metal | gl, or internal/raster
                  └─ internal/text: DirectWrite | Core Text | Pango | canvas
  internal/surface.Conn ◄── content.go ◄── platform.Surface (per backend)
  ```

  The backend only provides a surface and its input; renderers know nothing
  of either. GPU renderers are tested against the CPU renderer
  (`internal/raster`). `ui/editor`, `ui/richtext`, `ui/markdown`, `ui/chart`
  are built on ui's public API.
- **CLI** (`cmd/mygo`): init, generate, dev, build, doctor, plus `vet` and
  `migrate-ui` for the element API (`docs/ui/migration.md`). Its config type
  `Config` (`cmd/mygo/config.go`) is mirrored in `packages/cli/index.d.ts`.

To add a platform feature, follow "Adding a feature" in
`docs/architecture.md`: public API in `package mygo`, the smallest
`internal/platform` mechanism, every backend, core wiring with
`onMain`/`postMain`+`await`+`deliver`, a fake-backend test (plus e2e when the
toolkit matters), and before/after sizes of `examples/counter-native` and
`examples/hello`.
