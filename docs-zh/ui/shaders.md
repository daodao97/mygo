# 着色器

着色器根据像素位置、应用每帧传入的值和读取的图像，在 GPU 上计算矩形内每个像素的颜色，可用于绘制动态噪声和渐变、逐像素照亮的纹理，以及横跨窗口的波纹。`ui/shader` 包编译用 Go 语法编写的着色器，支持窗口所用的各种 GPU 渲染器（Metal、Direct3D 11、OpenGL 或 WebGL 2），没有 GPU 渲染器时则由 CPU 绘制。两种方式的渲染效果一致，像素值只会因数值计算的精度限制而略有不同。

```go
import "github.com/egoist/mygo/ui/shader"

var waves = shader.MustCompile(`package main

var Time float

func Fragment(dst vec4, pos vec2) vec4 {
	v := 0.5 + 0.5*sin(pos.x/20+Time)
	return vec4(v*0.2, v*0.4, v, 1)
}
`)

ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
	waves.Draw(p, r, shader.Options{
		Uniforms: map[string]any{"Time": p.Now().Sub(start).Seconds()},
	})
	p.AnimationFrame()
})
```

`Draw` 填充绘制器中的一个矩形，和其他绘制内容一样受绘制器的裁剪、变换和透明度影响；`Options.Radius` 设置矩形的圆角。

## 文本、路径和图标

着色器还可以代替颜色来填充形状，就像渐变那样，也像 SwiftUI 用着色器填充形状或文本：`FillPath` 和 `StrokePath` 绘制一个 `ui.Path`，`Text` 绘制一行文本，`Fill` 则在指定的盒子中绘制一个函数所画的任何文本、路径和图标：

```go
var circle ui.Path
circle.Circle(60, 60, 50)

ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
	opts := shader.Options{Uniforms: map[string]any{"Time": t}}
	waves.FillPath(p, &circle, opts)
	waves.Text(p, 140, 40, "Hello", 48, opts)
	waves.Fill(p, ui.Rect{X: 140, Y: 100, W: 200, H: 40}, opts, func() {
		p.RichText(140, 100, 200, ui.Span{Text: "and more", Size: 24, Weight: 700})
		p.Icon(star, ui.Rect{X: 300, Y: 100, W: 24, H: 24}, ui.RGB(0, 0, 0))
	})
})
```

每个像素显示的是着色器的颜色乘以形状在该处的覆盖率，文本边缘的校正方式与该平台上针对这种颜色的文本一致。`pos` 的范围是形状的盒子（路径的盒子外扩半个描边宽度、文本的盒子，或 `Fill` 接受的盒子），`rectSize()` 返回这个尺寸；emoji 保持自己的颜色，`Fill` 的函数所绘制的矩形和图像则按它们原本的样子绘制。

## 一次绘制多个四边形

节点编辑器、白板笔画、MIDI 音符和粒子都是每帧都在变化的成千上万个小形状：每个形状调用一次 `Draw`，CPU 就要花掉整帧的时间来构建它们的命令。`DrawInstances` 一次调用就能绘制全部形状，每个四边形是一个以 DIP 为单位的矩形，并带有几个由其着色器读取的、属于它自己的浮点数：

```go
var dots = shader.MustCompile(`package main

func Fragment(dst vec4, pos vec2) vec4 {
	c := vec4(instanceData(0), instanceData(1), instanceData(2), 1)
	d := length(pos-rectSize()/2) / (rectSize().x / 2)
	return c * (1 - smoothstep(instanceData(3), 1, d))
}
`)

ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
	rects := make([]ui.Rect, 0, len(particles))
	data := make([]float32, 0, 4*len(particles))
	for _, pt := range particles {
		rects = append(rects, ui.Rect{X: pt.X - 6, Y: pt.Y - 6, W: 12, H: 12})
		data = append(data, pt.R, pt.G, pt.B, 0.6) // color, then where the glow starts
	}
	dots.DrawInstances(p, shader.Instances{Rects: rects, Data: data, Stride: 4}, shader.Options{})
})
```

