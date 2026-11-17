# 原生模块

**实验性。** 原生模块，以及用 `ffi` 绑定 C 库，可能在任意发布版本中变化，包括破坏性变化：Go 侧、生成的 Swift、Kotlin、Rust 和 Zig、`mygo native` 命令以及 `mygo-plugin.json`。发布说明会列出每项破坏性变化以及该怎么应对。

原生模块是用 Swift、Kotlin、Rust 或 Zig 实现的 Go 接口。你在 Go 中声明接口，再按照 MyGo 生成的声明编写实现，就能像调用普通 Go 值一样使用它：支持 `context.Context`、错误、迭代器和回调，Go 侧由 Go 编译器检查，实现侧由相应语言的编译器检查。不使用 cgo，也无需手写两种语言之间的胶水代码。

适合用在 Go 缺少或做得慢的地方：

- macOS 和 iOS 仅向 Swift 提供的功能：系统框架（NaturalLanguage、Vision、AVFoundation、Speech……）、Swift 包，以及嵌入原生 UI 的视图；
- Android 用 Kotlin 和 Java 提供的功能：它的 SDK、Jetpack 和其他 Android 库，以及视图；
- 在所有平台上使用 Rust crate 或 Zig、C 库，例如解析器、编解码器、数据库或推理引擎。

模块把每种实现放在以对应语言命名的目录里，用该语言的工具构建：

| 目录 | 语言 | 运行于 |
|---|---|---|
| `swift/` | Swift，作为 Swift 包 | macOS 和 iOS，以其 `Package.swift` 列出的为准（以及在 `go test` 下的 Linux，仅用 Foundation） |
| `kotlin/` | Kotlin，作为 Gradle 的 Android 库 | Android |
| `rust/` | Rust，作为 crate | macOS、Linux、Windows |
| `zig/` | Zig，作为包 | macOS、Linux、Windows |

