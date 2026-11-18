# 视图

视图是将应用状态映射为界面的函数。收到输入、模型发生变化或动画运行时，MyGo 会在主线程上调用视图来构建帧。一帧可以包含多次构建，因此绘制时就能呈现操作后的结果。

窗口有一个固定的 `*ui.Context`。子构建器共享它，并在回调执行期间切换当前的父元素。构建视图某一部分的辅助函数也应接收它。

每个 `ui.Element` 值只属于一次构建。访问控件前会检查它的所有者、槽位和构建代次。可选元素用 `Valid` 判断，缺失时用 `ui.Element{}`。在开发模式和 `Tester` 中，过期使用会被诊断出来；生产环境中的方法会返回空结果或忽略该操作。

不要把构建值存进应用状态，也不要带进后台任务。持久的控件身份用 `ui.Handle` 保存，构建之间的回调用 `ui.Services` 保存。模型数据和 `ListState` 之类的控件状态则应由应用自己持有。

```go
// todoList is the app's state, which lasts.
type todoList struct {
	todos []Todo
	draft string
}

// view builds the interface from it.
func (app *todoList) view(c *ui.Context) {
	ui.Column(c).Fill().Padding(16).Gap(8).Children(func() {
		for i := range app.todos {
			ui.Checkbox(c, &app.todos[i].Done, app.todos[i].Title)
		}
		if ui.TextInput(c, &app.draft).Placeholder("New to-do").Submitted() {
			app.todos = append(app.todos, Todo{Title: app.draft})
			app.draft = ""
		}
	})
}
```

在应用启动时创建一次状态，并把它的视图交给窗口：

```go
app := &todoList{}
mygo.NewWindow(mygo.WindowOptions{Title: "To-dos", Content: ui.View(app.view)})
```

`ui.View(app.view)` 是窗口的内容（`mygo.WindowOptions.Content`）；同一个 `Content` 可以供多个窗口使用，每个窗口有自己的元素状态。

这些指南中的示例都是这样一个视图的片段：`c` 是视图的 `*ui.Context`，`app` 是它的接收者，也就是你自定义类型中保存状态的那个值，如这里的 `todoList`。`app.volume` 这样的字段或 `app.save()` 这样的方法，都是你在该类型上声明的。需要长期存在的值（如[路由器](navigation.md)）要和其他状态一起创建为字段，绝不能在视图里创建，因为视图每帧都会运行。

## 事件与动作

`Clicked` 报告自上一帧以来的单击，让控件旁边仍能使用普通的控制流：

```go
if ui.Button(c, "Save").Clicked() {
    app.save()
}
```

控件在构造函数返回时就接收用户的输入，因此内联的保存操作能看到此前构建的控件的最新编辑。

`OnClick` 会把动作安排到视图构建完成、绑定的输入也应用之后：

```go
ui.Button(c, "Save").Disabled(app.saving).OnClick(app.save)
```

如果内联的 `return`、`break` 或 `continue` 更方便，就用轮询。会修改正在构建的集合的动作则用回调，这样循环结束后模型才改变：

```go
for _, item := range app.items {
    ui.Row(c.Key(item.ID)).Children(func() {
        ui.Text(c, item.Title).Grow(1)
        ui.Button(c, "Delete").OnClick(func() {
            app.removeItem(item.ID)
        })
    })
}
```

已处理的输入会在重新构建前被消费，因此同一输入只会触发一次操作。MyGo 会在绘制该帧前，根据变化后的状态重新构建界面。

会修改值的控件接收指向该值的指针，因此不需要处理器：`ui.Checkbox(c, &app.settings.Sync, "Sync")` 把字段绑定到控件。与 Dear ImGui 和 egui 一样，控件会在构造函数返回之前通过指针写入用户的编辑，因此下一条语句就能读到新值。`Changed` 报告它在这轮构建中发生了变化，`Submitted` 则报告单行文本输入框中按下了 Enter。

```go
if ui.TextInput(c, &app.query).Placeholder("Search").Changed() {
	app.results = search(app.query)
}
```

局部副本同样可用，包括从 map 中读出的值：可以和原值比较，也可以通过 `Changed` 判断是否变化：

```go
viewed := app.viewed[file.ID]
if ui.Checkbox(c, &viewed, "Viewed").Changed() {
    app.viewed[file.ID] = viewed
}

chosen := app.language
ui.RadioGroup(c, func() {
    for _, tag := range app.languages {
        ui.Radio(c, &chosen, tag, i18n.Autonym(tag))
    }
})
if chosen != app.language {
    app.setLanguage(chosen)
}
```

容器会连同子元素一起，在构建它的调用返回时接收输入：例如单选按钮组的方向键可以在组内所有单选按钮之间移动。输入遵循用户操作时的选项：显示为只读的输入框不接受键入，本轮构建中设置的 `Disabled`、`ReadOnly` 或 `Step` 会按其显示效果，约束之后各帧的输入。