每个四边形的绘制方式与 `Draw` 单独绘制其矩形时相同：`pos` 从它的左上角开始，`rectSize()` 返回它的尺寸，`Options.Radius` 设置它的圆角，选项中的 uniform 变量和图像对所有四边形都相同。在着色器中，`instanceData(i)` 返回该四边形自己的第 `i` 个浮点数，取自 `Data[index*Stride:]`，`instanceIndex()` 返回它在 `Rects` 中的索引；在 `Draw` 中两者都为 0。节点编辑器中的一条连线就是围绕其曲线盒子的一个四边形，以四个控制点作为数据，由着色器计算到曲线的距离；一个音符、一段笔画或一个粒子，各是一个四边形。

GPU 渲染器用一次实例化绘制来绘制它们（Metal、Direct3D 11、OpenGL 和 WebGL 2 均如此），数据随每个四边形的实例传递；CPU 渲染器则逐个绘制，与同样数量的 `Draw` 绘制出相同的像素。一次调用最多绘制 `shader.MaxInstances` 个四边形（65,536 个），每个最多带 `shader.MaxInstanceStride` 个浮点数（16 个），超出时 `DrawInstances` 会 panic；`Data` 的长度不等于 `len(Rects)*Stride` 时，或着色器读取背景时（这类着色器只绘制一个矩形），也会 panic。

## 对元素的效果

着色器还可以基于元素的渲染结果来绘制该元素，如同 SwiftUI 的 `colorEffect`、`distortionEffect` 和 `layerEffect`。入口函数代替 `Fragment`，用来说明是哪一种效果：

```go
// The content's colors, changed pixel by pixel.
func ColorEffect(pos vec2, color vec4) vec4

// Where to read the content for the pixel at pos.
func DistortionEffect(pos vec2) vec2

// The pixel's color, from the content read anywhere.
func LayerEffect(pos vec2) vec4
```

`Element.Effect` 接受着色器的 `Effect`：

```go
var wave = shader.MustCompile(`package main

var Time float

func DistortionEffect(pos vec2) vec2 {
	return pos + vec2(0, 6*sin(pos.x/12+Time))
}
`)

ui.Text(c, "Wavy").FontSize(24).Effect(wave.Effect(shader.Options{
	Uniforms:        map[string]any{"Time": t},
	MaxSampleOffset: ui.Point{Y: 6},
}))
```

元素每次绘制时，会将自身及子元素、阴影和 `Draw` 回调绘制的内容渲染到 GPU 位图中，再由效果根据这张位图绘制，替代元素原本的绘制结果。`pos` 从元素盒子的左上角算起，`rectSize()` 返回盒子的尺寸。`LayerEffect` 用 `layerAt(p)` 读取 `p` 处的像素，或用 `layerSample(p)` 读取周围像素的混合值；`p` 和 `pos` 一样以 DIP 计。`DistortionEffect` 用 `layerSample` 读取内容。超出渲染范围的内容读取为透明：`Options.MaxSampleOffset` 指定在盒子四周额外渲染的距离，供效果读取或移动该范围内的像素，效果也会绘制整个扩展范围。元素用于点击、焦点和辅助功能的位置不变：改变的只有显示内容。效果着色器可读取自带的图像 0 到 2；图像 3 用于元素内容。

## 背景

着色器可以读取窗口在它后面绘制的内容，就像 Flutter 中配合着色器的 `BackdropFilter`：磨砂窗格、透镜、随下方滚动内容变化的色调。`backdropAt(p)` 返回它在 `p` 处的像素，`backdropSample(p)` 混合 `p` 周围的像素，`p` 以 DIP 计，和 `pos` 一样；`Options.BackdropBlur` 会先模糊背景，其标准差以 DIP 计：

```go
var frost = shader.MustCompile(`package main

var Tint vec4

func Fragment(dst vec4, pos vec2) vec4 {
	b := backdropSample(pos)
	return mix(b, Tint, Tint.a)
}
`)

ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
	frost.Draw(p, r, shader.Options{
		BackdropBlur: 12,
		Uniforms:     map[string]any{"Tint": ui.RGBA(255, 255, 255, 0.3)},
		Radius:       16,
	})
})
```

