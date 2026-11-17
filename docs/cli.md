# The mygo CLI

`mygo` creates, develops, packages and publishes MyGo apps.

## Install

Projects with a web frontend depend on the CLI as the `mygo-cli` npm
package, which holds a prebuilt binary for macOS, Linux and Windows on
arm64 and x64, and run it from their scripts:

```sh
bun add -d mygo-cli    # or: npm install -D mygo-cli
bun run dev            # "dev": "mygo dev" in package.json
bunx mygo doctor       # any command, in the project
```

Outside a project, run `bunx mygo-cli` or `npx mygo-cli`, e.g.
`bunx mygo-cli init my-app`: npm has an unrelated package named `mygo`, so
`bunx mygo` would run it where mygo-cli is not installed. `npm install -g
mygo-cli` puts `mygo` on your `PATH`. `MYGO_CLI_BINARY` makes the package
run another build of the CLI.

Projects of [native UI](ui/README.md) have no npm packages: their `go.mod` has
the CLI as a [tool](https://go.dev/doc/modules/managing-dependencies#tools),
which `go tool mygo` runs at the version it pins.

The CLI is a Go program, which Go runs too, without installing it:

```sh
go run github.com/egoist/mygo/cmd/mygo@latest init my-app
```

`go install` puts `mygo` on your `PATH`:

```sh
go install github.com/egoist/mygo/cmd/mygo@latest
```

Either way, building apps needs Go, which compiles them.

The commands that take a project directory, `[dir]`, default to the
current directory.

## mygo init

```
mygo init [flags] <dir>
```

Creates a project in a new or empty directory: a Go module and a
TypeScript frontend built with Vite, side by side, a default icon in
`resources/icon.png`, and a package.json with the scripts `dev`, `build`
and `generate`. It installs the dependencies with Bun and generates the
TypeScript client. See [a web frontend](getting-started.md#a-web-frontend).

With `-template native` the project is a Go module alone, whose window
shows [native UI](ui/README.md): `main.go`, a test of its view, `mygo.json` and the
icon. The module has the CLI as a [tool](https://go.dev/doc/modules/managing-dependencies#tools),
so `go tool mygo dev` and `go tool mygo build` run the version it pins,
without Bun. See [native UI](getting-started.md#native-ui).

Both templates include the `mygo-maintenance` agent skill in
`.agents/skills/mygo-maintenance/`, with guidance on element lifetimes,
stable identity, threading, and testing when maintaining the app.

| Flag | |
|---|---|
| `-template` | `web`, a TypeScript frontend (the default), or `native`, native UI in Go |
| `-name` | the app's name (default: the directory's name) |
| `-module` | the Go module path (default: the directory's name) |
| `-mygo` | a checkout of MyGo to use, through a `replace` directive, instead of the released module; the scripts then run the checkout's CLI with `go run`; run `bun install && bun run build` in the checkout first, for the web template |

## mygo install-skills

```sh
mygo install-skills [dir]
```

Installs or updates the agent skills bundled with the CLI version you run
in an existing project's `.agents/skills/`. The directory defaults to the
current directory; no project configuration, Go build, or frontend tools
are needed. New projects receive these skills through `mygo init` too.

The command overwrites bundled files such as
`mygo-maintenance/SKILL.md` and `mygo-maintenance/agents/openai.yaml`,
including local edits to those files. Other skills and extra custom files
are preserved. Upgrade the CLI, then run the command again to refresh its
guidance.

## mygo dev

```
mygo dev [flags] [dir]
```

Develops the app with live reload. It writes the TypeScript client, runs
`devCommand` from the configuration, such as a Vite dev server, waits for
`devUrl` to answer, then builds a development app, which loads `devUrl` in
place of its built frontend, and starts it. Without `devUrl` the app serves
`frontendDist` from disk.

Changes to the Go code, the configuration, the icon, the resources or the
[catalogs](i18n.md) rebuild the app, regenerate the TypeScript client and
the functions of the catalogs, and restart the app: the running
build quits, then the new one starts, so the two never hold the same files,
locks or web view profile at once. A build that fails to compile keeps the
running one. Frontend changes are the dev server's to handle. Quitting the app, or
Ctrl+C, ends mygo dev (press Ctrl+C again not to wait for the app to quit),
and `App.Relaunch` restarts the app, with the arguments it passes.

On a terminal, each step shows a spinner with what it does, such as the Go
packages being compiled, then how long it took; what the dev server and the
app print is labeled `web │` and `app │`, and a rebuild names the files that
changed. Type a key, then Enter, as in Vite: `r` rebuilds and restarts the
app, `c` clears the console, `q` quits and `h` lists them. `NO_COLOR` turns
colors off, and output to a file or a pipe has a line per step.

The development app is named `<name> Dev`, with the identifier
`<identifier>.dev`, so that its data, preferences and single instance lock
stay apart from the installed app's. On macOS it is a real app bundle, in
`.mygo/dev`; on Windows its executable carries the icon, manifest and
version information that `mygo build` embeds.

On macOS mygo dev signs the development app with an Apple Development
identity from the user's keychains, as Xcode signs Debug builds: the one of
the team of `macos.signingIdentity` when there are several. Xcode makes one
for any Apple ID, free, under Settings > Accounts > Manage Certificates.
Its signature stays the same from build to build, so macOS keeps what the
user allowed the development app, such as Accessibility, Screen Recording
or the camera, and "Always Allow" for the Keychain items it reads through
[`mygo.Secrets`](native.md#secrets). Without one, mygo dev signs ad hoc and
says so: ad hoc signatures change with every build, and macOS takes each
one for another app. `-sign` uses another identity, which a provisioning
profile needs, and `-sign -` signs ad hoc.

| Flag | |
|---|---|
With `-web`, mygo dev develops an app of [native UI in browsers](ui/browsers.md)
instead: it compiles the app to WebAssembly, serves it at
`http://localhost:5174` (or the next free port), opens it, and rebuilds it as
its Go code changes, which reloads the page. A build that fails to compile
leaves the page running the last one, and shows why over it until a fix
compiles. `o` opens the page again.

| Flag | |
|---|---|
| `-skip-dev-command` | does not run `devCommand`, e.g. when the dev server already runs |
| `-sign` | the identity that signs the development app on macOS (default: an Apple Development identity, see below, else `-`, ad hoc) |
| `-web` | develops the app in browsers |
| `-port` | with `-web`, the port to serve the app at, or the next free one (default: 5174) |
| `-open` | with `-web`, opens the app in the browser (default: true) |
| `-lang` | runs the app in a language, such as `de` or `zh-Hant`, or a pseudo locale, `en-XA` or `ar-XB`, without changing the language it keeps; see [trying a language](i18n.md#trying-a-language) |
| `-background` | starts the app, and again after each rebuild, without bringing it to the front: its windows show without the focus until you click one, so the editor keeps it as you save (a maximized window on Windows still takes it) |

## mygo build

```
mygo build [flags] [dir]
```

Builds production apps: runs `buildCommand`, compiles the app with
`frontendDist` embedded, and packages it for each platform in
`<out>/<os>-<arch>`. See [Building and distributing](distribution.md).

The platform `web` (or `js/wasm`) builds an app of native UI for browsers
into `<out>/web`, a static site to serve from anywhere: `index.html`, which
loads the app, Go's `wasm_exec.js`, the app's WebAssembly, named by its
content so that it can be cached for good, and the icon. See
[Browsers](ui/browsers.md).

| Flag | |
|---|---|
| `-platform` | comma separated `GOOS/GOARCH` targets, e.g. `darwin/universal,windows/amd64,linux/amd64`, or `web` (default: this machine's) |
| `-debug` | keeps development features, such as the web inspector and the [inspector of native UI](ui/inspector.md) |
| `-o` | the output directory (default: `out` of the configuration) |
| `-sign` | the macOS signing identity (default: `macos.signingIdentity`) |
| `-skip-build-command` | does not run `buildCommand` |
| `-skip-dmg` | does not make macOS disk images |
| `-skip-notarize` | does not notarize, even with `macos.notarize` set |
| `-skip-installer` | does not make Windows installers, nor download NSIS for them (nor `install.ps1`, which runs the installer) |
| `-upload` | uploads the installers and updates to a draft GitHub release of the version (`updates.github`), with `gh`, or to the bucket of `updates.s3` |

## mygo generate

```
mygo generate [-o file] [dir]
```

Writes the typed TypeScript client for the services bound with `mygo.Bind`
and the events declared with `mygo.NewEvent`, to `bindings` of the
configuration unless `-o` names another file. `mygo dev` and `mygo build` run
it for you; apps without a frontend get no client. See
[the generated client](bindings.md#the-generated-client).

With `i18n` in the configuration, it first checks the app's
[catalogs](i18n.md) and writes their functions: `catalog_gen.go` in the
catalogs' directory, and a TypeScript module next to the client, named
after the directory (`src/locales.ts`). Errors in the catalogs stop it.

## mygo i18n check

```
mygo i18n check [dir]
```

Checks the app's [catalogs](i18n.md#checking-the-catalogs): it lists their
errors, such as messages that do not parse or arguments a translation adds,
and their warnings, such as missing translations, and exits with status 1
when there are errors.

## mygo native

Experimental, as native modules are: these commands and their flags may
change in any release. The commands of [native modules](native-modules.md): Go interfaces
implemented in Swift, Kotlin, Rust or Zig, built into a library per
platform (on phones, with the app), and
of the C libraries that packages bind with `ffi`
([bind a C library yourself](native-modules.md#bind-a-c-library-yourself)).
`[module dir]` defaults to the current directory.

### mygo native build

```
mygo native build [flags] [dir]
```

Builds the [native modules](native-modules.md) in `dir`, each in the
language of each platform (Swift on macOS where a module has it,
otherwise Rust, then Zig; phones build theirs with the app), and the libraries that the packages in `dir`
name in their `mygo-plugin.json`, from their sources, with swift, cargo
or zig, for every platform, writes the files to publish into
`.mygo/native/release` of the module, named after their SHA-256, and
their names, URLs and SHA-256 into `mygo-plugin.json`. Swift builds on a
Mac, universal, as a `.tar.gz` with the frameworks it brings
(`darwin-universal`). Platforms that do not build here (Rust's and
Swift's macOS libraries off a Mac) are listed at the end and left as they
were. Flags go before the directory. `mygo dev` and `mygo build` build the
modules and libraries of the app's own packages themselves; this
publishes those of a package for the apps of others, and for machines
without the languages' tools.

| Flag | |
|---|---|
| `-target` | the platforms, `GOOS-GOARCH` separated by commas, `all` (the default) or `host`, which builds this machine's library for `go run` and `go test` and leaves `mygo-plugin.json` alone |
| `-lang` | builds modules in this language (`swift`, `rust` or `zig`) where they are implemented in it, rather than in each platform's: with `-target host`, to test each implementation |
| `-release` | the URL of the directory the files are published in, as `https://github.com/<owner>/<repo>/releases/download/<tag>` (default: the library's `release`, else with `-upload` the library's release `native-<name>` of the repository of the git checkout) |
| `-out` | the directory of the files to publish |
| `-upload` | uploads the files to the GitHub release of `-release` with the GitHub CLI, creating it if needed |
| `-force` | builds the platforms whose published files were built from the sources as they are too, which it otherwise skips |

### mygo native new

```
mygo native new [-lang <languages>] <module name>
```

Makes a module in `modules/<module name>`, or at the path given instead of
a name: the Go package with the interface, its generated Go, an
`AGENTS.md`, and a directory per language. `-lang` takes the languages,
separated by commas: `swift` (the default), `kotlin`, `rust`, `zig`, or `none` for
the Go side alone.

### mygo native add

```
mygo native add <language> [module dir]
```

Implements an existing module in another language, `swift`, `kotlin`,
`rust` or `zig`, with a factory for every entry point and methods that say they are
unsupported, so the module builds at once.

### mygo native check

```
mygo native check [-lang <language>] [module dir]
```

Generates the module, builds its implementation for this machine, or the
one in `-lang`, and vets the Go package. It lists every problem as
`file:line: message`, with what to change, and exits with status 1 when
there are problems.

### mygo native call

```
mygo native call [-timeout <duration>] [module dir] <Interface>.<Method> [<arguments>]
```

Makes the implementation of an entry point, calls one method with
`<arguments>`, a JSON object of its parameters by their Go names, and
prints the result as JSON, a line per value of a stream. It makes the
method's context, which `-timeout` limits, and prints the calls of
callbacks.

`mygo dev` and `mygo build` generate and build every native module of the
app; see
[build, run and test](native-modules.md#build-run-and-test).

## mygo keygen

```
mygo keygen [flags]
```

Creates the key pair that signs [updates](updates.md): `mygo-update.key`,
the secret, and `mygo-update.pub`, and prints the `updates` section to add
to the configuration.

| Flag | |
|---|---|
| `-o` | the directory to write the keys to (default: `mygo/update-keys` in the user's configuration directory) |
| `-force` | replaces existing keys |

## mygo doctor

Checks the development machine: Go, Bun and the webview of the platform,
which projects with a web frontend need (on Linux GTK 3 too, and not
WebKitGTK in a project whose `linux.webview` is `false`), and the tools of optional
features: NSIS for Windows installers (which `mygo build` downloads on
Windows), `signtool` or `osslsigncode` for signing Windows apps, `gh` for
`-upload`, and Developer ID identities for signing macOS apps.

## mygo version

Prints the version of the CLI.

## Environment variables

| Variable | |
|---|---|
| `CGO_ENABLED` | `0` by default when unset or empty; set to `1` for app dependencies that need cgo, including in `mygo dev`, `mygo build` and `mygo generate` |
| `MYGO_LDFLAGS` | more `-ldflags` for the builds of `mygo build`, after its own, such as `-X example.com/app/internal/crash.DSN=…` or `-buildid=`; split at spaces outside quotes, as `go build -ldflags` splits them |
| `GOFLAGS` | Go's flags; `mygo build` adds the tags of its `-tags=…` to its own, such as `-tags=mygo_nowebview` for a Windows app [without WebView2](ui/windows.md#no-webview) |
| `MYGO_INSPECTOR` | `1` keeps the [inspector of native UI](ui/inspector.md) in production builds of `mygo build`, which leave it out |
| `MYGO_UPDATER_PRIVATE_KEY` | the secret key that signs updates, for `mygo build` |
| `MYGO_WINDOWS_CERTIFICATE_PASSWORD` | the password of `windows.certificate`, for `mygo build` |
| `MYGO_CLI_BINARY` | a build of the CLI for the `mygo-cli` package to run |
| `MYGO_ENV` | `production` makes an app behave like a production build, e.g. without the web inspector |
| `MYGO_NATIVE_<NAME>` | the file of the [native library](native-modules.md#where-libraries-are-found) `<name>` (in upper case, with `_` for other characters than letters and digits) that package `ffi` loads, over the others |
| `MYGO_LOCALE` | the language the app runs in, over the one it keeps, which `mygo dev -lang` sets |

For an app with dependencies that need cgo:

```sh
CGO_ENABLED=1 go tool mygo dev
CGO_ENABLED=1 go tool mygo build
```
