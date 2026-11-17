# SQLite

SQLite 插件为网页和 Go 提供本地数据库，包括原生 UI 应用。它的 SQLite 是一个[原生模块](../native-modules.md)：在桌面上用 Zig 编写，按平台发布预构建版本，所以应用构建时不需要 cgo、C 编译器或 Zig；在 iOS 上用 Swift、在 Android 上用带 C 的 Kotlin 编写，随应用一起构建。它支持 macOS、Linux 和 Windows 上的 amd64 与 arm64，iOS 16 及以上，以及 Android。

## 使用页面插件

仅当 Web 前端使用 `@mygo-plugins/sqlite` 时，才调用 `mygo.Use(sqlite.Plugin)`。它注册该 TypeScript 客户端调用的 Go 服务。Go 和原生 UI 应用直接使用 `sqlite.Open`，如[下文](#从-go-或原生-ui)所示。

```go
import (
    "github.com/egoist/mygo"
    "github.com/egoist/mygo/plugins/sqlite"
)

func main() {
    mygo.Use(sqlite.Plugin)
    // Create windows and call App.Run as usual.
}
```

用 `bun add @mygo-plugins/sqlite` 安装前端包，然后：

```ts
import { open } from "@mygo-plugins/sqlite";

const db = await open("app.db");
await db.execute("CREATE TABLE IF NOT EXISTS notes (id INTEGER PRIMARY KEY, title TEXT)");
const result = await db.execute("INSERT INTO notes(title) VALUES (?)", ["Hello SQLite"]);
const { columns, rows } = await db.query("SELECT id, title FROM notes WHERE id = ?", [result.lastInsertId]);
// columns: ["id", "title"], rows: [[1, "Hello SQLite"]]
await db.close();
```

`open` 接受数据库文件名，例如 `app.db`，或 `:memory:` 表示私有的内存数据库。文件位于 `mygo.PathUserData`。绝对路径、目录遍历、符号链接的数据库文件和 Windows 设备名会被拒绝。要选择目录和锁等待时间：

```go
mygo.Use(sqlite.New(sqlite.Options{
    Directory: "/path/to/databases",
    BusyTimeout: 2 * time.Second,
}))
```

`Directory` 默认为 `mygo.PathUserData`；对于可写数据库，目录不存在时会自动创建。`BusyTimeout` 默认为五秒；负的时长会禁用等待。`open("app.db", { readOnly: true })` 打开现有文件而不允许写入。

## 查询和值

`execute(sql, params?)` 只接受一条语句，会读取完所有 `RETURNING` 结果行，并返回 `{ changes, lastInsertId }`。`changes` 是直接受影响的行数；`lastInsertId` 是该连接上 SQLite 最后插入的 rowid，在 TypeScript 中始终为 `bigint`。

`query(sql, params?)` 返回 `{ columns, rows }`。每一行是按列顺序排列的单元格数组，保留重复的列名。它支持 `SELECT`、pragma 以及带 `RETURNING` 的语句。结果会被缓冲；对于大表，使用 `LIMIT` 和分页。

参数按其 SQLite 索引绑定：接受 `?`、`?NNN`、`:name`、`$name` 和 `@name`，值通过数组提供。所有参数索引都必须有值；缺少或多余的参数会失败。绝不要把应用中的值插值进 SQL 字符串。多个语句会在第一个执行之前被拒绝；允许末尾的分号和注释。

| SQLite 存储类 | TypeScript | Go |
| --- | --- | --- |
| NULL | `null` | `nil` |
| INTEGER | 安全时为 `number`，否则为 `bigint` | `int64` |
| REAL | `number` | `float64` |
| TEXT | `string` | `string` |
| BLOB | `Uint8Array` | `[]byte` |

布尔参数会变成整数 0 或 1。整数超出 JavaScript 的安全范围时，绑定 `bigint`；SQLite 的有符号 64 位范围之外的值会失败。非有限的数值参数会失败。空 BLOB 和空文本与 NULL 仍然不同。文本和 BLOB 参数由 SQLite 复制，结果 BLOB 会在语句关闭之前复制。

## 事务和生命周期

```ts
await db.transaction([
  { sql: "INSERT INTO notes(title) VALUES (?)", params: ["first"] },
  { sql: "INSERT INTO notes(title) VALUES (?)", params: ["second"] },
]);
```

`transaction` 在 COMMIT 之后为每个语句返回一个执行结果。它使用 `BEGIN IMMEDIATE`，在整个批次期间保持连接，并在任何错误或取消时回滚。空批次返回 `[]`。事务控制语句（`BEGIN`、`COMMIT`、`ROLLBACK`、`SAVEPOINT`、`RELEASE`）在用户 SQL 中会被拒绝；使用批次 API。`ATTACH`、`DETACH` 和 `VACUUM INTO` 会被拒绝，使 SQL 无法打开配置目录之外的文件。普通的 `VACUUM` 受支持。SQLite 扩展无法加载。

每个连接归发起调用的页面所有，其他窗口不能使用其句柄。页面导航或窗口关闭会取消调用，并关闭该页面的连接；应用退出会关闭所有剩余连接。`close()` 是幂等的，还会取消正在进行的操作。同一连接上的并发操作会串行执行；存在先后依赖时，应等待前一次调用完成后再发起下一次。

取消操作时，会通过 `sqlite3_interrupt` 中断正在执行的语句，以及对其他连接所持锁的等待。Go 调用者可以通过各次操作的上下文取消操作。

## 从 Go 或原生 UI

不需要 `mygo.Use(sqlite.Plugin)` 注册。直接打开连接，并在完成后关闭它：

```go
ctx := context.Background()
db, err := sqlite.Open(ctx, "app.db", sqlite.OpenOptions{})
if err != nil { return err }
defer db.Close()

_, err = db.Execute(ctx, "CREATE TABLE IF NOT EXISTS notes (title TEXT)")
if err != nil { return err }
_, err = db.Transaction(ctx, []sqlite.Statement{
    {SQL: "INSERT INTO notes VALUES (?)", Args: []any{"first"}},
    {SQL: "INSERT INTO notes VALUES (?)", Args: []any{"second"}},
})
if err != nil { return err }
rows, err := db.Query(ctx, "SELECT title FROM notes")
// rows.Columns and rows.Values contain copied results.
```

Go 的 `Open` 直接接受路径；其父目录必须存在。连接可以从任何 goroutine 安全使用。连接的生命周期由 Go 调用者管理，调用者必须调用 `Close`。`sqlite.Error` 暴露 SQLite 的主要错误码和扩展错误码；已关闭的连接返回 `sqlite.ErrClosed`，被取消的操作返回各自的上下文错误。

<!-- repository-only:start -->

## 构建与发布

SQLite 在原生模块 `plugins/sqlite/internal/sqlitecore` 中运行：一个 Go 接口 `Engine`，其 Zig 实现通过编译 SQLite 的合并源码（amalgamation）来实现该接口，`zig/build.zig.zon` 用哈希固定这份源码。`mygo dev` 和 `mygo build` 把该平台的模块库放进应用的资源中，在 macOS 上与应用一起签名；应用的 `go run` 和 `go test` 则会下载它。打包的应用从不在运行时下载原生代码。

手机运行的是同一个 SQLite，版本为 3.53.4，编译选项相同，随应用一起构建（[手机](../native-modules.md#手机)）。在 iOS 上，它的 Swift 从 `CSQLite.xcframework` 链接 SQLite：这个 XCFramework 由 `swift/build-xcframework.sh` 从合并源码构建，存放在发布版本 `native-sqlitecore` 中，由 `Package.swift` 中的校验和固定。系统自带的 SQLite 在较旧的 iOS 上版本更旧，编译选项也不同。在 Android 上，它的 Kotlin 通过 CMake 从合并源码构建的 C 调用 SQLite，合并源码由 SHA-256 固定，需要 Android SDK Manager 中的 CMake。

在这个仓库中，模块用 Zig 0.16.0 从源代码构建：

```sh
go run ./cmd/mygo native build -target host plugins/sqlite/internal/sqlitecore
```

在模块未构建的平台上，插件的测试会跳过。`mygo native build -upload` 为每个平台发布它（见[发布模块](../native-modules.md#发布模块)）。

关于发布版本资源的上传与编译选项，见[插件 README](https://github.com/egoist/mygo/tree/main/plugins/sqlite)。SQLite 上游的 [C API](https://www.sqlite.org/c3ref/intro.html) 描述它的 SQL 行为。

<!-- repository-only:end -->