渲染器会像处理 glass 插件的窗格那样，每一帧捕获背景，因此它会跟随背后移动的一切。元素的效果也会读取它。读取背景的着色器绘制矩形（`Draw`、`Effect`），而不是形状（`Fill`），并读取自己的图像 0、1 和 3；图像 2 是背景。

## 语言

着色器是一个 `package main` 的 Go 文件。它的函数 `Fragment` 返回像素的颜色，颜色已预乘（每个颜色分量都乘以 alpha）：

```go
func Fragment(dst vec4, pos vec2) vec4
```

`dst` 是像素中心的坐标，单位是窗口或所绘制画布的像素；`pos` 是同一个点，单位是 DIP，从矩形的左上角算起。`Fragment` 可以只接受 `dst`，或什么都不接受。

该语言是 [Ebitengine 的 Kage](https://ebitengine.org/en/documents/shader.html)，MyGo 的编译器基于 Kage 编译器派生而来：使用 Go 语法，支持以下类型：

| 类型 | |
| --- | --- |
| `bool`, `int`, `uint`, `float` | 32 位标量 |
| `vec2`, `vec3`, `vec4` | 浮点向量，带 swizzle（`v.xy`、`c.rgb`） |
| `ivec2` 到 `ivec4`，`uvec2` 到 `uvec4` | int 和 uint 向量 |
| `mat2`, `mat3`, `mat4` | 浮点矩阵，按列 |
| `[N]T` | 其中任何类型的数组 |

还可以使用自己的函数、`if`、`for` 循环（上界不必是常量，也可对 int 使用 `range`）、常量，以及 GLSL 的内置函数：`sin`、`cos`、`atan2`、`pow`、`exp`、`sqrt`、`abs`、`floor`、`fract`、`mod`、`min`、`max`、`clamp`、`mix`、`step`、`smoothstep`、`length`、`distance`、`dot`、`cross`、`normalize`，以及其他函数。int 和 uint 是两类整数，只在移位中混用，和 Go 一样；uint 会回绕，哈希正需要这一点：

```go
func pcg(v uint) uint {
	s := v*747796405 + 2891336453
	w := ((s >> ((s >> 28) + 4)) ^ s) * 277803737
	return (w >> 22) ^ w
}
```

`discard()` 让像素保持原样。`dfdx`、`dfdy` 和 `fwidth` 给出一个值到相邻像素的变化量，在 GPU 上有效，在 CPU 上为零。

## Uniform 变量

包级的导出变量就是 uniform：应用在 `Options.Uniforms` 中按名称设置每次绘制所用的值。一个数字设置标量；数字的切片或数组设置向量、矩阵（按列）或数组（元素按顺序）；一个 `ui.Color` 设置 `vec4`，其分量从 0 到 1。选项中没有设置的 uniform 为零。

```go
var Taps [6]vec4 // x, y, when, how hard
var Count int
```

```go
taps := make([]float32, 0, 24)
for _, t := range ripples {
	taps = append(taps, t.x, t.y, t.at, t.force)
}
fx.Draw(p, r, shader.Options{Uniforms: map[string]any{"Taps": taps, "Count": len(ripples)}})
```

对于每帧都要设置的值，`Values` 只需查找一次变量，之后每次绘制都无需使用 map、转换类型或分配内存，Flutter 的 `FragmentShader` 和 Skia 的 `SkRuntimeShaderBuilder` 也采用这种方式保存值：

```go
var (
	timeVar = waves.Uniform("Time") // panics for a name the shader lacks
	tintVar = waves.Uniform("Tint")
	values  = waves.NewValues()
)

values.Float(timeVar, float32(t)).Color(tintVar, theme.Accent)
waves.Draw(p, r, shader.Options{Values: values})
```

每个设置函数都会检查变量类型（`Float`、`Vec2` 到 `Vec4`、`Int`、`Uint`、`Bool`，以及用于向量、矩阵和数组的 `Floats`、`Ints` 和 `Uints`），类型不符或数字过多时会 panic；绘制时会复制这些值，所以一个 `Values` 可以用于多次绘制。随后，`Options.Uniforms` 中同名的值会覆盖它们。

颜色（`Values.Color`，或 `Options.Uniforms` 中的 `ui.Color`）会把 `vec3` 或 `vec4` 设为其各分量，范围 0 到 1，不预乘，采用 sRGB；窗口使用广色域时，sRGB 之外的颜色（例如 `ui.Oklch` 生成的颜色）会以扩展 sRGB 到达着色器，分量可能小于 0 或大于 1，和窗口自身的颜色一样。

## 图像

着色器最多读取四张图像（`Options.Images`）：位图，或画布（`Canvas.Bitmap`），坐标以像素计，从各自的左上角开始：

| 函数 | |
| --- | --- |
| `image0At(p vec2) vec4` | `p` 处的像素，图像外为透明 |
| `image0Sample(p vec2) vec4` | 在 `p` 周围的四个像素之间混合，并钳制到边缘 |
| `image0Size() vec2` | 图像的尺寸，以像素计 |

以及 `image1At` 到 `image3At`，等等。`rectSize()` 返回着色器填充的矩形尺寸，以 DIP 计，`pixelScale()` 返回每个 DIP 的像素数。

绘制到[画布](drawing.md#画布)中的着色器会成为另一个着色器的输入，如同效果的各个阶段：前一个在小画布的少量采样点上计算一个场，后一个在窗口的每个像素处对它采样读取。

```go
field := ui.NewCanvas(cols, rows)

box.Draw(func(p *ui.Painter, r ui.Rect) {
	field.Draw(p, func(cp *ui.Painter) {
		windShader.Draw(cp, ui.Rect{W: float32(cols), H: float32(rows)}, shader.Options{Uniforms: wind})
	})
	glyphShader.Draw(p, r, shader.Options{Images: [4]*ui.Bitmap{field.Bitmap(), atlas}})
})
```

## 错误与开销

`Compile` 会带着行号报告着色器的问题；着色器应只编译一次，可借助 `MustCompile` 作为包变量编译。`Compile` 大约需要一毫秒：它会检查着色器，并把它翻译成 GPU 的语言（Metal、HLSL 或 GLSL），再由 GPU 驱动程序编译，如同 SwiftUI 的 `Shader.compile(as:)` 在使用前所做的那样。在 macOS 上，`Compile` 会在独立的 goroutine 上启动这一步：应用第一次在这台 Mac 上运行时，Metal 的库和管线合计约需 0.2 秒，每个窗口的渲染器都用它们绘制，只需等待还没完成的部分；作为包变量编译的着色器在第一帧之前很久就准备好了，Metal 还会把两者都缓存到磁盘，之后的启动可以直接使用。Linux 的 Mesa 驱动也会把它们缓存到磁盘（`~/.cache/mesa_shader_cache`）：第一次在 CPU 的 llvmpipe 上需要 0.1 到 0.3 秒，之后只需几毫秒。在 Windows 上，Direct3D 的编译器编译短着色器需要 0.1 秒，几百行的着色器要一秒，所以 `Compile` 会在独立的 goroutine 上启动它：每个窗口的渲染器共享同一份代码，第一次绘制时只等待还没完成的部分，编译结果会留在用户的缓存目录（`MyGo\ShaderCache`）中，供之后的启动使用。在 GPU 上绘制着色器的开销很小，不管它逐像素计算什么；在 CPU 上（最后的备选渲染器，以及 `ui.Tester`），它以解释方式运行，每个像素要几微秒：整个窗口就要几秒。如果着色器覆盖窗口的大部分区域，可以查询 `p.Accelerated()`；在 CPU 上以及 GPU 渲染器实际用 CPU 绘制时（WARP、llvmpipe、SwiftShader），它返回 false，此时可以少绘制或不绘制：

```go
ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
	if !p.Accelerated() {
		return // a backdrop, which the window does without
	}
	backdrop.Draw(p, r, shader.Options{})
})
```

uniform 被设成非数字的值时，`Draw` 会 panic。
