# Native modules

**Experimental.** Native modules, and binding a C library with `ffi`, may
change in any release, including in breaking ways: the Go side, the
generated Swift, Kotlin, Rust and Zig, the `mygo native` commands and
`mygo-plugin.json`. Release notes list each breaking change and what to
do about it.

A native module is a Go interface implemented in Swift, Kotlin, Rust or
Zig. You declare the interface in Go, implement what MyGo generates from
it, and call it from Go like any other Go value: with `context.Context`,
errors, iterators and callbacks, checked by the Go compiler on one side and
Swift's, Kotlin's, Rust's or Zig's on the other. There is no cgo, and nothing to
write by hand between the two.

Use one for what Go lacks or does slowly:

- what macOS and iOS offer in Swift only: their frameworks
  (NaturalLanguage, Vision, AVFoundation, Speech …), Swift packages, and
  views shown in native UI;
- what Android offers in Kotlin and Java: its SDK, Jetpack and other
  Android libraries, and views;
- a Rust crate or a Zig or C library, such as a parser, a codec, a
  database or an inference engine, on every platform.

A module keeps each implementation in a directory named after its
language, built with that language's tools:

| Directory | Language | Runs on |
|---|---|---|
| `swift/` | Swift, as a Swift package | macOS and iOS, as its `Package.swift` names them (and Linux under `go test`, with Foundation alone) |
| `kotlin/` | Kotlin, as an Android library of Gradle | Android |
| `rust/` | Rust, as a crate | macOS, Linux, Windows |
| `zig/` | Zig, as a package | macOS, Linux, Windows |

