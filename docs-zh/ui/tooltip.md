# 工具提示

<!-- preview tooltip 160 -->

`Tooltip` 在指针停留在元素上，或键盘焦点移到元素上之后显示提示，并向辅助技术描述这个元素：

```go
ui.Button(c, "").Label("Share").Tooltip("Share with others").Children(func() {
	ui.Icon(c, share)
})
```

指针在元素上停留 0.6 秒后显示提示；通过键盘将焦点移到元素上时则立即显示。提示始终在元素上方居中显示，上方空间不足时改在下方，不随指针在元素内的位置变化，这与 Base UI 一致。一个提示消失后的 0.4 秒内，下一个提示会立即显示，例如指针沿工具栏移动时。指针离开后提示消失；在元素上按下或单击，或按 Escape，也会隐藏提示，直到指针移出后重新进入，或焦点移走后重新进入。因此单击后不会再次显示提示，菜单按钮打开菜单后也不会在旁边显示提示。元素嵌套时，只显示最内层元素的提示，且一次只显示一个。只显示图标的按钮，以及仅靠标签无法说明的内容，都应配上提示。

如果元素通过 `ActiveDescendant` 指定了焦点子元素，则以该子元素为准。例如，窗口中的自绘菜单将焦点交给键盘高亮的菜单项，提示就显示在该项旁边。

被 `MaxLines` 或 `SingleLine` 截断的文本，可以只在被截断时把完整文本显示为提示，是否截断由 `Truncated` 报告（见[文本](text.md)）。

提示采用主题的 `Inverse` 和 `InverseText` 颜色，除非主题另行设置，否则它的文本和背景会互换（见[主题](styling.md#主题)）。

## 延迟

主题的 `TooltipDelay` 设置指针停留多久后显示提示，对 `Tooltip` 和 `TooltipBase` 都有效：0 表示 0.6 秒，负值表示立即显示。切换到下一个提示时可免等待的时间窗口也随之变化，为该延迟的三分之二（默认 0.4 秒）。内容密集的设置页可以缩短延迟；小按钮较多的工具栏，或已有行内帮助的页面，可以延长延迟：

```go
t := *c.Theme()
t.TooltipDelay = 300 * time.Millisecond
c.SetTheme(&t)
```

若只用于窗口的一部分，例如工具栏，就在 `ui.Themed` 中用那个主题构建它（见[窗口一部分的主题](styling.md#窗口一部分的主题)）；每个元素的提示会按构建它时所用的主题等待。

## 无外观

`TooltipBase` 在元素旁显示自定义提示，如同 Base UI 的 Tooltip：它按上文所述决定提示何时显示和消失，`fn` 设置提示的样式并构建其内容。提示显示在元素上方并居中，上方没有空间时显示在下方；用 `AttachTo` 把它放在别处，其外边距使它和元素保持距离。`AttachedAnchors` 说明它实际落到了哪一侧，`AttachedOrigin` 让它从最靠近元素的边缘缩放（见[浮层](overlays.md#你自己的浮层)）：

```go
b := ui.Button(c, "").Label("Share").Description("Share with others")
b.Children(func() { ui.Icon(c, share) })
ui.TooltipBase(c, b, func(tip ui.Element) {
	tip.AttachTo(b, ui.AnchorRight, ui.AnchorLeft).Margin(0, 0, 0, 6)
	tip.Padding(4, 8).Radius(6).Background(ink).TextColor(paper)
	tip.Children(func() {
		ui.Text(c, "Share with others")
		ui.Text(c, "⌘S").TextColor(muted)
	})
})
```

显示时返回提示元素，否则返回 nil。提示不拦截指针事件。

## 辅助功能

辅助技术把 `Tooltip` 的提示作为帮助文本，在名称之后读作元素的描述，除非 `Description` 给出了其他描述。它把 `TooltipBase` 的提示看作工具提示，但不会朗读：应给元素一个 `Description`，说明提示的内容。