模块可以有多种实现：每个平台选一种——iOS 上用 Swift，Android 上用 Kotlin，macOS 上有 Swift 实现就用 Swift，否则用 Rust，再否则用 Zig。因此一个模块可以在 Mac 和 iPhone 上使用 Apple 的框架，在 Android 手机上使用 Android 的框架，在其他地方使用 Rust crate，而对外仍是同一个 Go 接口（见[手机](#手机)）。Go 侧说明模块做什么，每个目录说明怎么做。在模块没有实现的平台上，其构造函数返回 `errors.ErrUnsupported`，这样应用在所有平台都能编译，并自行决定没有它时该怎么办。

Go 侧：

```go
package language

// Analyzer reads natural language text.
//
//mygo:native
type Analyzer interface {
	// DetectLanguage returns the BCP 47 code of the text's language, such
	// as "en" or "zh-Hans".
	DetectLanguage(text string) (string, error)
	// Tokens yields the words, sentences or paragraphs of the text.
	Tokens(ctx context.Context, text string, unit Unit) iter.Seq2[Token, error]
}

type Unit string

const (
	Word      Unit = "word"
	Sentence  Unit = "sentence"
	Paragraph Unit = "paragraph"
)

// Token is a range of the text, in UTF-8 bytes.
type Token struct {
	Start, End int
	Text       string
}
```

它在 `swift/` 中的实现，使用 NaturalLanguage：

```swift
import NaturalLanguage

func makeAnalyzer() -> any Analyzer { NLAnalyzer() }

final class NLAnalyzer: Analyzer {
    func detectLanguage(text: String) throws -> String {
        guard let lang = NLLanguageRecognizer.dominantLanguage(for: text) else {
            throw LanguageError.undetermined
        }
        return lang.rawValue
    }

    func tokens(text: String, unit: Unit) -> AsyncThrowingStream<Token, Error> {
        AsyncThrowingStream { continuation in
            let tokenizer = NLTokenizer(unit: NLTokenUnit(unit))
            tokenizer.string = text
            tokenizer.enumerateTokens(in: text.startIndex..<text.endIndex) { range, _ in
                continuation.yield(Token(range, in: text))
                return !Task.isCancelled
            }
            continuation.finish()
        }
    }
}
```

以及调用它的应用：

```go
a, err := language.NewAnalyzer()
if err != nil {
	return err // errors.ErrUnsupported where the module has no implementation
}
lang, err := a.DetectLanguage("Bonjour tout le monde")
for tok, err := range a.Tokens(ctx, text, language.Sentence) {
	if err != nil {
		return err
	}
	fmt.Println(tok.Start, tok.Text)
}
```

你只需声明接口，其余声明和胶水代码由 MyGo 生成，并由 `mygo dev` 在编辑时保持同步：Go 侧生成 `NewAnalyzer`，实现侧生成各语言需要遵循的声明。这里包括 Swift 协议 `Analyzer`、结构体 `Token`、枚举 `Unit` 和 `LanguageError`；Rust 中生成 trait `Analyzer`，Zig 中则生成一个结构体，在编译时检查你的结构体是否实现了所有方法（见 [Rust 和 Zig](#rust-和-zig)）。改动接口后，所有实现都必须同步更新才能通过编译；漏掉方法时，相应语言的编译器会指出缺少哪个方法。

## 创建模块

```sh
mygo native new [-lang <languages>] <module name>
```

在应用的 `modules/<module name>` 下创建一个名为 `<module name>` 的模块，或者用你给出的路径代替名称，例如 `internal/<module name>`。`-lang` 指定用来实现它的语言，用逗号分隔：默认 `swift`，还有 `kotlin`、`rust`、`zig`。`-lang none` 只生成 Go 侧，以便先确定接口，之后再实现。

```
modules/<module name>/
├── <module name>.go              # the interface: yours
├── <module name>_native.go       # generated
├── AGENTS.md                     # how to write and check the module, for agents
├── mygo-plugin.json              # optional: its published builds (see publishing)
├── swift/
│   ├── Package.swift             # the Swift package: yours, for its dependencies
│   ├── Info.plist                # optional: keys added to the app's Info.plist
│   ├── Entitlements.plist        # optional: entitlements added to the app's
│   └── Sources/<Module>/
│       ├── <Module>.swift        # the implementation: yours
│       └── <Module>.mygo.swift   # generated
├── kotlin/
│   ├── build.gradle.kts          # the Android library: yours, for its dependencies
│   └── src/main/kotlin/<package as a path>/
│       ├── <Module>.kt           # the implementation: yours
│       └── <Module>.mygo.kt      # generated
├── rust/
│   ├── Cargo.toml                # the crate: yours, for its dependencies
│   └── src/
│       ├── lib.rs                # the implementation: yours
│       └── mygo.rs               # generated
└── zig/
    ├── build.zig                 # yours, with build.zig.zon for dependencies
    └── src/
        ├── <module name>.zig     # the implementation: yours
        └── mygo.zig              # generated, the library's root
```

模块名就是其 Go 包名，因此必须是合法的包名，例如 `language` 或 `media`。`<Module>` 是大驼峰形式的名称，即 Swift 模块和库的名称：`language` 对应 `Language`，构建为 `libLanguage.dylib`、`libLanguage.so` 和 `Language.dll`。

要用另一种语言实现已有模块：

```sh
mygo native add <language> [module dir]
```

该命令在模块中创建对应语言的目录（模块目录默认为当前目录），为每个入口点生成工厂函数，并生成所有方法均返回“不支持”的占位实现。模块因此可以立即构建，之后再逐个实现方法。如果模块用到了 Kotlin、Rust 或 Zig 无法实现的功能，就不能添加相应的实现（见 [Rust 和 Zig](#rust-和-zig) 和[手机](#手机)）。

模块就是普通的 Go 包，可以从应用或其他 Go 模块中导入。`mygo dev` 和 `mygo build` 通过 `go list -deps` 找出依赖中所有带实现的原生模块包，为目标平台构建实现并放入应用，无需额外注册。

提交生成的文件。`<module name>_native.go` 让应用在任意机器上编译、vet 和测试，无论是否安装了这些语言的工具；`<Module>.mygo.swift`、`<Module>.mygo.kt`、`mygo.rs` 和 `mygo.zig` 保存 Go 侧的声明和 Go 调用的 C 函数，让编辑器补全并检查实现；永远不要编辑它们。它们会注明自己来自哪个接口，当某个文件比接口旧时，`mygo build` 会拒绝构建。

## 声明接口

模块的 Go 侧就是模块的契约：接口、这些接口使用的类型和返回的错误，其中不含任何平台相关的东西。

**入口点。** 接口前的注释 `//mygo:native` 使它成为模块的入口点，包会因此得到一个构造函数：接口 `T` 得到 `NewT() (T, error)`。该指令不接受任何选项。像打开文件一样，只创建一次这个值，然后一直留着。

**工厂。** 每个实现都通过一个与入口点同名的工厂函数决定用什么来实现它；生成的代码会调用这个函数，编译器也要求它存在：`Analyzer` 在 Swift 和 Zig 中是 `makeAnalyzer`，在 Rust 中是 `make_analyzer`。没有它包就无法编译，Swift 会说 `cannot find 'makeAnalyzer' in scope`。

```swift
func makeAnalyzer() throws -> any Analyzer {
    guard #available(macOS 14, *) else { throw NativeError.unsupported }
    return NLAnalyzer()
}
```

工厂可以抛出错误，构造函数会返回该错误。`NativeError.unsupported` 在 Go 中对应 `errors.ErrUnsupported`，表示系统缺少所需功能，例如某个框架要求更新的系统版本。Swift 实现支持的 macOS 版本由 `Package.swift` 中的 `platforms` 声明；在更旧的版本或没有实现的平台上，构造函数不会调用工厂，直接返回 `errors.ErrUnsupported`。

**其他接口。** 入口点的方法接收或返回的接口也是模块的一部分，会与模块一起生成，但没有构造函数。它们不需要指令。由谁实现取决于它出现的位置：模块的方法返回的接口，例如已加载的模型，由平台实现（见[对象](#对象)）；方法只接收的接口，例如委托，由 Go 实现（见 [Go 实现的接口](#go-实现的接口)）。如果某个接口既是返回的又是只接收的，MyGo 会报错停止。

**能力。** 方法以外的能力，通过 Go 的接口嵌入来声明：

- `native.View`：实现是平台的视图，用 `ui.NativeView` 显示在原生 UI 中（见[视图](#视图)）。
- `io.Closer`，或 `Close() error` 方法：对象在 Go 关闭它时被释放（见[对象](#对象)）。

**线程。** 接口前的注释 `//mygo:mainthread` 使其所有方法在主线程上运行；放在方法前则只作用于该方法。在 Swift 中它们是 `@MainActor`；Rust 和 Zig 的实现则在主线程上被调用。视图无需该指令就在主线程上运行。

接口及其方法、类型的 Go 文档注释会变成生成代码的文档注释，因此每个实现读到相同的契约。

### 方法

方法可以：

- 接收任意多个下面的[类型](#类型)（位于可选的 `context.Context` 之后），以及函数参数，即[回调](#回调)；
- 不返回任何东西、返回一个值、一个错误、一个值和一个错误、一个 `iter.Seq[T]` 或 `iter.Seq2[T, error]`，或一个用于停止该方法所启动之事的 `io.Closer`（见[订阅](#订阅)）。

在 Swift 中（Rust 和 Zig 的见[下文](#rust-和-zig)）：

| Go | Swift |
|---|---|
| `M(a int, b string)` | `func m(a: Int, b: String)` |
| `M(...) error` | `func m(...) throws` |
| `M(...) (T, error)` | `func m(...) throws -> T` |
| `M(ctx context.Context, ...) (T, error)` | `func m(...) async throws -> T` |
| `M(ctx context.Context, ...) iter.Seq2[T, error]` | `func m(...) -> AsyncThrowingStream<T, Error>` |
| `M(ctx context.Context, ...) iter.Seq[T]` | `func m(...) -> AsyncStream<T>` |
| `M(..., onChange func(float64))` | `func m(..., onChange: @escaping @Sendable (Double) -> Void)` |
| `M(..., allow func(url string) bool)` | `func m(..., allow: @escaping @Sendable (String) -> Bool)` |
| `M(..., check func(string) error)` | `func m(..., check: @escaping @Sendable (String) throws -> Void)` |
| `M(...) io.Closer` | `func m(...) -> any Closer` |
| `//mygo:mainthread` | `@MainActor` |

方法名变成小驼峰（`DetectLanguage` → `detectLanguage`，`URLFor` → `urlFor`），参数保留其 Go 名称作为 Swift 标签。

**上下文。** 第一个参数是 `context.Context` 的方法，在 Swift 中是 `async` 方法，并在 `Task` 中运行。取消上下文会取消该任务，因此 `Task.isCancelled` 和 `try Task.checkCancellation()` 能看到它；可取消的 await，例如 `Task.sleep`，会抛出 `CancellationError`，Go 会以 `ctx.Err()` 收到。上下文的截止时间一到，任务就会被取消。上下文不会作为参数传给实现：在实现里，它只用于传递取消。没有上下文的方法是同步的，在调用方 goroutine 的线程上运行，或在有 `//mygo:mainthread` 时在主线程上运行：让这些方法保持快速，因为视图和处理器会在主线程上调用它们。

**流。** 方法返回的迭代器会依次产出 Swift 流中的值。循环开始时启动流；循环中断、上下文取消、流结束或抛出错误时停止流。如果 Go 先停止迭代，就会取消流的任务并运行 `onTermination`：应在其中停止流启动的工作。

### 回调

函数参数会变成一个 `@Sendable` 闭包，实现可以保存它，并从任意线程调用任意多次。如果方法返回订阅，闭包在订阅关闭前有效；否则，其有效期与方法所属对象的生命周期相同。

- 没有返回值的回调是通知：闭包立即返回，Go 之后在主线程上运行该函数，与所有处理器一样，顺序与实现调用的顺序相同。
- 有返回值的回调是提问：闭包等待答案，Go 在调用它的线程上立即运行该函数。系统在主线程上提问时（大多数委托如此）要尽快回答，绝不要在那里等待需要主线程的工作。回调的 `error` 结果会在 Swift 中以抛出错误的形式返回。

### 类型

参数、结果和结构体的字段使用这些类型：

| Go | Swift |
|---|---|
| `bool`, `int`, `int64`, `uint8` … | `Bool`, `Int`, `Int64`, `UInt8` … |
| `float32`, `float64` | `Float`, `Double` |
| `string` | `String` |
| `[]byte` | `Data` |
| `time.Time`, `time.Duration` | `Date`, `Duration` |
| `[]T`, `map[string]T` | `[T]`, `[String: T]` |
| `*T` | `T?` |
| 包的一个结构体 | 一个字段相同的 `struct`，`Sendable`，字段可哈希时也是 `Hashable` |
| 带常量的 `type Unit string`（或整数类型） | `enum Unit: String`，每个常量一个 case |
| 模块的一个接口 | 生成的协议，作为 `any P` |
| 另一个模块的接口或类型 | 该模块中的类型，来自它的 Swift 模块（见[使用其他模块的模块](#使用其他模块的模块)） |
| `*mygo.Window` | `NSWindow`（见[窗口和锚点](#窗口和锚点)） |
| `ui.Rect` | `CGRect`，在窗口内容视图的坐标系中 |
| `native.View` | `NSView` |
| `*native.Buffer` | `NativeBuffer`（见[缓冲区](#缓冲区)） |
| `*native.Texture` | `NativeTexture`（见[纹理](#纹理)） |

导出字段变成小驼峰属性；未导出的字段被省略。结构体和切片跨边界时会复制，因此双方都看不到对方之后的改动。`[]byte` 参数在调用期间借给 Swift，不复制；如果要保留，就用 `Data(data)` 复制一份。

除上表列出的类型外，MyGo 会在声明所在行报错：不支持通道、`any`、键不是 string 的 map、泛型类型，以及其他包的类型。这类数据应改用包内定义的结构体表示。

Swift 自有的、Go 没有对应物的类型，例如带关联值的枚举、泛型类型和重载方法，留在实现内部：在那里把它们转换为结构体，或转换为一个每个 case 一个字段的结构体，并用一个 `type Kind string` 说明设置了哪一个。

### 错误

把实现可能返回的错误声明为包级变量，用 `native.NewError` 和一个错误代码创建：

```go
var (
	ErrUndetermined = native.NewError("undetermined")
	ErrNoEmbedding  = native.NewError("no_embedding")
)
```

Swift 会把它们变成一个以模块命名的枚举 `<Module>Error`，每个错误一个小驼峰 case（`ErrNoEmbedding` → `.noEmbedding`）：

```swift
guard let lang = NLLanguageRecognizer.dominantLanguage(for: text) else {
    throw LanguageError.undetermined
}
```

Go 用 `errors.Is(err, language.ErrUndetermined)` 比较它们。Swift 抛出的其他错误会以不属于模块错误的 `*native.Error` 到达 Go，并带有错误的域或类型（`"NSCocoaErrorDomain"`、`"ParseError.badHeader"`）、错误码和 `localizedDescription`。`NativeError.unsupported` 以 `errors.ErrUnsupported` 到达 Go。

实现中的崩溃，例如 `fatalError`、强制解包 `nil` 或下标越界，会结束应用，如同 Go 未 recover 的 panic 一样。Rust 的 panic 则在边界处被捕获，并作为错误到达 Go。

## Rust 和 Zig

同一个模块也可以用 Rust 实现，并在所有平台上运行：`rust/` 是一个 crate，其 `lib.rs` 声明生成的 `mod mygo;` 并实现它的 trait，并在 `Cargo.toml` 中使用一个来自 crates.io 的 crate：

```rust
mod mygo;

use mygo::*;
use std::sync::Arc;
use unicode_segmentation::UnicodeSegmentation;

pub fn make_analyzer() -> Result<Arc<dyn Analyzer>, Error> {
    Ok(Arc::new(Segmenter))
}

struct Segmenter;

impl Analyzer for Segmenter {
    fn detect_language(&self, _text: String) -> Result<String, Error> {
        Err(Error::Unsupported) // errors.ErrUnsupported in Go
    }

    fn tokens(&self, _ctx: &Context, text: String, unit: Unit) -> Stream<Result<Token, Error>> {
        let found: Vec<(usize, &str)> = match unit {
            Unit::Word => text.unicode_word_indices().collect(),
            _ => text.split_sentence_bound_indices().collect(),
        };
        let tokens: Vec<_> = found
            .into_iter()
            .map(|(i, s)| Ok(Token { start: i as i64, end: (i + s.len()) as i64, text: s.to_string() }))
            .collect();
        stream_of(tokens.into_iter())
    }
}
```

Zig 中则是：`zig/src/<module name>.zig` 存放工厂函数，以及一个与接口方法一一对应的结构体；生成的 `mygo.zig`（库的根）会在编译时检查它并调用它：

```zig
const std = @import("std");
const mygo = @import("mygo.zig");
const Allocator = std.mem.Allocator;

pub fn makeAnalyzer() mygo.Error!mygo.Analyzer {
    const a = try mygo.allocator.create(Analyzer);
    a.* = .{};
    return mygo.Analyzer.of(a);
}

const Analyzer = struct {
    pub fn detectLanguage(_: *Analyzer, _: Allocator, _: []const u8) mygo.Error![]const u8 {
        return error.Unsupported;
    }

    pub fn tokens(_: *Analyzer, _: Allocator, _: *mygo.Context, text: []const u8, _: mygo.Unit) mygo.Error!mygo.Iterator(mygo.Token) {
        const w = try mygo.allocator.create(Words);
        w.* = .{ .it = std.mem.tokenizeScalar(u8, text, ' '), .text = text };
        return mygo.Iterator(mygo.Token).of(w);
    }

    pub fn deinit(self: *Analyzer) void {
        mygo.allocator.destroy(self);
    }
};

const Words = struct {
    it: std.mem.TokenIterator(u8, .scalar),
    text: []const u8,

    pub fn next(self: *Words, _: Allocator) mygo.Error!?mygo.Token {
        const w = self.it.next() orelse return null;
        const start: i64 = @intCast(@intFromPtr(w.ptr) - @intFromPtr(self.text.ptr));
        return .{ .start = start, .end = start + @as(i64, @intCast(w.len)), .text = w };
    }

    pub fn deinit(self: *Words) void {
        mygo.allocator.destroy(self);
    }
};
```

| Go | Rust | Zig |
|---|---|---|
| `M(a int, b string)` | `fn m(&self, a: i64, b: String)` | `pub fn m(self: *T, arena: Allocator, a: i64, b: []const u8) void` |
| `M(...) error` | `-> Result<(), Error>` | `mygo.Error!void` |
| `M(...) (T, error)` | `-> Result<T, Error>` | `mygo.Error!T` |
| `M(ctx context.Context, ...)` | `ctx: &Context` 放在最前 | 在 arena 之后的 `ctx: *mygo.Context` |
| `iter.Seq2[T, error]` | `Stream<Result<T, Error>>`，来自 `stream_of(iterator)` | `mygo.Iterator(T)`，来自 `Iterator(T).of(p)` |
| `iter.Seq[T]` | `Stream<T>` | `mygo.Iterator(T)` |
| `onChange func(float64)` | `Arc<dyn Fn(f64) + Send + Sync>` | `mygo.<Interface><Method><Param>`，带 `call` |
| `check func(string) error` | `Arc<dyn Fn(String) -> Result<(), Error> + Send + Sync>` | 相同，其 `call` 返回 `mygo.Error!void` |
| `M(...) io.Closer` | `-> Box<dyn Closer>`，来自 `closer(f)` | `mygo.Error!mygo.Closer`，来自 `Closer.of(p)` |
| 模块的一个接口 | `Arc<dyn I>` | `mygo.I`，来自 `I.of(p)` |
| Go 实现的一个接口 | `Arc<dyn I>`，其方法调用 Go | `mygo.I`，其方法调用 Go |

| Go | Rust | Zig |
|---|---|---|
| `bool`, `int`, `int64`, `uint8` … | `bool`, `i64`, `i64`, `u8` … | `bool`, `i64`, `i64`, `u8` … |
| `float32`, `float64` | `f32`, `f64` | `f32`, `f64` |
| `string`, `[]byte` | `String`, `Vec<u8>` | `[]const u8` |
| `time.Time`, `time.Duration` | `SystemTime`, `Duration` | `mygo.Time`，`i64` 纳秒 |
| `[]T`, `map[string]T` | `Vec<T>`, `HashMap<String, T>` | `[]const T`, `[]const mygo.Entry(T)` |
| `*T` | `Option<T>` | `?T` |
| 包的一个结构体、一个枚举 | 一个 `struct`、一个 `enum` | 一个带默认值的 `struct`、一个 `enum` |
| `*native.Buffer`（一个参数） | `Buffer`，带 `bytes_mut()` | `mygo.Buffer`，带 `bytes()` |

名称遵循各语言的惯例：方法和字段在 Rust 中用 snake case（`detect_language`），在 Zig 中用小驼峰（`detectLanguage`）；Go 名称若是关键字会被转义（`r#type`、`@"type"`）。

**错误。** 包的错误是一个枚举，Rust 中是 `LanguageError::Undetermined`（`?` 把它变成 `Error`），Zig 中是 `error.Undetermined`。`Error::Unsupported` 和 `error.Unsupported` 以 `errors.ErrUnsupported` 到达 Go；其他错误则作为带有 `domain`、`number` 和 `message` 的 `*native.Error` 到达，Rust 写作 `Error::Other { domain, number, message }`（或 `Error::other(message)`），Zig 写作 `mygo.fail(domain, number, message)`。

**线程。** 没有上下文的方法在调用方 goroutine 所在线程上运行，有上下文的方法在独立线程上运行：Rust 的 `Context` 和 Zig 的 `mygo.Context` 可以查询 Go 是否已取消调用（`is_canceled`、`isCanceled`），也可以休眠直到取消（`sleep`）。流同样在独立线程上运行，按 Go 循环的需要向迭代器取值。实现可能从任意线程被调用，因此 Rust 的 trait 是 `Send + Sync`，修改共享状态需要 `Mutex` 或原子操作。

**Zig 中的内存。** 每次调用都有一个 arena，用于存放参数，发送结果后释放；返回其中的切片或参数的切片时无需复制。流的 arena 和参数保留到流结束，每个产出的值另有独立的 arena。需要在调用结束后继续保留的对象、迭代器，以及用 `keep()` 保留的回调，应使用 `mygo.allocator` 分配，并在 Go 释放该值时调用的 `deinit` 中释放内存。方法若在调用结束后仍保留某个回调，就要对它调用 `keep()`，用完再调用 `release()`。

**只有 Swift 能实现的东西。** 视图和窗口属于平台，因此使用它们的模块只能用 Swift 实现，在 Android 上则用 Kotlin 实现，`mygo native add rust` 会指出它的哪些方法造成了阻碍。纹理和其他模块的接口只有 Swift 能实现，返回 `*native.Buffer`（而不是接收它）也是如此。

## 手机

iPhone 和 Android 手机随应用一起从源码构建模块：`-platform ios` 的 `mygo dev` 和 `mygo build` 用 `xcodebuild` 把模块的 Swift 构建为应用中的一个 framework，`-platform android` 则把模块的 Kotlin 加入应用的 Gradle 构建，每个模块一个库。手机从不使用已发布的构建：iOS 运行的代码随应用签名，Android 应用的库也是它自己构建的。模块没有对应目录时，在手机上就没有实现，其构造函数返回 `errors.ErrUnsupported`，和在其他缺少实现的平台上一样。

### iOS 上的 Swift

模块的 `Package.swift` 在平台中列出 iOS 及其支持的最低版本时，它的 Swift 在 iOS 上运行；列出 macOS 或不列出任何平台时，在 macOS 上运行：

```swift
platforms: [.macOS(.v13), .iOS(.v16)],
```

同一份 Swift 同时服务两者，二者不同之处用 `#if os(iOS)` 区分，生成的代码也是如此：iOS 上视图是 `UIView`（`NativeView` 表示 `NSView` 或 `UIView`），窗口以显示它的 `UIViewController` 传入，可以从它弹出界面。只用于 iOS 的模块只列出 `.iOS`，macOS 上就用它的 Rust 或 Zig，SQLite 插件就是这样。Swift 包、二进制 framework（`.binaryTarget`）和资源的用法与 Xcode 中相同，因为构建它们的就是 Xcode；iOS 需要 Xcode，仅有 Command Line Tools 不够。

### Android 上的 Kotlin

`mygo native add kotlin` 创建 `kotlin/`：一个 Android 库，其 `build.gradle.kts` 写明命名空间和依赖，Kotlin 代码位于模块导入路径对应的包中，像 Java 包名那样把域名倒过来：`example.com/app/modules/language` 对应 `com.example.app.modules.language`。应用的构建设置它的 SDK 级别、Java 和 Kotlin，以及它对 MyGo 宿主库 `dev.mygo` 的依赖，协程也由这个库带来。实现由每个入口点一个函数和实现各接口的类组成：

```kotlin
package com.example.app.modules.language

import android.icu.text.BreakIterator
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow

fun makeAnalyzer(): Analyzer = ICUAnalyzer()

private class ICUAnalyzer : Analyzer {
    override fun detectLanguage(text: String): String = throw LanguageException(LanguageError.UNDETERMINED)

    override fun tokens(text: String, unit: Unit): Flow<Token> = flow {
        val it = if (unit == Unit.WORD) BreakIterator.getWordInstance() else BreakIterator.getSentenceInstance()
        it.setText(text)
        var start = it.first()
        var end = it.next()
        while (end != BreakIterator.DONE) {
            val s = text.substring(start, end)
            val at = text.substring(0, start).toByteArray().size.toLong()
            emit(Token(at, at + s.toByteArray().size, s))
            start = end
            end = it.next()
        }
    }
}
```

| Go | Kotlin |
|---|---|
| `M(a int, b string)` | `fun m(a: Long, b: String)` |
| `M(...) error`、`M(...) (T, error)` | `fun m(...)`、`fun m(...): T`，出错时抛出异常 |
| `M(ctx context.Context, ...)` | `suspend fun m(...)`，随 Go 的 context 取消 |
| `iter.Seq2[T, error]`、`iter.Seq[T]` | `Flow<T>`，由 Go 的循环拉取 |
| `onChange func(float64)` | `(Double) -> Unit` |
| `M(...) io.Closer` | `AutoCloseable`，可用 `NativeCloser { }` 创建 |
| 模块的接口 | 该接口，由一个类实现 |
| Go 实现的接口 | 该接口，其方法调用 Go |
| `bool`、`int`、`int32`、`uint16`、`uint64`…… | `Boolean`、`Long`、`Int`、`UShort`、`ULong`…… |
| `float32`、`float64` | `Float`、`Double` |
| `string`、`[]byte` | `String`、`ByteArray` |
| `time.Time`、`time.Duration` | `java.time.Instant`、`kotlin.time.Duration` |
| `[]T`、`map[string]T`、`*T` | `List<T>`、`Map<String, T>`、`T?` |
| 包中的结构体、枚举 | `data class`、`enum class` |
| `*native.Buffer`（作为参数） | `NativeBuffer`，其 `bytes` 是直接 `ByteBuffer` |
| `native.View`、`*mygo.Window` | `android.view.View`，即窗口的视图 |
| `ui.Rect` | `android.graphics.RectF`，以视图的像素为单位 |

包的错误写作 `<Name>Exception(<Name>Error.X)`；`NativeUnsupportedException` 以 `errors.ErrUnsupported` 到达 Go，其他异常则作为 `*native.Error` 到达，用 `NativeException(domain, number, message)` 可以设置它的 domain 和 number。方法在调用方 goroutine 的线程上运行，该线程已附加到 JVM；`suspend` 方法和流在各自的协程上运行；`//mygo:mainthread` 方法、视图及其回调在 UI 线程上运行。`dev.mygo.Native.context` 是应用的 `Context`，`Native.activity` 是当前 activity，可以从它启动其他 activity。视图是模块用 `Native.context` 创建的 `android.view.View`。

除了纹理、返回 `*native.Buffer` 和其他模块的接口，Swift 能实现的 Kotlin 都能实现，`mygo native add kotlin` 会指出不能实现的部分。

**Android 上的 C。** Kotlin 模块可以带上 C 或 C++，由它的 `build.gradle.kts` 用 CMake（`externalNativeBuild`）构建；应用的构建会给它 NDK 和 Go 所构建的 ABI，Kotlin 用 `System.loadLibrary` 加载它。SQLite 插件的模块 `plugins/sqlite/internal/sqlitecore` 就是这样：它的 Kotlin 调用 SQLite 的 amalgamation，由 CMake 下载并用 SHA-256 固定版本；它的 Swift 则以二进制 framework 链接同一个 SQLite。

## 对象

方法也可以返回包里的另一个接口，这种接口不需要指令：实现返回任何符合该接口的对象，Go 就得到一个可通过它调用该对象的接口值。它适用于有生命周期的东西：已加载的模型、会话、连接。

```go
//mygo:native
type Analyzer interface {
	// ...
	// Embedding loads the word embedding of a language.
	Embedding(ctx context.Context, language string) (Embedding, error)
}

// Embedding places words of a language by meaning.
type Embedding interface {
	io.Closer
	Distance(a, b string) float64
	Neighbors(word string, n int) []string
}
```

```swift
extension NLAnalyzer {
    func embedding(language: String) async throws -> any Embedding {
        guard let e = NLEmbedding.wordEmbedding(for: NLLanguage(language)) else {
            throw LanguageError.noEmbedding
        }
        return WordEmbedding(e)
    }
}

final class WordEmbedding: Embedding {
    let e: NLEmbedding
    init(_ e: NLEmbedding) { self.e = e }
    func distance(a: String, b: String) -> Double { e.distance(between: a, and: b) }
    func neighbors(word: String, n: Int) -> [String] { e.neighbors(for: word, maximumCount: n).map(\.0) }
    func close() throws {}
}
```

```go
e, err := a.Embedding(ctx, "en")
if errors.Is(err, language.ErrNoEmbedding) {
	// ...
}
defer e.Close()
fmt.Println(e.Neighbors("coffee", 5))
```

实现是类：生成的协议是 `AnyObject`。Go 值持有对 Swift 对象的强引用。嵌入 `io.Closer` 或有 `Close() error` 方法的接口，其对象会被显式关闭：Go 调用对象的 `close()`，然后释放它，之后每次调用都返回 `native.ErrClosed`。在其他情况下，以及作为从未关闭对象的安全兜底，对象会在 Go 值变得不可达之后被释放。

模块返回的对象可以被传回给它，或传给另一个模块：实现收到同一个实例，而不是副本。

### 订阅

如果方法启动了持续存在的东西，比如观察某个值或监听系统事件，它就返回一个用于停止它的 `io.Closer`：

```go
type Analyzer interface {
	// ...
	// OnLanguagesChange calls f when the user changes the preferred
	// languages, until the subscription is closed.
	OnLanguagesChange(f func(languages []string)) io.Closer
}
```

在 Swift 中它返回一个 `Closer`，由 `NativeCloser` 从一个只运行一次的闭包创建：

```swift
func onLanguagesChange(f: @escaping @Sendable ([String]) -> Void) -> any Closer {
    let token = NotificationCenter.default.addObserver(
        forName: NSLocale.currentLocaleDidChangeNotification, object: nil, queue: nil
    ) { _ in f(Locale.preferredLanguages) }
    return NativeCloser { NotificationCenter.default.removeObserver(token) }
}
```

```go
sub := a.OnLanguagesChange(func(langs []string) { ... })
defer sub.Close()
```

关闭时会运行一次这个关闭函数并释放回调；此后实现再调用它也不会有任何效果。

## Go 实现的接口

模块的方法只接收、从不返回的接口由 Go 实现：平台会向它提问，如同 AppKit 向委托或数据源提问。传入任何实现它的 Go 值即可。

```go
// Ranker decides the order of suggestions.
type Ranker interface {
	// Score is higher for better suggestions.
	Score(word string) float64
	// Explain describes a suggestion, which may take a request.
	Explain(ctx context.Context, word string) (string, error)
}

type Analyzer interface {
	// ...
	// Suggest yields the neighbors of word in the embedding, best first by
	// the ranker's scores.
	Suggest(ctx context.Context, word string, r Ranker) iter.Seq2[string, error]
}
```

它同样会变成一个协议，Swift 收到一个符合该协议、会回调 Go 的对象：

```swift
func suggest(word: String, r: any Ranker) -> AsyncThrowingStream<String, Error> {
    AsyncThrowingStream { continuation in
        let task = Task {
            let best = candidates(of: word).sorted { r.score(word: $0) > r.score(word: $1) }
            for w in best { continuation.yield(w) }
            let why = try await r.explain(word: best[0]) // Go's method with a context
            // ...
            continuation.finish()
        }
        continuation.onTermination = { _ in task.cancel() }
    }
}
```

它的方法遵循[有结果的回调](#回调)那一节的规则：没有上下文的方法在调用线程上立即运行并返回结果；有上下文的方法在 Swift 中是 `async throws`，在自己的 goroutine 上运行，取消 Swift 任务会取消其上下文。Go 错误会以 `NativeError` 抛出；如果它属于包声明的错误，则以模块的错误枚举抛出。

Swift 可以保留这个对象；只要 Swift 还保留着，Go 就让该值保持存活，直到 Swift 释放对象。

## 视图

嵌入 `native.View` 的接口由平台原生视图实现。将它作为 `native.View` 传给 `ui.NativeView` 即可显示，类似于 `ui.WebView` 显示 webview：在构建视图的函数之外创建一次，再在该函数中显示。

```go
// Player plays a video.
//
//mygo:native
type Player interface {
	native.View
	Load(url string)
	Play()
	Pause()
	Seek(ctx context.Context, to time.Duration) error
	OnTime(f func(time.Duration)) io.Closer
}
```

在 Swift 中，视图是一个符合该协议的 `NSView` 子类；生成的代码把该协议声明为 `@MainActor`，并约束为 `NSView`：

```swift
import AVKit

@MainActor func makePlayer() -> any Player { PlayerView() }

final class PlayerView: AVPlayerView, Player {
    func load(url: String) { player = URL(string: url).map(AVPlayer.init(url:)) }
    func play() { player?.play() }
    func pause() { player?.pause() }
    func seek(to: Duration) async throws { await player?.seek(to: CMTime(to)) }
    func onTime(f: @escaping @Sendable (Duration) -> Void) -> any Closer {
        let token = player?.addPeriodicTimeObserver(forInterval: CMTime(seconds: 0.25, preferredTimescale: 600), queue: .main) {
            f(Duration($0))
        }
        return NativeCloser { [weak self] in token.map { self?.player?.removeTimeObserver($0) } }
    }
}
```

```go
type app struct {
	win     *mygo.Window
	player  media.Player
	timeSub io.Closer
	now     time.Duration
}

func (a *app) view(c *ui.Context) {
	// What the view paints after the native view, its children first,
	// shows over it.
	ui.NativeView(c, a.player).Fill().Children(func() {
		ui.Text(c, captionAt(a.now)).Absolute().Left(24).Bottom(24)
	})
}

// After making the player:
a.timeSub = a.player.OnTime(func(t time.Duration) { a.now = t; a.win.Invalidate() })
```

原生视图像 webview 一样，是原生 UI 中的一个盒子：它没有自己的尺寸，从布局取得尺寸，并被裁剪到元素的 `Radius`。视图之后绘制的内容会显示在它之上，因此字幕、控件、浮层和对话框都绘制在视频上方。MyGo 把 `NSView` 放在原生 UI 的帧之下，设置其尺寸并裁剪：不要自己移动、调整大小或移除它。它接收鼠标，在获得焦点时作为第一响应者接收键盘，并像任何 `NSView` 一样加入窗口的辅助功能树。如果窗口的视图没有构建某个原生视图，它就会隐藏，但保留自己的状态。

原生视图按 AppKit 的要求在主线程上运行，无需 `//mygo:mainthread`。应在应用启动后，即 `App.WhenReady` 中或之后创建；关闭和释放也在主线程上进行。它的回调同样在主线程上运行；在回调中调用窗口的 `Invalidate` 来显示变化。

### 窗口和锚点

系统显示的共享菜单、Quick Look 面板、浮层或附着式对话框（sheet），需要指定所属窗口以及窗口内的锚点位置。为此可以接收一个 `*mygo.Window` 和窗口内的一个 `ui.Rect`，例如某个元素的边界：

```go
//mygo:native
//mygo:mainthread
type Sharing interface {
	// Share shows the system's share menu for the files, from rect of win.
	Share(win *mygo.Window, rect ui.Rect, paths []string)
}
```

Swift 收到 `NSWindow`，矩形则是其内容视图坐标系中的 `CGRect`，从原生 UI 的左上原点翻转过来，可直接用于 AppKit 的 `relativeTo:of:` 参数：

```swift
@MainActor final class SystemSharing: Sharing {
    func share(win: NSWindow, rect: CGRect, paths: [String]) {
        let picker = NSSharingServicePicker(items: paths.map(URL.init(fileURLWithPath:)))
        picker.show(relativeTo: rect, of: win.contentView!, preferredEdge: .minY)
    }
}
```

```go
btn := ui.Button(c, "Share")
if btn.Clicked() {
	a.sharing.Share(a.win, btn.Bounds(), []string{path})
}
```

`native.View` 参数（例如另一个模块的视图）在 Swift 中对应原来的 `NSView`，也可以用它作为锚点。

## 使用其他模块的模块

模块可以使用另一个原生模块的类型和接口：导入它的 Go 包，并像任何 Go 类型一样在接口中使用它们。

```go
package player

import "example.com/app/modules/media"

//mygo:native
type Player interface {
	native.View
	// Load plays an asset that the media module opened.
	Load(asset media.Asset)
}
```

生成的 Swift 会导入另一个模块的 Swift 模块，实现收到的就是该模块返回的对象——它自己类中的 `AVAsset`，而不是副本。

Go 的导入关系也决定了原生模块之间的依赖，无需再修改 `Package.swift`。生成的声明为 `public`，可供其他模块访问；Go 包加载时会先加载所依赖的模块。`mygo dev`、`mygo build` 和 `mygo native check` 先从各自的 Swift 包构建依赖模块，再用这些 Swift 模块和库编译、链接当前模块。无论被多少模块引用，每个模块在应用中都只有一份，统一放在 `Contents/Frameworks` 中。

`mygo dev` 还会在 `.mygo/native` 中生成统一的 Swift 包，将应用的每个模块作为一个 target；各 target 依赖其 Go 包导入的模块及自身 `Package.swift` 声明的包，源文件则链接到原模块目录。在 Xcode 中打开这个包，或让 SourceKit-LSP 指向它，就能获得跨模块的代码补全。它的构建方式与应用一致，因此两个模块若要求同一 Swift 包的不兼容版本，SwiftPM 会在解析依赖时报告冲突并停止。

## 大数据

值跨边界时是复制的，这适合参数和结果，却不适合视频帧或音频采样的流。有两种类型可以不复制就跨边界。

### 缓冲区

`*native.Buffer` 是 Go 和实现共同使用的内存：`native.NewBuffer(n)` 创建一个，`Bytes()` 是它在 Go 中的内容。传入它让实现来填充，就像 `io.Reader` 填充切片；也可以从流中产出缓冲区：

```go
type Recorder interface {
	// Samples yields the microphone's PCM samples, 16-bit, a buffer at a
	// time. A buffer is valid until the loop asks for the next one.
	Samples(ctx context.Context) iter.Seq2[*native.Buffer, error]
	// Read fills buf with what was recorded since the last read.
	Read(buf *native.Buffer) (int, error)
}
```

在 Swift 中它是 `NativeBuffer`，其 `bytes` 是指向它的内存的 `UnsafeMutableRawBufferPointer`，在缓冲区存在期间有效：

```swift
func read(buf: NativeBuffer) throws -> Int {
    ring.read(into: buf.bytes)
}
```

流产出的缓冲区在循环开始下一次迭代前有效；需要保留的内容，应像使用 `bufio.Scanner.Bytes` 时一样复制。Swift 从 `NativeBufferPool` 取出缓冲区，Go 的循环继续后即可复用，因此流开始运行后不再分配新的缓冲区。

### 纹理

`*native.Texture` 是 GPU 上的一张图像，实现向其中绘制，原生 UI 显示它，全程不经过 CPU 复制：它是 `ui.ImageSource`，如同 `*ui.Bitmap` 一样。

```go
type Frames interface {
	// Attach draws the video's frames into tex as they play.
	Attach(tex *native.Texture) io.Closer
}
```

在 Swift 中它是 `NativeTexture`，通过 `present` 接收帧：一个 `IOSurface`，或由它支撑的 `CVPixelBuffer`。`pixelBufferAttributes` 以原生 UI 无需转换即可绘制的像素格式创建这样的缓冲区，`displayLink` 在窗口显示器的每次刷新时回调：

```swift
func attach(tex: NativeTexture) -> any Closer {
    let output = AVPlayerItemVideoOutput(pixelBufferAttributes: tex.pixelBufferAttributes)
    item.add(output)
    let link = tex.displayLink { time in
        if let pb = output.copyPixelBuffer(forItemTime: output.itemTime(forHostTime: time), itemTimeForDisplay: nil) {
            tex.present(pb)
        }
    }
    return NativeCloser { link.invalidate(); self.item.remove(output) }
}
```

```go
tex := native.NewTexture()
sub := a.frames.Attach(tex)

func (a *app) view(c *ui.Context) {
	ui.Image(c, tex).Fill().Radius(12)
}
```

Swift 提交的内容会在窗口的下一帧显示，应用无需调用 `Invalidate`。原生 UI 会保留当前显示的绘制表面，并将 GPU 尚可能读取的表面标记为使用中（`IOSurfaceIncrementUseCount`）。直到没有任何帧使用某个表面，像素缓冲区池才会再次将它交给实现；因此每一帧都应从池中取缓冲区绘制，不要反复写入同一个缓冲区。`displayLink` 的回调会收到该帧显示时的主机时间，单位为秒，与 `CACurrentMediaTime` 使用同一时间基准；`invalidate()` 会停止回调。与[视图](#视图)不同，纹理由原生 UI 自己绘制：它可以像其他图像一样被覆盖、裁剪、缩放、旋转和淡出，也可以由[着色器](ui/shaders.md)绘制。

## 依赖

`swift/Package.swift` 是模块的 Swift 包，由你编辑：向其 `dependencies` 添加 Swift 包和二进制 target，用 `linkerSettings` 链接系统框架，并在 `platforms` 中列出它运行的系统。

```swift
// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "Language",
    platforms: [.macOS(.v12)],
    products: [.library(name: "Language", type: .dynamic, targets: ["Language"])],
    dependencies: [
        .package(url: "https://github.com/apple/swift-collections", from: "1.1.0"),
    ],
    targets: [
        .target(name: "Language", dependencies: [
            .product(name: "Collections", package: "swift-collections"),
        ]),
    ]
)
```

保持 `<Module>` 产品为动态库、以模块命名，并把生成的文件列入其源文件。`mygo native new` 写出的包以 Swift 5 语言模式编译（`swiftLanguageModes: [.v5]`），这样 Swift 6 严格的并发检查就不会妨碍实现——Go 会从自己的线程调用这些实现的类。包引入的动态框架（例如 XCFramework）会随它一起进入应用的 `Contents/Frameworks`，`mygo build` 用应用的身份为它们全部签名。

`rust/Cargo.toml` 属于 crate，其 `[dependencies]` 和任何 crate 一样用来添加依赖；保持 `crate-type = ["cdylib"]`。`zig/build.zig` 从 `src/mygo.zig` 构建库，`build.zig.zon` 用来获取 Zig 包，或获取供 `build.zig` 编译的 C 库源码，正如 [SQLite 插件](plugins/sqlite.md)所做的那样。

## Info.plist 和权限

有些框架需要在应用的 `Info.plist` 中提供用途说明，例如 Speech、麦克风和相机，或者需要权限（entitlement），例如共享钥匙串或 App Groups。把它们放在模块的 `swift/Info.plist` 和 `swift/Entitlements.plist` 中，`mygo build` 和 `mygo dev` 会在 macOS 上把它们合并到应用的对应文件中：

```xml
<!-- swift/Info.plist -->
<plist version="1.0">
<dict>
    <key>NSSpeechRecognitionUsageDescription</key>
    <string>Recognize speech in your recordings.</string>
</dict>
</plist>
```

字典按键合并，数组按值合并，应用自己的 `macos.infoPlist` 和 `macos.entitlements`（[配置](configuration.md)）最后合并、优先级最高，因此应用可以自己为模块的用途说明措辞。如果两个模块给同一个键设了不同的值，构建会停止并列出这两个模块，除非应用自己设置了这个键。像 App Groups 这样需要描述文件允许的权限，就得有一个允许它的描述文件（[描述文件](distribution.md#描述文件)）。

## 构建、运行和测试

构建模块需要其语言的工具：`swift/` 需要 Xcode 或其 Command Line Tools，以及 Swift 6（iOS 需要 Xcode）；`kotlin/` 需要 Android 应用本来就需要的 Android SDK 和 Gradle 所用的 JDK（[手机和平板](mobile/README.md#你需要什么)），带 C 的模块还需要 SDK Manager 中的 CMake；`rust/` 需要 cargo（[rustup](https://rustup.rs)）；`zig/` 需要 [Zig](https://ziglang.org) 0.16。`mygo doctor` 会告诉你装了哪些。

接口变化后，`mygo dev` 会重新生成模块；实现文件变化后，会重新构建当前机器的实现并重启应用，窗口会在原位置重新打开。`mygo build` 则以发布模式为目标平台构建每个模块：Swift 生成兼容 arm64 和 x86_64 的通用二进制，连同依赖框架一起签名，再进行应用的签名和公证（[分发](distribution.md)）；Rust 和 Zig 生成各架构的库，放入应用资源。cargo 和 Zig 的构建文件位于 Go 模块的 `.mygo/native`，而不是原生模块目录；应将 `.mygo/` 加入 `.gitignore`，`mygo init` 创建的项目已这样配置。

先用 `mygo dev` 或 `mygo native build -target host` 构建实现，之后 `go test` 和 `go run` 就能运行当前平台的实现；如果构建产物缺失或比源码旧，会提示先执行构建。用 `-lang` 可以改为构建模块的另一种语言实现，以便逐一测试：

```sh
mygo native build -target host -lang rust ./modules/language && go test ./modules/language
mygo native build -target host -lang zig ./modules/language && go test ./modules/language
```

在手机上，模块在应用中运行：`mygo dev -platform ios` 和 `-platform android` 随应用构建它，并在模拟器或设备上运行，它的日志和 Go 的日志显示在一起。`go test` 不在手机上运行，所以通过接口的假实现测试使用模块的 Go 代码（见下文），在应用中测试实现。

在 Linux 上，同样的命令可以用 [Swift 工具链](https://www.swift.org/install/linux/) 构建仅依赖 Foundation 的 Swift 模块。GitHub 的 Ubuntu runner 镜像自带该工具链，因此这些模块也能在 Linux 任务中测试；应用本身只在 macOS 上运行 Swift 实现。依赖接口而不是构造函数返回值的代码，可以在任何平台上用假实现测试：

```go
type fakeAnalyzer struct{ lang string }

func (f fakeAnalyzer) DetectLanguage(text string) (string, error) { return f.lang, nil }

func (f fakeAnalyzer) Tokens(ctx context.Context, text string, unit language.Unit) iter.Seq2[language.Token, error] {
	return func(yield func(language.Token, error) bool) {
		yield(language.Token{Start: 0, End: len(text), Text: text}, nil)
	}
}
```

要在实现中停在断点处，把调试器附加到 `mygo dev` 运行的应用上：Xcode 的（Debug ▸ Attach to Process），或 `lldb -p <pid>` 和 `gdb -p <pid>`。它用 `Logger`、`print`、`eprintln!` 或 `std.debug.print` 记录的内容会显示在 `mygo dev` 的输出中，紧挨着 Go 的输出。

## 发布模块

如果模块属于应用自己，或位于应用用目录替换的 Go 模块中，应用就会从源码构建它们，手机则总是如此。从模块缓存中使用你的模块的应用，以及没有该语言工具的机器，例如构建 macOS 应用的 GitHub Actions Linux runner，则使用已发布的构建。`mygo native build` 生成这些构建：

```sh
mygo native build -upload [module dir]
```

为 macOS、Linux 和 Windows 的 amd64 与 arm64 构建实现（每个平台用对应的语言），把文件作为 GitHub 发布版本的资产上传，并把它们记录在模块旁边的 `mygo-plugin.json` 中：名称、URL、SHA-256，以及每个构建所依据源码的哈希。Swift 发布为单个通用文件 `darwin-universal`，即库及其带入的框架的 `.tar.gz`。提交 `mygo-plugin.json`：之后每台机器都会下载这些构建而不是编译它们，只要它们与源码匹配。

命令通过 [GitHub CLI](https://cli.github.com)，将文件上传到当前模块所属 git 仓库中专用的 `native-<name>` 发布版本（例如 `native-adder`）；该发布版本只创建一次，不会设为最新版本。也可以用 `-release` 指定上传地址。模块的历次构建都存放在同一位置，文件以各自的 SHA-256 命名且不被替换，因此旧提交记录的构建仍可下载。不带 `-upload` 时，命令将文件写入 Go 模块的 `.mygo/native/release`，由你自行发布到 `-release` 指向的位置。

源码哈希覆盖该语言目录中的文件，换行符按 LF 计算，使各平台的检出保持一致；对于 Swift，还覆盖该模块使用的原生模块的文件。如果某个平台记录的构建与源码匹配，就跳过该平台，因此每次推送都发布时只会上传发生变化的部分；`-force` 会强制重新构建这些平台。发布应用使用的每个模块：模块的 Swift 归档包含它自己的库，而不是它使用的模块的库。

当已发布构建的哈希与源码一致时，`mygo build` 会采用它；缺少该语言工具时，`mygo dev` 也会采用。源码一旦变化，有工具的机器会构建它们并给出警告，没有工具的机器则停止并提示重新发布。

构建失败时，命令会停止，不发布任何文件。如果某个目标必须在其他系统上构建，例如在非 Mac 系统上构建 macOS 的 Swift 或 Rust 库，命令会跳过该目标，在末尾列出，并保留 `mygo-plugin.json` 中原有的记录。这样就能分别在合适的系统上完成各平台构建：

- **Swift** 在 Mac 上构建。
- **Zig** 交叉编译到每个平台，包括 macOS。
- **Rust** 需要每个 target（`rustup target add aarch64-unknown-linux-gnu`……）以及对应的链接器：安装 [cargo-zigbuild](https://github.com/rust-cross/cargo-zigbuild) 后，CLI 用 Zig 链接 Linux 和 Windows 库。macOS 库需要 Apple 的 SDK：在 Mac 上构建它们。

```sh
mygo native build -target darwin-arm64,darwin-amd64 -upload ./modules/language   # on a Mac
mygo native build -target linux-amd64,linux-arm64,windows-amd64,windows-arm64 -upload ./modules/language
```

Linux 库针对 glibc 2.28 链接（由 Zig 链接，装了 cargo-zigbuild 时也由它链接），因此能在 Debian 10、Ubuntu 20.04 及更高版本上加载。`mygo dev` 和 `mygo build` 构建的 macOS 库支持应用的 `macOS.minimumSystemVersion`，发布版本的库则支持 macOS 12 及更高版本。CLI 会移除它构建的 Linux 库中的调试信息。

| 标志 | |
|---|---|
| `-target` | 平台：用逗号分隔的 `GOOS-GOARCH`，或 `all`（默认）、`host` |
| `-lang` | 对实现了该语言的模块，用该语言构建，而不是按各平台默认的语言 |
| `-release` | 发布文件所在目录的 URL（默认：库的 `release`） |
| `-out` | 要发布的文件所在目录（默认：模块的 `.mygo/native/release`） |
| `-upload` | 把文件上传到 `-release` 对应的 GitHub 发布版本 |
| `-force` | 即使某个平台已发布的文件就是由当前源码构建的，也重新构建 |

## 检查和调用模块

有两个命令无需打开应用就能检查模块，供你和 agent 使用：

```sh
mygo native check [-lang <language>] [module dir]
```

生成模块，为这台机器（或用 `-lang` 指定的语言）构建其实现并 vet Go 包，然后把每个问题列为 `file:line: message`，并说明要改什么，例如 `language.go:14: Tokens returns chan Token, which an implementation cannot return: return iter.Seq2[Token, error]`。有问题时以状态码 1 退出。

```sh
mygo native call [-timeout <duration>] [module dir] <Interface>.<Method> [<arguments>]
```

创建接口（一个入口点）的实现，用 `<arguments>`（按方法参数的 Go 名称组成的 JSON 对象）调用它的某个方法，并把结果打印为 JSON，流中的每个值一行：

```sh
mygo native call modules/language Analyzer.DetectLanguage '{"text": "Bonjour"}'
```

它不接受 `ctx` 参数：它会自行创建上下文，并由 `-timeout` 限制时长。回调会打印各自的调用。

`mygo native` 的每个命令都在 [CLI 参考](cli.md#mygo-native)中。`mygo native new` 写入模块的 `AGENTS.md` 说明了这些内容；`mygo install-skills` 会安装一个教 agent 编写原生模块的 skill。

## 从网页中使用它

原生模块就是一个 Go 值，因此可以把它绑定，通过[生成的客户端](bindings.md)把它的方法也提供给网页：

```go
a, _ := language.NewAnalyzer()
mygo.Bind(&Text{a}) // a service whose methods call a
```

## 工作原理

生成的 Go 代码通过 `ffi` 查找、加载实现所在的库，并用 purego 调用，不使用 cgo，与 MyGo 调用系统 API 的方式相同。每个方法都有一个 C 入口点，Swift 用 `@_cdecl` 导出，Rust 用 `#[no_mangle] extern "C"`，Zig 用 `export`（在 Android 上则是生成的 Kotlin 的 `invoke`，由宿主的 JNI 调用）；所有模块共享一个 Go 回调，传递异步结果、流中的值、回调调用，以及对 Go 实现接口的调用。三种语言使用相同的调用接口，Go 无需区分实现语言。加载库时，Go 将所用 C 运行时的 `malloc` 和 `free` 传给库，双方用同一组函数分配和释放跨边界传递的数据。

值以二进制编码跨越边界，双方的生成代码按声明的顺序写入和读取，不使用反射，也没有字段名：整数为 8 字节，浮点为 float64，字符串和 `[]byte` 为长度后跟字节，切片和 map 为长度后跟元素，指针为一个标志后跟值，结构体为其字段，枚举为其值，时间为 Unix 秒和纳秒。结果直接从实现返回它们的那块内存中就地读取。只使用数字和 bool、且不返回错误的方法连这一步都省了：Go 直接把它们作为寄存器参数调用对应的 C 函数。缓冲区是双方映射的内存，纹理是与 GPU 共享的 `IOSurface`。没有任何原生模块的应用不会链接这其中的任何东西。

## 自己绑定 C 库

模块生成的代码是调用 Rust、Zig 或 Swift 的最简单方式。如果改为直接通过库自己的 C 函数调用——例如现成的 C 库，或 Zig 包从 C 源码构建的库——可以用包 `ffi` 来绑定：CLI 从源码构建库，把它放进应用，并像发布模块的库那样发布它。[`rust-native` 示例](../examples/rust-native)是一个以此方式调用 Rust crate 的原生 UI 窗口，[终端](plugins/terminal.md)插件绑定 libghostty-vt，一个由 Zig 构建的 C 库。

把库的源码放在绑定它的 Go 包旁边，形式可以是 Rust 的 `cdylib` crate、`build.zig` 会安装动态库的 Zig 包，或者在 macOS 上是一个 Swift 包——其产品与库同名，是动态库，函数由 `@_cdecl` 导出：

```
adder/
├── adder.go
├── mygo-plugin.json
└── rust/
    ├── Cargo.toml          # [lib] crate-type = ["cdylib"]
    └── src/lib.rs
```

```rust
// adder/rust/src/lib.rs
#[no_mangle]
pub extern "C" fn add(a: i32, b: i32) -> i32 {
    a + b
}
```

`mygo-plugin.json` 说明库以及如何构建它：

```json
{
  "libraries": [
    { "name": "adder", "build": { "cargo": "./rust" } }
  ]
}
```

Go 包嵌入它并绑定这些函数：

```go
package adder

import (
	_ "embed"

	"github.com/egoist/mygo/ffi"
)

//go:embed mygo-plugin.json
var manifest []byte

var lib = ffi.New("adder", manifest)

var fns struct {
	Add func(a, b int32) int32
}

func Add(a, b int32) (int32, error) {
	if err := lib.Bind(&fns); err != nil {
		return 0, err
	}
	return fns.Add(a, b), nil
}
```

`mygo dev` 和 `mygo build` 像构建模块一样构建它，`mygo native build` 为 `go test`（`-target host`）构建它并发布它，包中的一行 `go:generate` 会为测试构建它：

```go
//go:generate go tool mygo native build -target host .
```

而 `{"zig": "./zig"}` 则指定一个 Zig 包，CLI 用 `zig build -Dtarget=<target> -Doptimize=ReleaseFast` 构建其库；`{"swift": "./swift"}` 指定一个 Swift 包，它在 macOS 上构建为通用二进制，并进入应用的 `Frameworks`。

### 绑定函数

`Library.Bind` 接收一个指向结构体的指针，首次调用时绑定其中的函数字段；之后用同一个指针从任意 goroutine 调用，都会直接返回首次绑定的结果。因此像上例那样在每次使用前调用，既安全也没有重复绑定的开销。未知的 `tag` 选项或缺少对应函数等问题会使 `Bind` 返回错误。各字段绑定到 `tag` 指定的库函数；没有 `tag` 时，则将字段名转为 snake case 作为函数名（`CountVowels` 调用 `count_vowels`）：

```go
var fns struct {
	Add         func(a, b int32) int32
	CountVowels func(s string) uint32
	Version     func() string `ffi:"adder_version"`
	Extra       func()        `ffi:"adder_extra,optional"` // nil when the library lacks it
}
```

`BindFunc` 绑定一个函数，`Symbol` 返回任何符号的地址，用于 `purego.SyscallN` 或指向数据的指针。

函数通过 [purego](https://github.com/ebitengine/purego) 在调用方的 goroutine 上调用，传递方式与 C 一致：

| Go | C | Rust |
|---|---|---|
| `int8` … `int64`, `uint8` … `uint64`, `int`, `uintptr` | 相同大小的整数 | `i8` … `i64`, `u8` … `u64`, `isize`, `usize` |
| `float32`, `float64` | `float`, `double` | `f32`, `f64` |
| `bool` | `bool` | `bool` |
| `string` 参数 | `const char *`，在调用期间有效 | `*const c_char` |
| `string` 结果 | `const char *`，被复制 | `*const c_char` |
| `unsafe.Pointer`, `*T` | 指针 | `*mut T`, `*const T` |
| `[]byte` | 指向第一个字节的指针，在调用期间有效 | `*const u8`（也要传长度） |
| 上述类型的结构体 | 结构体，其填充必须由 Go 结构体明确写出 | `#[repr(C)]` 结构体 |

和 cgo 一样，库不得在调用结束后保留 Go 的内存。库返回的字符串会被复制到 Go 中，但不会被释放，因此应返回静态字符串，或返回一个指针（`unsafe.Pointer`）并导出释放库所分配内容的函数：

```rust
#[no_mangle]
pub extern "C" fn greeting(name: *const c_char) -> *mut c_char {
    let name = unsafe { CStr::from_ptr(name) }.to_string_lossy();
    CString::new(format!("Hello, {name}")).unwrap().into_raw()
}

#[no_mangle]
pub extern "C" fn free_string(s: *mut c_char) {
    if !s.is_null() {
        drop(unsafe { CString::from_raw(s) });
    }
}
```

```go
var fns struct {
	Greeting   func(name string) unsafe.Pointer
	FreeString func(s unsafe.Pointer)
}

func Greeting(name string) string {
	p := fns.Greeting(name)
	defer fns.FreeString(p)
	return ffi.GoString(p) // copies it
}
```

库只加载一次并保持加载。`ffi` 在 macOS、Linux 和 Windows 的 amd64 与 arm64 上工作；在其他地方，例如浏览器中，`Load` 返回一个包装 `errors.ErrUnsupported` 的错误。

### 库的查找位置

`Library.Load` 和模块的构造函数按以下顺序查找库，加载第一个找到的文件：

1. `$MYGO_NATIVE_<NAME>` 指定的文件（`MYGO_NATIVE_ADDER`）；
2. 应用资源（或 `Frameworks`）中的那个，也就是 `mygo dev` 和 `mygo build` 放置它的位置，在 macOS 上会与应用一起签名；
3. 可执行文件旁边的那个；

并且，对于不是打包应用的程序：

4. CLI 构建的那个，位于工作目录所在模块的 `.mygo/native/<goos>-<goarch>`；
5. 已发布的那个，下载一次到用户的缓存中，并根据其 SHA-256 校验。

打包应用从不下载库：`mygo build` 的生产构建不包含下载器，也就不包含 HTTP。

## mygo-plugin.json

清单放在绑定库的 Go 包或原生模块旁边，列出其中的库及其已发布的构建。CLI 会在应用导入的每个包里查找它。一个正在发布的库看起来像这样：

```json
{
  "libraries": [
    {
      "name": "adder",
      "build": { "cargo": "./rust", "args": ["--features", "simd"], "macOS": "13.0" },
      "release": "https://github.com/me/adder/releases/download/native-adder",
      "files": {
        "linux-amd64": {
          "name": "libadder.so",
          "url": "https://github.com/me/adder/releases/download/native-adder/adder-linux-amd64-1a2b3c4d5e6f.so",
          "sha256": "1a2b3c4d5e6f…",
          "sourceHash": "9f8e7d6c5b4a…"
        }
      }
    }
  ]
}
```

你写 `name` 和 `build`；`mygo native build` 写 `release` 和 `files`。原生模块的库以模块命名（`Language`），没有 `build`：它的语言目录就是它的构建方式，`mygo native build` 自己写入它的条目。

**name** 在各处标识库：`ffi.New("adder", manifest)` 按它找到库的条目，其文件以它命名（`libadder.dylib`、`libadder.so` 和 `adder.dll`；开头的 `lib` 不会重复，在 Windows 上会被去掉），`MYGO_NATIVE_ADDER`（`ffi.EnvVar`）指定改为加载的文件，应用对每个名称只携带一个库，即使它的多个包都指定了该名称。它可以包含字母、数字、`-`、`_` 和 `.`。

**build** 是如何从包目录中的源码构建库；用其他方式发布的库可以省略它。

| 字段 | |
|---|---|
| `cargo` | Rust crate 的目录（其 `Cargo.toml`），如 `"./rust"`，其 `crate-type` 包含 `"cdylib"` |
| `zig` | Zig 包的目录（其 `build.zig`），如 `"./zig"`，它安装一个动态库 |
| `swift` | Swift 包的目录（其 `Package.swift`），如 `"./swift"`，其以库命名的产品是动态库；它在 macOS 上构建 |
| `args` | `cargo build`、`zig build` 或 `swift build` 的更多参数；指定 profile（`--profile`）或优化模式（`-Doptimize=…`）的参数会取代原本的发布模式 |
| `macOS` | 库能运行的最旧 macOS 版本，当它要求的版本比应用的最低版本或 12.0 更新时 |

在 `cargo`、`zig` 和 `swift` 中恰好指定一个，即包内的一个目录。

**release** 是发布文件所放目录的 URL：对于 GitHub 发布版本形如 `https://github.com/<owner>/<repo>/releases/download/<tag>`，由 `-release` 设置，之后的运行会沿用。应用从不读取它：每个文件都有完整的 `url`。

**files** 是已发布的构建，按 `GOOS-GOARCH`（`darwin-arm64`、`linux-amd64`、`windows-arm64`……）索引，或用 `darwin-universal` 表示同时包含两种 macOS 架构的一个文件，就像 Swift 库那样：

| 字段 | |
|---|---|
| `name` | 应用中的文件名，如 `libadder.so` |
| `url` | 下载它的地址 |
| `sha256` | 它的 SHA-256：不匹配的下载会被拒绝，文件以其 SHA-256 留在用户的缓存中 |
| `sourceHash` | 它所依据源码的哈希：构建目录中的文件（隐藏文件以及 `target`、`zig-out`、`zig-pkg`、`node_modules` 除外，换行按 LF 计）、`args` 和 `macOS` |

当 `sourceHash` 与源码匹配时，`mygo dev` 和 `mygo build` 使用已发布的文件，`mygo native build` 跳过该平台。包也可以发布不带 `build` 的库：无论用什么方式构建，只要自己写入 `files`，并且不带 `sourceHash`。
