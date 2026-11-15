package shader

import (
	"github.com/egoist/mygo/internal/gpu/metal"
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/shader"
	"github.com/egoist/mygo/internal/shaderir"
	"github.com/egoist/mygo/internal/shaderir/msl"
)

// code gives fx p's code for the Metal renderer, and returns where the
// uniforms' lanes lie in its buffer and the buffer's size: the whole
// library, whose tail (ShaderSource in package metal) calls shaderMain.
// It starts making the library and its pipeline, which take Metal 0.1 s
// each the first time, on a goroutine of its own, which the renderers
// then share. What the shader reads of a quad of those one draw draws
// (DrawInstances) comes from the instance drawing it, in the slots of an
// effect's parameters (gpu.Builder).
func code(p *shaderir.Program, fx *scene.Effect) ([]int, int) {
	fx.Metal.Source = metal.ShaderSource(msl.Fragment(p, "shaderFragment", shader.InstanceUniform) + `
float4 shaderMain(constant Uniforms& uniforms, texture2d<float> T0, texture2d<float> T1, texture2d<float> T2, texture2d<float> T3, float2 p, float4 rect, thread const Inst& i) {
	// grad.z is 2 for a quad of those one draw draws.
	bool many = i.grad.z > 1.5f;
	float4 none = float4(0.0f);
	array<float4, 5> instance;
	instance[0] = many ? i.inner : none;
	instance[1] = many ? i.color : none;
	instance[2] = many ? i.color2 : none;
	instance[3] = many ? i.border : none;
	instance[4] = many ? float4(i.grad.w, i.grad.xy, 1.0f) : none;
	return shaderFragment(uniforms, T0, T1, T2, T3, instance, float4(p, 0.0f, 1.0f), (p - rect.xy) / uniforms.U2);
}
`)
	metal.PrecompileShader(fx.Metal.Source)
	return msl.UniformLanes(p)
}