A module may have several: each platform gets one, Swift on iOS, Kotlin on
Android, Swift on macOS where there is one, and otherwise Rust, then Zig.
So a module can use Apple's frameworks on Macs and iPhones, Android's on
Android phones and a Rust crate elsewhere, behind the same Go interface
(see [Phones](#phones)). The Go side says what a module does, and each directory how. On
a platform the module has no implementation for, its constructors return
`errors.ErrUnsupported`, so the app compiles everywhere and decides what to
do without it.

The Go side:

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

Its implementation in `swift/`, with NaturalLanguage:

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

And the app, calling it:

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

The interface is the only declaration. MyGo generates the rest, and
`mygo dev` keeps it up to date as you edit:
`NewAnalyzer` in Go, and for each language the declarations its
implementation must satisfy, here the Swift protocol `Analyzer`, the struct
`Token`, the enums `Unit` and `LanguageError`, with the glue between them;
in Rust a trait `Analyzer`, in Zig a struct that checks at compile time
that yours has every method (see [Rust and Zig](#rust-and-zig)). Change the
interface and every implementation stops compiling until it matches;
forget a method and the language's compiler names it.

## Create a module

```sh
mygo native new [-lang <languages>] <module name>
```

makes a module named `<module name>` in `modules/<module name>` of the app,
or at the path you give instead of a name, such as `internal/<module name>`.
`-lang` names the languages to implement it in, separated by commas:
`swift` by default, `kotlin`, `rust`, `zig`. `-lang none` makes the Go side alone, to
agree on the interface first and implement it later.

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

The module's name is its Go package's, so it is a valid package name, such
as `language` or `media`. `<Module>` is the name in upper camel case, the
name of the Swift module and of the library: `Language` for `language`,
built as `libLanguage.dylib`, `libLanguage.so` and `Language.dll`.

To implement an existing module in another language:

```sh
mygo native add <language> [module dir]
```

makes the language's directory in the module, the current directory by
default, with a factory for every entry point and an implementation whose
every method says it is unsupported, so the module builds at once and the
methods are implemented one at a time. Kotlin, Rust and Zig refuse a
module that uses what they cannot implement (see [Rust and
Zig](#rust-and-zig) and [Phones](#phones)).

A module is an ordinary Go package. Import it from the app, or from any Go
module: `mygo dev` and `mygo build` find every package of the app with an
implementation among its dependencies (`go list -deps`), build the one for
the platform they build for and put it into the app. There is nothing to
register.

Commit the generated files. `<module name>_native.go` lets the app compile,
vet and test on any machine, with or without the languages' tools, and
`<Module>.mygo.swift`, `<Module>.mygo.kt`, `mygo.rs` and `mygo.zig`, which hold the
declarations of the Go side and the C functions Go calls, let editors
complete and check the implementation; never edit them. They say which
interface they came from, and `mygo build` refuses to build when one is
older than the interface.

## Declare the interface

The Go side of a module is its contract: interfaces, the types they use and
the errors they return, with nothing of any platform in it.

**Entry points.** A comment `//mygo:native` before an interface makes it
an entry point of the module, which the package gets a constructor for:
`NewT() (T, error)` for an interface `T`. The directive takes no options.
Make the value once, as you would open a file, and keep it.

**Factories.** Each implementation chooses what implements an entry point
with a factory named after it, which the generated code calls and the
compiler requires: `makeAnalyzer` for `Analyzer` in Swift and Zig,
`make_analyzer` in Rust. Without it the package does not compile, and
Swift says `cannot find 'makeAnalyzer' in scope`.

```swift
func makeAnalyzer() throws -> any Analyzer {
    guard #available(macOS 14, *) else { throw NativeError.unsupported }
    return NLAnalyzer()
}
```

The factory may throw, and the constructor returns its error.
`NativeError.unsupported` reaches Go as `errors.ErrUnsupported`, for what
the system lacks, such as a framework of a newer version. The macOS
versions a Swift implementation runs on are those its `Package.swift` lists
in `platforms`: on older ones, and on platforms with no implementation, the
constructor returns `errors.ErrUnsupported` without asking.

**Other interfaces.** The interfaces that the methods of an entry point
take or return are part of the module too, and generated with it, without
a constructor. They need no directive. Who implements one follows from
where it appears: an interface that a method of the module returns, such
as a loaded model, is implemented by the platform (see
[objects](#objects)); one that methods only take, such as a delegate, is
implemented by Go (see [interfaces Go implements](#interfaces-go-implements)).
MyGo stops with an error at an interface that is both.

**Capabilities.** What an implementation is, beyond its methods, is said in
Go's own way, by embedding:

- `native.View`: the implementation is a view of the platform, shown in
  native UI with `ui.NativeView` (see [views](#views)).
- `io.Closer`, or a `Close() error` method: the object is released when Go
  closes it (see [objects](#objects)).

**Threads.** A comment `//mygo:mainthread` before an interface runs all its
methods on the main thread; before a method, that method. In Swift they are
`@MainActor`; Rust and Zig are called on the main thread. Views run on the
main thread without it.

The Go doc comments of the interfaces, their methods and their types become
the documentation comments of the generated code, so every implementation
reads the same contract.

### Methods

A method may:

- take any number of the [types](#types) below, after an optional
  `context.Context`, and function parameters, which are
  [callbacks](#callbacks);
- return nothing, a value, an error, a value and an error, an
  `iter.Seq[T]` or an `iter.Seq2[T, error]`, or an `io.Closer` that stops
  what the method started (see [subscriptions](#subscriptions)).

In Swift (Rust's and Zig's are [below](#rust-and-zig)):

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

Method names become lower camel case (`DetectLanguage` → `detectLanguage`,
`URLFor` → `urlFor`) and parameters keep their Go names as Swift labels.

**Contexts.** A method that takes a `context.Context` first is `async` in
Swift and runs in a `Task`. Canceling the context cancels the task, so
`Task.isCancelled` and `try Task.checkCancellation()` see it, and a
cancellable await, such as `Task.sleep`, throws `CancellationError`, which
Go gets as `ctx.Err()`. A deadline of the context cancels the task when it
passes. The context is not a parameter of the implementation: cancellation
is all it carries there. A method without a context is synchronous and
runs on the calling goroutine's thread, or on the main thread with
`//mygo:mainthread`: keep those quick, as the view and handlers call them
on the main thread.

**Streams.** A method that returns an iterator yields what the Swift stream
yields. The stream starts when the loop starts and stops when the loop
breaks, the context is canceled or the stream finishes or throws. When Go
stops first, the stream's task is canceled and its `onTermination` runs:
stop there what the stream started.

### Callbacks

A function parameter becomes a `@Sendable` closure the implementation may
keep and call from any thread, any number of times. It stays valid as long
as what it was given to: the subscription the method returns, until it is
closed, or else the object the method belongs to.

- A callback with no results is a notification: the closure returns at
  once, and Go runs the function later on the main thread, as it runs
  every handler, in the order the implementation called it.
- A callback with results is a question: the closure waits for the answer,
  and Go runs the function on the thread that calls it, at once. Answer
  quickly when the system asks on its main thread, as it does for most
  delegates, and never wait there for work that needs the main thread.
  A callback's `error` result is thrown in Swift.

### Types

Parameters, results and the fields of structs use these types:

| Go | Swift |
|---|---|
| `bool`, `int`, `int64`, `uint8` … | `Bool`, `Int`, `Int64`, `UInt8` … |
| `float32`, `float64` | `Float`, `Double` |
| `string` | `String` |
| `[]byte` | `Data` |
| `time.Time`, `time.Duration` | `Date`, `Duration` |
| `[]T`, `map[string]T` | `[T]`, `[String: T]` |
| `*T` | `T?` |
| a struct of the package | a `struct` with the same fields, `Sendable`, and `Hashable` when its fields are |
| `type Unit string` (or an integer type) with constants | `enum Unit: String`, a case per constant |
| an interface of the module | the generated protocol, as `any P` |
| an interface or type of another module | that module's, from its Swift module (see [modules that use other modules](#modules-that-use-other-modules)) |
| `*mygo.Window` | `NSWindow` (see [windows and anchors](#windows-and-anchors)) |
| `ui.Rect` | `CGRect`, in the coordinates of the window's content view |
| `native.View` | `NSView` |
| `*native.Buffer` | `NativeBuffer` (see [buffers](#buffers)) |
| `*native.Texture` | `NativeTexture` (see [textures](#textures)) |

Exported fields become lower camel case properties; unexported ones are
left out. Structs and slices are copied across, so neither side sees the
other's later changes. A `[]byte` argument is lent to Swift for the call
without a copy: copy it, with `Data(data)`, to keep it.

MyGo stops with an error, at the line of the declaration, for
anything else: channels, `any`, maps with other keys, generic types and
types of other packages, except those above. Give such a value a struct of
the package.

Swift's own types that Go has no equivalent of, such as enums with
associated values, generic types and overloaded methods, stay inside the
implementation: convert them there to a struct, or to a struct with a field
per case and a `type Kind string` saying which one is set.

### Errors

Declare the errors an implementation may return as variables of the
package, made with `native.NewError` and a code:

```go
var (
	ErrUndetermined = native.NewError("undetermined")
	ErrNoEmbedding  = native.NewError("no_embedding")
)
```

Swift gets them as an enum named after the module, `<Module>Error`, with a
case per error in lower camel case (`ErrNoEmbedding` → `.noEmbedding`):

```swift
guard let lang = NLLanguageRecognizer.dominantLanguage(for: text) else {
    throw LanguageError.undetermined
}
```

and Go compares them with `errors.Is(err, language.ErrUndetermined)`. Any
other error Swift throws reaches Go as a `*native.Error` with no code, and
the error's domain or type (`"NSCocoaErrorDomain"`, `"ParseError.badHeader"`),
code and `localizedDescription`. `NativeError.unsupported` reaches Go as
`errors.ErrUnsupported`.

A crash in the implementation, such as `fatalError`, a force unwrap of
`nil` or an index out of range, ends the app, as a panic Go does not
recover would. A Rust panic is caught at the boundary instead, and reaches
Go as an error.

## Rust and Zig

The same module implemented in Rust, on every platform: `rust/` is a crate
whose `lib.rs` declares the generated `mod mygo;` and implements its trait,
with a crate from crates.io in `Cargo.toml`:

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

And in Zig: `zig/src/<module name>.zig` holds the factory and a struct with
a method per method of the interface, which the generated `mygo.zig`, the
library's root, checks at compile time and calls:

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
| `M(ctx context.Context, ...)` | `ctx: &Context` first | `ctx: *mygo.Context` after the arena |
| `iter.Seq2[T, error]` | `Stream<Result<T, Error>>`, from `stream_of(iterator)` | `mygo.Iterator(T)`, from `Iterator(T).of(p)` |
| `iter.Seq[T]` | `Stream<T>` | `mygo.Iterator(T)` |
| `onChange func(float64)` | `Arc<dyn Fn(f64) + Send + Sync>` | `mygo.<Interface><Method><Param>`, with `call` |
| `check func(string) error` | `Arc<dyn Fn(String) -> Result<(), Error> + Send + Sync>` | the same, whose `call` returns `mygo.Error!void` |
| `M(...) io.Closer` | `-> Box<dyn Closer>`, from `closer(f)` | `mygo.Error!mygo.Closer`, from `Closer.of(p)` |
| an interface of the module | `Arc<dyn I>` | `mygo.I`, from `I.of(p)` |
| an interface Go implements | `Arc<dyn I>`, whose methods call Go | `mygo.I`, whose methods call Go |

| Go | Rust | Zig |
|---|---|---|
| `bool`, `int`, `int64`, `uint8` … | `bool`, `i64`, `i64`, `u8` … | `bool`, `i64`, `i64`, `u8` … |
| `float32`, `float64` | `f32`, `f64` | `f32`, `f64` |
| `string`, `[]byte` | `String`, `Vec<u8>` | `[]const u8` |
| `time.Time`, `time.Duration` | `SystemTime`, `Duration` | `mygo.Time`, `i64` nanoseconds |
| `[]T`, `map[string]T` | `Vec<T>`, `HashMap<String, T>` | `[]const T`, `[]const mygo.Entry(T)` |
| `*T` | `Option<T>` | `?T` |
| a struct, an enum of the package | a `struct`, an `enum` | a `struct` with defaults, an `enum` |
| `*native.Buffer` (a parameter) | `Buffer`, with `bytes_mut()` | `mygo.Buffer`, with `bytes()` |

Names follow each language: methods and fields are snake case in Rust
(`detect_language`) and lower camel case in Zig (`detectLanguage`), and Go
names that are keywords are escaped (`r#type`, `@"type"`).

**Errors.** The package's errors are an enum, `LanguageError::Undetermined`
in Rust (`?` turns it into an `Error`) and `error.Undetermined` in Zig.
`Error::Unsupported` and `error.Unsupported` reach Go as
`errors.ErrUnsupported`; any other error as a `*native.Error` with a domain,
number and message, which Rust writes `Error::Other { domain, number,
message }` (or `Error::other(message)`) and Zig `mygo.fail(domain, number,
message)`.

**Threads.** A method without a context runs on the calling goroutine's
thread, and one with a context on a thread of its own: Rust's `Context` and
Zig's `mygo.Context` say when Go canceled it (`is_canceled`, `isCanceled`)
and sleep until then (`sleep`). A stream runs on a thread of its own, which
asks the iterator for each value Go's loop wants. Implementations are
called from any thread, so Rust's traits are `Send + Sync`, and state they
change needs a `Mutex` or atomics.

**Memory in Zig.** Each call gets an arena, which holds its arguments and
is freed once its results are sent: return slices of it, or of the
arguments, without copying. A stream's arena and arguments last until it
ends, and each value it yields gets an arena of its own. What outlives the
call, as objects, iterators and callbacks kept with `keep()`, is allocated
with `mygo.allocator` and freed in its `deinit`, which Go calls when it
releases the value. A callback a method keeps past the call is `keep()`ed
and `release()`d when done.

**What only Swift implements.** Views and windows are the platform's,
so a module that uses them is implemented in Swift, or in Kotlin for
Android, and `mygo native add rust` says which of its methods stop it.
Textures and the interfaces of other modules are Swift's only, and so is
returning a `*native.Buffer`, rather than taking one.

## Phones

iPhones and Android phones build a module from its sources, with the app:
`mygo dev` and `mygo build` for `-platform ios` build its Swift with
`xcodebuild` into a framework of the app, and for `-platform android` add
its Kotlin to the app's Gradle build, one library per module. They never
use published builds: iOS signs what it runs with the app, and an
Android app's libraries are its own build. A module has no
implementation on a phone without the directory, and its constructors
return `errors.ErrUnsupported` there, as on any platform it lacks.

### Swift on iOS

The module's Swift runs on iOS when its `Package.swift` names iOS among its
platforms, with the oldest version it supports, and on macOS when it names
macOS, or no platform:

```swift
platforms: [.macOS(.v13), .iOS(.v16)],
```

The same Swift serves both, with `#if os(iOS)` where they differ, as the
generated code does: views are `UIView`s on iOS (`NativeView` names
`NSView` or `UIView`), and windows arrive as the `UIViewController` that
shows them, to present from. A module for iOS alone names only `.iOS`:
macOS then uses its Rust or Zig, as the SQLite plugin does. Swift packages,
binary frameworks (`.binaryTarget`) and resources work as in Xcode, which
builds them; iOS needs Xcode, not only its Command Line Tools.

### Kotlin on Android

`mygo native add kotlin` makes `kotlin/`, an Android library whose
`build.gradle.kts` names its namespace and its dependencies, and whose
Kotlin is in the package of the module's import path, reversed as Java's
are: `example.com/app/modules/language` is
`com.example.app.modules.language`. The app's build sets its SDK levels,
Java and Kotlin, and its dependency on MyGo's host library, `dev.mygo`,
which brings coroutines. The implementation is a function per entry point
and a class implementing each interface:

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
| `M(...) error`, `M(...) (T, error)` | `fun m(...)`, `fun m(...): T`, which throw |
| `M(ctx context.Context, ...)` | `suspend fun m(...)`, canceled with Go's context |
| `iter.Seq2[T, error]`, `iter.Seq[T]` | `Flow<T>`, which Go's loop pulls |
| `onChange func(float64)` | `(Double) -> Unit` |
| `M(...) io.Closer` | `AutoCloseable`, from `NativeCloser { }` |
| an interface of the module | the interface, implemented by a class |
| an interface Go implements | the interface, whose methods call Go |
| `bool`, `int`, `int32`, `uint16`, `uint64` … | `Boolean`, `Long`, `Int`, `UShort`, `ULong` … |
| `float32`, `float64` | `Float`, `Double` |
| `string`, `[]byte` | `String`, `ByteArray` |
| `time.Time`, `time.Duration` | `java.time.Instant`, `kotlin.time.Duration` |
| `[]T`, `map[string]T`, `*T` | `List<T>`, `Map<String, T>`, `T?` |
| a struct, an enum of the package | a `data class`, an `enum class` |
| `*native.Buffer` (a parameter) | `NativeBuffer`, with `bytes`, a direct `ByteBuffer` |
| `native.View`, `*mygo.Window` | an `android.view.View`, the window's view |
| `ui.Rect` | `android.graphics.RectF`, in the view's pixels |

The package's errors are `<Name>Exception(<Name>Error.X)`;
`NativeUnsupportedException` reaches Go as `errors.ErrUnsupported`, and
any other exception as a `*native.Error`, with `NativeException(domain,
number, message)` setting its domain and number. Methods run on the
calling goroutine's thread, attached to the JVM, `suspend` ones and
streams on coroutines of their own, and `//mygo:mainthread` ones, views
and their callbacks on the UI thread. `dev.mygo.Native.context` is the
application's `Context`, and `Native.activity` the activity, to start
others from. A view is an `android.view.View` the module makes with
`Native.context`.

Kotlin implements what Swift does but textures, returning a
`*native.Buffer` and the interfaces of other modules, which `mygo native
add kotlin` says.

**C on Android.** A Kotlin module may bring C, or C++, which its
`build.gradle.kts` builds with CMake (`externalNativeBuild`); the app's
build gives it the NDK and the ABIs that Go builds for, and the Kotlin
loads it with `System.loadLibrary`. The SQLite plugin's module,
`plugins/sqlite/internal/sqlitecore`, does so: its Kotlin calls SQLite's
amalgamation, which its CMake downloads, pinned by its SHA-256, and its
Swift links the same SQLite as a binary framework.

## Objects

A method may return another interface of the package, which needs no
directive: the implementation returns any object conforming to it, and Go
gets a value of the interface that calls it. Use this for what has a
lifetime: a loaded model, a session, a connection.

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

An implementation is a class: the generated protocols are `AnyObject`.
The Go value holds a strong reference to the Swift object. An interface
that embeds `io.Closer`, or has a `Close() error` method, is closed
explicitly: Go calls the object's `close()`, then releases it, and every
later call returns `native.ErrClosed`. Otherwise, and as a safety net for
objects never closed, the object is released after the Go value becomes
unreachable.

An object the module returns can be passed back to it, or to another
module: the implementation receives the same instance, not a copy.

### Subscriptions

A method that starts something lasting, such as observing a value or
listening for a system event, returns an `io.Closer` that stops it:

```go
type Analyzer interface {
	// ...
	// OnLanguagesChange calls f when the user changes the preferred
	// languages, until the subscription is closed.
	OnLanguagesChange(f func(languages []string)) io.Closer
}
```

In Swift it returns a `Closer`, which `NativeCloser` makes from a closure
that runs once:

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

Closing it runs the closer once and releases the callback; the
implementation calling it afterwards does nothing.

## Interfaces Go implements

An interface that methods of the module only take, never return, is
implemented by Go: the platform asks it, as AppKit asks a delegate or a
data source. Pass any Go value that implements it.

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

It becomes a protocol too, and Swift receives an object conforming to it
that calls Go:

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

Its methods follow the rules of [callbacks with results](#callbacks): one
without a context runs on the calling thread, at once, and returns its
results; one with a context is `async throws` in Swift, runs on a goroutine
of its own, and canceling the Swift task cancels its context. A Go error is
thrown as a `NativeError`, or as the module's error enum when it is one of
the package's.

Swift may keep the object, and Go keeps the value alive as long as Swift
does, until it releases the object.

## Views

An interface that embeds `native.View` is implemented by a view of the
platform and shown with `ui.NativeView`, which takes a `native.View`, as
`ui.WebView` shows a web view: make it once, outside the view, and show it
from the view.

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

In Swift the view is an `NSView` subclass conforming to the protocol, which
the generated code declares `@MainActor` and constrained to `NSView`:

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

A native view is a box of native UI like a web view: it has no size of its
own, takes its size from layout, and is cut to the element's `Radius`.
What the view paints after it shows over it, so captions, controls,
popovers and dialogs draw on top of the video. MyGo places the `NSView`
under the frame of native UI, sizes and clips it: don't move, resize or
remove it yourself. It receives the mouse, and the keyboard when it has
the focus, as the first responder, and joins the window's accessibility
tree, as any `NSView`. A view the window's view does not build hides and
keeps its state.

A view runs on the main thread, as AppKit wants, with no
`//mygo:mainthread`. Make it once the app runs, in `App.WhenReady` or
later; it is closed and released there too. Its callbacks run on the main thread too; call the
window's `Invalidate` from them to show what changed.

### Windows and anchors

What the system shows of its own, such as a share menu, a Quick Look
panel, a popover or a sheet, needs the window it belongs to and where in it
to point. Take a `*mygo.Window`, and a `ui.Rect` in the window, such as an
element's bounds:

```go
//mygo:native
//mygo:mainthread
type Sharing interface {
	// Share shows the system's share menu for the files, from rect of win.
	Share(win *mygo.Window, rect ui.Rect, paths []string)
}
```

Swift receives the `NSWindow`, and the rectangle as a `CGRect` in the
coordinates of its content view, flipped from native UI's top-left origin,
ready for AppKit's `relativeTo:of:` arguments:

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

A `native.View` parameter, such as a view of another module, arrives as
its `NSView`, to anchor to it instead.

## Modules that use other modules

A module may use the types and interfaces of another native module: import
its Go package and use them in the interface, as any Go type.

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

The generated Swift imports the other module's Swift module, and the
implementation receives the object that module returned, an `AVAsset` in
its own class, not a copy.

Go's imports are the dependencies between modules: there is nothing to add
to `Package.swift`. The generated declarations of a module are `public`
for others to see; its Go package loads the modules it uses first.
`mygo dev`, `mygo build` and `mygo native check` build the modules a
module uses before it, each from its own Swift package, and build it
with their Swift modules and libraries, which it links: the app gets each
module once, however many modules use it, next to the others in
`Contents/Frameworks`.

`mygo dev` also writes the app's modules as the targets
of one Swift package, `.mygo/native`, each depending on the modules its
Go package imports and on the packages its own `Package.swift` declares,
its sources a link to the module's. Open that package in Xcode, or point
SourceKit-LSP at it, to edit a module that uses others with completion;
it builds as the app does, so two modules that need incompatible versions
of one Swift package stop it as SwiftPM resolves them.

## Large data

Values cross as copies, which suits arguments and results but not a stream
of video frames or audio samples. Two types cross without copying.

### Buffers

A `*native.Buffer` is memory that Go and the implementation both use:
`native.NewBuffer(n)` makes one, and `Bytes()` is its contents in Go.
Pass it to have the implementation fill it, as `io.Reader` fills a slice,
or yield buffers from a stream:

```go
type Recorder interface {
	// Samples yields the microphone's PCM samples, 16-bit, a buffer at a
	// time. A buffer is valid until the loop asks for the next one.
	Samples(ctx context.Context) iter.Seq2[*native.Buffer, error]
	// Read fills buf with what was recorded since the last read.
	Read(buf *native.Buffer) (int, error)
}
```

In Swift it is a `NativeBuffer`, whose `bytes` is an
`UnsafeMutableRawBufferPointer` over it, valid as long as the buffer:

```swift
func read(buf: NativeBuffer) throws -> Int {
    ring.read(into: buf.bytes)
}
```

A buffer a stream yields is lent until the loop continues: copy what you
keep, as with `bufio.Scanner.Bytes`. Swift yields buffers from a
`NativeBufferPool`, which reuses each one once Go's loop moves on, so a
stream of buffers allocates none once it runs.

### Textures

A `*native.Texture` is an image on the GPU that the implementation draws
into and native UI shows, with no copy through the CPU: it is a
`ui.ImageSource`, as a `*ui.Bitmap` is.

```go
type Frames interface {
	// Attach draws the video's frames into tex as they play.
	Attach(tex *native.Texture) io.Closer
}
```

In Swift it is a `NativeTexture`, which takes frames with `present`: an
`IOSurface`, or a `CVPixelBuffer` backed by one. `pixelBufferAttributes`
makes such buffers in the pixel format native UI draws without converting,
and `displayLink` calls back on each refresh of the window's display:

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

The window shows what Swift presents in its next frame, without the app
calling `Invalidate`. Native UI holds the surface it shows, and those
frames on the GPU may still read, in use (`IOSurfaceIncrementUseCount`):
a pool of pixel buffers gives a surface to the implementation again only
once no frame shows it, so draw each frame into a buffer of a pool rather
than into one again and again. `displayLink`'s callback gets the host
time the frame shows at, in seconds, as `CACurrentMediaTime` counts, and
`invalidate()` stops it. Unlike a [view](#views), a texture is drawn by native
UI itself: it can be covered, clipped, scaled, rotated and faded like any
image, and drawn by a [shader](ui/shaders.md).

## Dependencies

`swift/Package.swift` is the module's Swift package, and yours to edit:
add Swift packages and binary targets to its `dependencies`, link system
frameworks with `linkerSettings`, and list the systems it runs on in
`platforms`.

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

Keep the `<Module>` product dynamic, named after the module, with the
generated file among its sources. The packages `mygo native new` writes
compile in the Swift 5 language mode (`swiftLanguageModes: [.v5]`), which
keeps Swift 6's strict checks of concurrency out of the way of
implementations, whose classes Go calls from its own threads. Dynamic frameworks the package brings,
such as an XCFramework's, go into the app's `Contents/Frameworks` with it,
and `mygo build` signs them all with the app's identity.

`rust/Cargo.toml` is the crate's, whose `[dependencies]` take crates as
any crate's do; keep its `crate-type = ["cdylib"]`. `zig/build.zig` builds
the library from `src/mygo.zig`, and `build.zig.zon` fetches Zig packages,
or a C library's sources for `build.zig` to compile, as the
[SQLite plugin](plugins/sqlite.md) does.

## Info.plist and entitlements

Some frameworks need a usage description in the app's `Info.plist`, such as
Speech, the microphone and the camera, or an entitlement, such as a shared
keychain or App Groups. Put them in the module's `swift/Info.plist` and
`swift/Entitlements.plist`, which `mygo build` and `mygo dev` merge into
the app's on macOS:

```xml
<!-- swift/Info.plist -->
<plist version="1.0">
<dict>
    <key>NSSpeechRecognitionUsageDescription</key>
    <string>Recognize speech in your recordings.</string>
</dict>
</plist>
```

Dictionaries merge by key, arrays by their values, and the app's own
`macos.infoPlist` and `macos.entitlements` ([configuration](configuration.md))
come last and win, so an app words a module's usage descriptions itself.
Two modules that set one key to different values stop the build, with both
named, unless the app sets it. An entitlement that a provisioning profile
must allow, such as App Groups, needs one that does
([provisioning profiles](distribution.md#provisioning-profiles)).

## Build, run and test

Building a module needs its language's tools: for `swift/`, Xcode or its
Command Line Tools, with Swift 6 (Xcode for iOS); for `kotlin/`, what
Android apps need already, the Android SDK and Gradle's JDK
([phones](mobile/README.md#what-you-need)), and CMake from the SDK Manager for
modules with C; for `rust/`, cargo
([rustup](https://rustup.rs)); for `zig/`, [Zig](https://ziglang.org) 0.16.
`mygo doctor` says which are installed.

`mygo dev` regenerates the module when its interface changes, rebuilds the
implementation for this machine when one of its files changes, and
restarts the app, which reopens its windows where they were. `mygo build`
builds the implementation of every module for the platform it builds for,
in release mode: Swift universal for arm64 and x86_64, signed with the
frameworks it brings, before it signs and notarizes the app
([distribution](distribution.md)), and Rust and Zig as the libraries of
each architecture, among the app's resources. cargo and Zig keep their
files in `.mygo/native` of the Go module, not in the module's directories:
keep `.mygo/` in your `.gitignore`, as the projects `mygo init` makes do.

`go test` and `go run` run the implementation of the platform they run on
once `mygo dev`, or `mygo native build -target host`, has built it;
they say to run it when the build is missing or older than the sources.
`-lang` builds another of the module's languages instead, to test each:

```sh
mygo native build -target host -lang rust ./modules/language && go test ./modules/language
mygo native build -target host -lang zig ./modules/language && go test ./modules/language
```

On phones, the module runs in the app: `mygo dev -platform ios` and
`-platform android` build it with the app and run it on the simulator,
emulator or device, where its logs show beside Go's. `go test` does not
run there, so test the Go that uses the module through a fake of its
interface, below, and the implementation in an app.

On Linux, the same commands build the Swift of a module that uses only
Foundation with a [Swift toolchain](https://www.swift.org/install/linux/),
as the images of GitHub's Ubuntu runners have, so its tests run in a Linux
job too; apps run Swift on macOS only. Code that takes the interface,
rather than the constructor's result, is tested anywhere with a fake of
it:

```go
type fakeAnalyzer struct{ lang string }

func (f fakeAnalyzer) DetectLanguage(text string) (string, error) { return f.lang, nil }

func (f fakeAnalyzer) Tokens(ctx context.Context, text string, unit language.Unit) iter.Seq2[language.Token, error] {
	return func(yield func(language.Token, error) bool) {
		yield(language.Token{Start: 0, End: len(text), Text: text}, nil)
	}
}
```

To stop at a breakpoint in the implementation, attach a debugger to the
app that `mygo dev` runs: Xcode's (Debug ▸ Attach to Process), or
`lldb -p <pid>` and `gdb -p <pid>`. What it logs, with `Logger`, `print`,
`eprintln!` or `std.debug.print`, shows in `mygo dev`'s output, beside
Go's.

## Publish a module

Apps whose modules are their own, or in a Go module they replace with a
directory, build them from their sources, as phones always do. Apps that use your module from
the module cache, and machines without its language, such as a Linux
runner of GitHub Actions building a macOS app, use its published builds
instead. `mygo native build` makes them:

```sh
mygo native build -upload [module dir]
```

builds the implementation for macOS, Linux and Windows on amd64 and arm64,
each in its language there, uploads the files as the assets of a GitHub
release, and records them in `mygo-plugin.json` next to the module: their
names, URLs and SHA-256, and the hash of the sources each was built from.
Swift is published as one universal file, `darwin-universal`, a `.tar.gz`
of the library and the frameworks it brings. Commit `mygo-plugin.json`:
every machine then downloads those builds instead of compiling them, while
they match the sources.

It uploads with the [GitHub CLI](https://cli.github.com) to the module's
own release, `native-<name>` (`native-adder`), of the repository of the
module's git checkout, which it creates once, not as the latest release,
or to the `-release` URL; every build of the module goes there. Each file is named after its SHA-256 and
never replaced, so the builds that older commits record stay
downloadable. Without `-upload`, it writes the files into
`.mygo/native/release` of the Go module, for you to publish under
`-release`.

The hash of the sources covers the files of the language's directory,
with line endings as LF so that checkouts on every platform agree, and,
for Swift, those of the native modules the module uses. A platform whose
recorded build matches them is left alone, so publishing on every push
uploads only what changed; `-force` builds it again. Publish each module
the app uses: a module's Swift archive holds its own library, not those of
the modules it uses.

`mygo build` takes the published build while its hash is the sources', and
`mygo dev` where the language's tools are missing; once the sources
change, a machine with the tools builds them, with a warning, and one
without stops and says to publish them again.

A build that fails stops the command, and nothing is published. Platforms
that build only on another system, as Swift and Rust libraries for macOS
off a Mac, are skipped, listed at the end, and left as they were in
`mygo-plugin.json`, so each can be built where it is easiest:

- **Swift** builds on a Mac.
- **Zig** cross-compiles to every platform, macOS included.
- **Rust** needs each target (`rustup target add aarch64-unknown-linux-gnu`
  …) and a linker for it: with
  [cargo-zigbuild](https://github.com/rust-cross/cargo-zigbuild)
  installed, the CLI links Linux and Windows libraries with Zig. macOS
  libraries need Apple's SDK: build them on a Mac.

```sh
mygo native build -target darwin-arm64,darwin-amd64 -upload ./modules/language   # on a Mac
mygo native build -target linux-amd64,linux-arm64,windows-amd64,windows-arm64 -upload ./modules/language
```

Linux libraries are linked against glibc 2.28 (by Zig, and by
cargo-zigbuild when it is installed), so that they load on Debian 10,
Ubuntu 20.04 and later. macOS libraries that `mygo dev` and `mygo build`
build support the app's `macOS.minimumSystemVersion`, and published ones
macOS 12 and later. The CLI removes debug information from the Linux
libraries it builds.

| Flag | |
|---|---|
| `-target` | the platforms, `GOOS-GOARCH` separated by commas, `all` (the default) or `host` |
| `-lang` | builds modules in this language where they have it, rather than each platform's |
| `-release` | the URL of the directory the files are published in (default: the library's `release`) |
| `-out` | the directory of the files to publish (default: `.mygo/native/release` of the module) |
| `-upload` | uploads the files to the GitHub release of `-release` |
| `-force` | builds the platforms whose published files were built from the sources as they are too |

## Check and call a module

Two commands check a module without opening the app, for you and for
agents:

```sh
mygo native check [-lang <language>] [module dir]
```

generates the module, builds its implementation for this machine (or in
`-lang`) and vets the Go package, then lists every problem as
`file:line: message`, with what to change, such as `language.go:14: Tokens
returns chan Token, which an implementation cannot return: return
iter.Seq2[Token, error]`. It exits with status 1 when there are problems.

```sh
mygo native call [-timeout <duration>] [module dir] <Interface>.<Method> [<arguments>]
```

makes the implementation of the interface, an entry point, calls one of its
methods with `<arguments>`, a JSON object of the method's parameters by
their Go names, and prints the result as JSON, a line per value of a
stream:

```sh
mygo native call modules/language Analyzer.DetectLanguage '{"text": "Bonjour"}'
```

It takes no `ctx` argument: it makes a context, which `-timeout` limits.
Callbacks print their calls.

Every command of `mygo native` is in [the CLI's reference](cli.md#mygo-native).
The `AGENTS.md` that `mygo native new` writes into the module says this, and
`mygo install-skills` installs a skill that teaches agents to write native
modules.

## Use it from web pages

A native module is a Go value, so bind it to give its methods to web pages
as well, through the [generated client](bindings.md):

```go
a, _ := language.NewAnalyzer()
mygo.Bind(&Text{a}) // a service whose methods call a
```

## How it works

The generated Go calls the implementation's library, which package `ffi`
finds and loads, through purego, with no cgo, as MyGo calls the system:
one C entry point per method, which the generated Swift exports with
`@_cdecl`, Rust with `#[no_mangle] extern "C"` and Zig with `export`
(on Android, the generated Kotlin's `invoke`, which the host's JNI calls),
and
one Go callback shared by every module, for results of asynchronous
methods, the values of streams, callbacks, and calls into interfaces Go
implements. The three languages share this one interface, so Go does not
know which one it calls. Go gives each library its C runtime's `malloc` and
`free` when it loads it, with which both sides allocate and free what they
hand each other.

Values cross in a binary encoding that both sides' generated code writes
and reads in the order of the declarations, with no reflection and no
field names: integers as 8 bytes, floats as float64, strings and `[]byte`
as their length then their bytes, slices and maps as their length then
their elements, pointers as a flag then the value, structs as their
fields, enums as their values, times as Unix seconds and nanoseconds.
Results are read in place from the memory the implementation returns them
in. A method of numbers and bools only, which returns no error, skips even
that: Go calls its C function with them as registers. Buffers are memory
both sides map, and textures `IOSurface`s shared with the GPU. An app that
has no native module links none of this.

## Bind a C library yourself

A module's generated code is the simplest way to call Rust, Zig or Swift.
To call a library through its own C functions instead, such as a C library
that already exists or one a Zig package builds from C sources, bind them
with package `ffi`: the CLI builds the library from its sources, puts it
into the app and publishes it as it does a module's. The
[`rust-native` example](../examples/rust-native) is a window of native UI
that calls a Rust crate so, and the [terminal](plugins/terminal.md)
plugin binds libghostty-vt, a C library that Zig builds.

Put the library's sources next to the Go package that binds it, as a Rust
`cdylib` crate, a Zig package whose `build.zig` installs a dynamic library,
or, on macOS, a Swift package whose product, named as the library, is a
dynamic library with its functions exported by `@_cdecl`:

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

`mygo-plugin.json` names the library and how it builds:

```json
{
  "libraries": [
    { "name": "adder", "build": { "cargo": "./rust" } }
  ]
}
```

and the Go package embeds it and binds the functions:

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

`mygo dev` and `mygo build` build it as they do modules, `mygo native
build` builds it for `go test` (`-target host`) and publishes it, and a
`go:generate` line in the package builds it for tests:

```go
//go:generate go tool mygo native build -target host .
```

`{"zig": "./zig"}` names a Zig package instead, whose library the CLI
builds with `zig build -Dtarget=<target> -Doptimize=ReleaseFast`, and
`{"swift": "./swift"}` a Swift package, which builds on macOS, universal,
and goes into the app's `Frameworks`.

### Binding functions

`Library.Bind` takes a pointer to a struct of functions, which it binds the
first time; later calls with the same pointer, from any goroutine, return
the first one's result without binding again, so calling it before each
use, as above, is safe and cheap. A function bound to nothing, as an
unknown tag option or a missing function, makes it return an error. A
field calls the library's function named by its tag, or else its name in
snake case (`CountVowels` calls `count_vowels`):

```go
var fns struct {
	Add         func(a, b int32) int32
	CountVowels func(s string) uint32
	Version     func() string `ffi:"adder_version"`
	Extra       func()        `ffi:"adder_extra,optional"` // nil when the library lacks it
}
```

`BindFunc` binds one function, and `Symbol` returns the address of any
symbol, for `purego.SyscallN` or a pointer to data.

Functions are called through [purego](https://github.com/ebitengine/purego),
on the calling goroutine, and pass what C does:

| Go | C | Rust |
|---|---|---|
| `int8` … `int64`, `uint8` … `uint64`, `int`, `uintptr` | integers of the same size | `i8` … `i64`, `u8` … `u64`, `isize`, `usize` |
| `float32`, `float64` | `float`, `double` | `f32`, `f64` |
| `bool` | `bool` | `bool` |
| `string` argument | `const char *`, valid during the call | `*const c_char` |
| `string` result | `const char *`, copied | `*const c_char` |
| `unsafe.Pointer`, `*T` | pointers | `*mut T`, `*const T` |
| `[]byte` | a pointer to the first byte, valid during the call | `*const u8` (pass the length too) |
| structs of the types above | structs, whose padding the Go struct must spell out | `#[repr(C)]` structs |

The library must not keep Go memory past the call, as with cgo. A string
the library returns is copied into Go but not freed, so return static
strings, or return a pointer (`unsafe.Pointer`) and export a function that
frees what the library allocated:

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

Libraries are loaded once and stay loaded. `ffi` works on macOS, Linux and
Windows on amd64 and arm64; elsewhere, as in browsers, `Load` returns an
error that wraps `errors.ErrUnsupported`.

### Where libraries are found

`Library.Load`, and a module's constructors, load the first of:

1. the file `$MYGO_NATIVE_<NAME>` names (`MYGO_NATIVE_ADDER`);
2. the one among the app's resources (or `Frameworks`), where `mygo dev`
   and `mygo build` put it, signed with the app on macOS;
3. the one next to the executable;

and, in programs that are not packaged apps:

4. the one the CLI built, in `.mygo/native/<goos>-<goarch>` of the module
   of the working directory;
5. the published one, downloaded once into the user's cache and checked
   against its SHA-256.

Packaged apps never download libraries: the production builds of `mygo
build` leave the downloader out, and HTTP with it.

## mygo-plugin.json

The manifest sits next to a Go package that binds libraries, or a native
module, and names its libraries and their published builds. The CLI finds
it in every package an app imports. A library being published looks like
this:

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

You write `name` and `build`; `mygo native build` writes `release` and
`files`. A native module's library is named after the module (`Language`)
and has no `build`: its languages are how it builds, and `mygo native
build` writes its entry itself.

**name** identifies the library everywhere: `ffi.New("adder", manifest)`
finds the library's entry by it, its files are named after it
(`libadder.dylib`, `libadder.so` and `adder.dll`; a leading `lib` is not
doubled, and is dropped on Windows), `MYGO_NATIVE_ADDER` (`ffi.EnvVar`)
names a file to load instead, and an app ships one library of each name,
even when several of its packages name it. It may hold letters, digits,
`-`, `_` and `.`.

**build** is how to build the library from sources in the package's
directory; leave it out for a library you publish by other means.

| Field | |
|---|---|
| `cargo` | the directory of a Rust crate (its `Cargo.toml`), as `"./rust"`, whose `crate-type` includes `"cdylib"` |
| `zig` | the directory of a Zig package (its `build.zig`), as `"./zig"`, which installs a dynamic library |
| `swift` | the directory of a Swift package (its `Package.swift`), as `"./swift"`, whose product named as the library is a dynamic library; it builds on macOS |
| `args` | more arguments of `cargo build`, `zig build` or `swift build`; one naming a profile (`--profile`) or an optimize mode (`-Doptimize=…`) replaces the release one |
| `macOS` | the oldest macOS the library runs on, when it needs a later one than the app's minimum or 12.0 |

Name exactly one of `cargo`, `zig` and `swift`, a directory inside the
package.

**release** is the URL of the directory the published files go into, as
`https://github.com/<owner>/<repo>/releases/download/<tag>` for a GitHub
release, which `-release` sets and later runs reuse. Apps never read it:
each file has its full `url`.

**files** are the published builds, by `GOOS-GOARCH` (`darwin-arm64`,
`linux-amd64`, `windows-arm64` …), or `darwin-universal` for one file of
both macOS architectures, as Swift libraries are:

| Field | |
|---|---|
| `name` | the file in apps, as `libadder.so` |
| `url` | where to download it |
| `sha256` | its SHA-256: a download that does not match is refused, and the file stays in the user's cache under it |
| `sourceHash` | the hash of the sources it was built from: the build directory's files (but hidden ones and `target`, `zig-out`, `zig-pkg`, `node_modules`, with LF line endings), `args` and `macOS` |

While `sourceHash` matches the sources, `mygo dev` and `mygo build` use
the published file, and `mygo native build` skips that platform. A package
may publish libraries without `build`, built by any means, by writing
`files` itself, without `sourceHash`.
