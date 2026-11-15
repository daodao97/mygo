# render

Videos of [MyGo](https://github.com/egoist/mygo)'s native UI: a
composition is a view of package `ui` that builds each frame from the
frame's number, drawn on the GPU (Metal, Direct3D 11, OpenGL through EGL)
or the CPU, and encoded with the system's encoders, with sound, into MP4,
QuickTime or WebM; or into PNGs and JPEGs.

```go
import "github.com/egoist/mygo/plugins/render"

comp := render.Composition{
	Width: 1920, Height: 1080, FPS: 30, Frames: 90, Scale: 2,
	View: func(c *ui.Context, f render.Frame) {
		ui.Text(c, "Hello").FontSize(64).
			Opacity(float32(f.Interpolate([]float64{0, 20}, []float64{0, 1})))
	},
}
err := render.Video(ctx, comp, "hello.mp4", render.Options{})
```

See [the documentation](../../docs/plugins/render.md), and
`examples/render`.
