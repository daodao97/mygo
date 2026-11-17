# 页面与导航

`ui.Router` 为窗口或窗口中的某个区域保存页面历史，类似浏览器标签页的历史记录。每个页面由路径表示，例如 `/notes/42`；`View` 用 `switch` 匹配路径模式，构建当前页面。模式中的 `{name}` 匹配一个路径段，`{name...}` 匹配其余路径。

路由器是应用状态的一部分：只创建一次，指定最先显示的页面，并保存在字段中。每帧都会运行的视图构建它所显示的页面：

```go
// notesApp is the app's state.
type notesApp struct {
	router *ui.Router
	// … the notes and the files
}

func main() {
	app := &notesApp{router: ui.NewRouter("/notes")}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: "Notes", Content: ui.View(app.view)})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (app *notesApp) view(c *ui.Context) {
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		app.sidebar(c) // stays as the pages change, see below
		app.router.View(c, func(r *ui.Route) {
			switch {
			case r.Match("/notes"):
				r.Title("Notes")
				app.notes(c)
			case r.Match("/notes/{id}"):
				r.Title("Note")
				app.note(c, r.Param("id"))
			case r.Match("/files/{path...}"):
				app.files(c, r.Param("path"))
			default:
				ui.Text(c, "Not found")
			}
		})
	})
}
```

`app.notes`、`app.note` 和 `app.files` 是 `notesApp` 的方法，它们构建那些页面，如同 `view` 构建窗口。

## 前往页面

`Push` 前往一个页面，把它排在当前页面之后；`Replace` 在它的位置显示一个页面；`Back`、`Forward` 和 `Go` 在历史中移动；相对于当前显示页面的路径会像网页中的链接那样解析，因此 `Push("?tab=info")` 改变查询，`Push("edit")` 前往一个同级页面。页面中指向某个路径的[链接](link.md)会在其路由器中前往那里，选中[侧边栏](sidebar.md)的一项会推送对应的页面，如同上面应用的侧边栏那样：

```go
func (app *notesApp) sidebar(c *ui.Context) {
	page := app.router.Path()
	ui.Sidebar(c, &page, func() {
		ui.SidebarItem(c, "/notes", nil, "Notes")
		ui.SidebarItem(c, "/files", nil, "Files")
	}).Width(220).OnChange(func() {
		app.router.Push(page)
	})
}

// In a page:
ui.Link(c, "Open note", "/notes/42")
```

`Path`、`Query` 和 `Location` 读取所显示的页面；`Location` 则包含查询，`NewRouter` 接受它，以便应用启动时再次显示这个页面。深度链接（`mygo.App.OnOpenURL`）会推送对应的页面。

## 后退与前进

用户像在浏览器和 Finder 中那样后退和前进：macOS 上的 Cmd+[ 和 Cmd+]，Linux 和 Windows 上的 Alt+Left 和 Alt+Right，鼠标的侧键以及带有这些键的键盘上的按键，macOS 上在触控板上于页面之间滑动，以及[后退和前进按钮](history-buttons.md)，工具栏可以容纳这些按钮，右键单击它们会按标题列出可后退或前进到的页面。按键会交给具有键盘焦点的路由器，没有这样的路由器时则交给窗口的第一个路由器。

在[浏览器](browsers.md)中，窗口显示的第一个路由器跟随标签页的历史，记录在 URL 的片段中（`#/notes/42`）：前往一个页面会向浏览器历史添加一个条目，浏览器的后退和前进按钮会在路由器的页面之间切换，带片段的 URL 在打开或重新加载时会显示对应的页面。重新加载后，随着浏览器前往这些页面，路由器也会得知当前页面之前和之后的页面。嵌入另一个页面框架中的应用不会改动标签页的历史。

在手机上，`router.SystemStack = true` 把历史显示为系统自己的屏幕栈，顶部是系统的导航栏，显示每页的标题和返回按钮：iOS 上由 UIKit 移动页面，并支持从左边缘轻扫返回；Android 上页面之上是 Material 的工具栏。此时 `InSystemStack` 告诉视图不要再显示自己的栏（见[手机上的界面](../mobile/interface.md#标签页和栈)）。

## 布局

在 `View` 周围构建的内容，例如侧边栏和工具栏，不随页面切换而改变。页面也可以为其路径下的子页面提供布局，例如设置页面在左侧列出各个设置分类，右侧显示选中的分类：在该路由上调用 `r.View`，即可匹配 `{name...}` 捕获的剩余路径并构建子页面。切换子页面时，外层布局及其状态保留，子页面在其中滑入或淡入；`Param` 也能读取外层布局的通配符参数。设置页面是上面视图中 `switch` 的另一个分支：

```go
case r.Match("/settings/{section...}"):
	ui.Row(c).Grow(1).Children(func() {
		app.sectionList(c) // pushes "/settings/general", "/settings/fonts"
		r.View(c, func(r *ui.Route) {
			switch {
			case r.Match("/general"):
				r.Title("General")
				app.general(c)
			case r.Match("/fonts"):
				r.Title("Fonts")
				app.fonts(c)
			}
		})
	})
```

页面内的第二个 `Router`，放在单独的字段中，就有自己的历史，如同分栏视图的详情深入下一页时，旁边的列表保持不变。

## 页面保留其状态

页面在历史中时，其元素会保留状态，前后各十个页面：回到某个页面时，会发现它仍停在原来的滚动位置，文本和键盘焦点也都在原处。带另一个查询的路径是同一个页面，位于历史的另一个条目中，保留其状态，如同标签页或搜索位于查询中的页面。

## 焦点与屏幕阅读器

切换页面会将键盘焦点移到该页面上次获得焦点的元素；如果没有，则移到页面本身，再按 Tab 可进入其中，屏幕阅读器会朗读页面的 `Title`。如果焦点在切换的页面之外，例如在侧边栏或外层布局的页面列表中，则保留原焦点，屏幕阅读器仍会朗读新页面的标题。

## 过渡

更深的路径中的页面，例如 `/notes` 之后的 `/notes/42`，会从右侧滑入，盖在它离开的页面上，返回时从左侧滑回；其他页面淡入。系统设置了减少动效时，所有页面都淡入，`router.Transition = ui.TransitionNone` 会立即显示它们。离开的页面在滑动时不接收指针或键盘输入。

`router.Transition = ui.TransitionMatch` 让离开的页面淡出，同时新页面淡入，两页中共有的 [`Match`](transitions.md#共享元素) 元素会从一处移动到另一处，如同 Keynote 的 Magic Move：例如照片的缩略图展开成它的页面。飞行的元素（`Fly`）会越过两个页面，离开页面中的元素会随新页面中的元素一起移动并淡出，到达后不再显示。

`router.Animation` 设置页面动画，可使用弹簧或曲线（见[动效](motion.md)），替代默认的缓出动画：滑动 250 毫秒、淡入 150 毫秒、交叉淡入淡出 300 毫秒：

```go
router.Animation = &ui.Snappy
```
