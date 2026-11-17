# SQLite

SQLite for MyGo, with Go connections and the `@mygo-plugins/sqlite`
TypeScript client. Its SQLite is a native module (`internal/sqlitecore`),
written in Zig on desktops, published prebuilt for each platform, so apps
build without cgo, a C compiler or Zig, and in Swift on iOS and Kotlin on
Android, which build with the app.

Register `mygo.Use(sqlite.Plugin)` only when the web frontend uses
`@mygo-plugins/sqlite`. Go and native UI apps call `sqlite.Open` directly;
they need no plugin registration and close their own connections.

```go
import (
    "github.com/egoist/mygo"
    "github.com/egoist/mygo/plugins/sqlite"
)

func main() {
    mygo.Use(sqlite.Plugin)
    // Create windows, then call mygo.App.Run().
}
```

```ts
import { open } from "@mygo-plugins/sqlite";

const db = await open("app.db");
await db.execute("CREATE TABLE IF NOT EXISTS tasks (id INTEGER PRIMARY KEY, title TEXT, done INTEGER)");
const inserted = await db.execute("INSERT INTO tasks(title, done) VALUES (?, ?)", ["Learn MyGo", false]);
const { columns, rows } = await db.query("SELECT id, title, done FROM tasks WHERE done = ?", [false]);
await db.transaction([
  { sql: "UPDATE tasks SET done = ? WHERE id = ?", params: [true, inserted.lastInsertId] },
  { sql: "INSERT INTO tasks(title, done) VALUES (?, ?)", params: ["Ship the app", false] },
]);
await db.close();
```

Use `bun add @mygo-plugins/sqlite` once the npm package is published. In a
checkout, `bun install && bun run build` installs and builds the workspace
package. See [the full guide](../../docs/plugins/sqlite.md) for the Go API,
types, cancellation, options and packaging.

## Building the native module

`internal/sqlitecore` declares the interface the plugin calls, `Engine`,
and `internal/sqlitecore/zig` implements it, compiling SQLite **3.53.4**,
which `build.zig.zon` pins by the hash of sqlite.org's amalgamation. Its
library exports only the module's functions: SQLite's own symbols stay
hidden, so a process that loaded another SQLite globally (WebKit does, on
Linux) cannot interpose on them.

With Zig **0.16.0** on the path, from the repository's root:

```sh
# Build this machine's library, which go test and go run use:
go run ./cmd/mygo native build -target host plugins/sqlite/internal/sqlitecore
# Build and publish every platform's:
go run ./cmd/mygo native build -upload plugins/sqlite/internal/sqlitecore
```

Linux targets glibc 2.28 or later; macOS targets 12 or later.

On iOS, `internal/sqlitecore/swift` links the same SQLite, built with the
same options by `swift/build-xcframework.sh` on a Mac into
`CSQLite-3.53.4.xcframework.zip`, an asset of the release
`native-sqlitecore` whose checksum `Package.swift` pins. On Android,
`internal/sqlitecore/kotlin` compiles it with CMake, which downloads the
amalgamation and checks its SHA-256. Upgrading SQLite changes the
version and hash in `build.zig.zon`, `build-xcframework.sh` and
`kotlin/src/main/c/CMakeLists.txt`, and the XCFramework's URL and
checksum.

SQLite is public domain; the plugin and its module use the repository's MIT
license. FTS5, RTree and JSON are enabled, with foreign keys on by default.
Loadable extensions, shared cache and deprecated APIs are omitted.