`OnChange` 和 `OnSubmit` 在构建、绑定输入之后、重新构建之前运行。如果动作应等到视图构建完成后再执行，就用回调，例如修改正在渲染的集合。它们也可以提交局部副本：

```go
viewed := app.viewed[file.ID]
ui.Checkbox(c, &viewed, "Viewed").OnChange(func() {
    app.viewed[file.ID] = viewed
})
```

## 键

在构建之前给控件设置键，这样周围集合变化时，它的状态会跟随对应条目：

```go
ui.Checkbox(c.Key(todo.ID), &todo.Done, todo.Title)
ui.TextInput(c.Key("search"), &app.query)
```

同级元素的键必须唯一，比较规则与 Go 的 map 键相同：指针按所指对象的身份比较，与对象内容无关，因此条目的指针和 ID 一样，都可以用作键。Go 无法比较的键（如切片），或与自身不相等的键（如 NaN），会按打印后的值比较。`Element.Key` 可在读取或绑定元素状态（如 `Local`、`Clicked`、`Hovered`、`Focused`、`OnUnmount`、`AnimateTo` 等）之前为容器设置键；之后再调用会在应用中记录警告，并在 `Tester` 中 panic。有状态的控件和自定义基础控件应在构建前使用 `Context.Key`。

`Context.Key` 为下一个控件设置一个待应用的键；再次调用会替换掉它。`c.Key("account").Key("secret")` 只使用 `"secret"`。创建一个带键的父元素，就能给它的子元素一个独立的身份作用域：

```go
ui.Column(c.Key(app.accountID)).Children(func() {
	ui.TextInput(c.Key("secret"), &app.secret).Password()
})
```

更改账户 ID 会给输入框一个新的身份，即使它自己的键仍然是 `"secret"`。在同一个账户内，这个输入框会在各次构建之间保留状态。新的编辑器会以 `app.secret` 为初始值；切换账户时，要单独清除或加载这个模型值。

## 从其他 goroutine 改变状态

视图在主线程上读取状态。从其他 goroutine 修改状态时用 `Window.Update`，它会在主线程上运行一个函数，然后绘制新帧：

```go
win := mygo.NewWindow(mygo.WindowOptions{Content: ui.View(app.view)})
go func() {
	items, err := fetchItems()
	win.Update(func() { app.items, app.err = items, err })
}()
```

`Window.Invalidate` 只绘制新帧，适合由你自己保证同步的状态。在视图内，`c.Invalidate()` 请求再绘制一帧，`c.After(d)` 请求延迟一段时间后再绘制一帧，例如时钟的下一秒。

每次调用 `Update` 后都会重新构建视图，无论函数有没有改变状态。如果某个 goroutine 在轮询很少变化的数据（例如连接数或状态），可以用 `mygo.RunOnMain` 在主线程上比较；它不会绘制任何内容，只在数据变化时调用 `Invalidate`：

```go
for n := range counts {
	mygo.RunOnMain(func() {
		if n != app.connections {
			app.connections = n
			win.Invalidate()
		}
	})
}
```

## 元素的状态

元素可以用 `ui.Local` 在帧之间保留自己的状态，适合自己构建的控件；见[自定义控件](custom-widgets.md)。如果它的值是 `io.Closer`，会在元素不再被构建时关闭，从而停止控件开始的工作（[停止控件开始的工作](custom-widgets.md#停止控件启动的工作)）。

如果只需在元素卸载时运行一个函数，例如取消视图发起的流式请求、计时器或订阅，就把该函数（如 `cancel`）交给 `OnUnmount`，不必定义带 `Close` 的类型：

```go
func Feed(c *ui.Context, win *mygo.Window, app *App, url string) {
	e := ui.Column(c)
	cancel := ui.Local(e, "stream", func() context.CancelFunc {
		ctx, cancel := context.WithCancel(context.Background())
		go read(ctx, url, func(line string) {
			win.Update(func() { app.lines = append(app.lines, line) })
		})
		return cancel
	})
	e.OnUnmount(*cancel)
	e.Children(func() {
		for _, line := range app.lines {
			ui.Text(c, line)
		}
	})
}
```

这个函数的运行方式和 `Local` 的 `Close` 一样：只运行一次，在主线程上，在第一个不再构建该元素的帧之后，例如视图切换到另一个屏幕、列表的行滚出视口，或浮层关闭。带退出过渡离开的元素，会在退出动画结束后运行它；每个元素在窗口关闭时也会运行它。在元素构建时、`Key` 之后调用 `OnUnmount`：每次调用都会替换该函数，最后设置的那个会运行；传入 nil 函数则移除它。

## 检查构建生命周期

在应用目录中运行 `mygo vet`，找出存放在结构体、包变量或 goroutine 中的构建值。源码迁移命令见[迁移](migration.md)，实测的帧开销见[性能](performance.md)。
