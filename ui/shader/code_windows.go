package shader

import (
	"github.com/egoist/mygo/internal/gpu/d3d11"
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/shader"
	"github.com/egoist/mygo/internal/shaderir"
	"github.com/egoist/mygo/internal/shaderir/hlsl"
)

// code gives fx p's code for the Direct3D renderer, as an effect's, with
// the uniforms in the cbuffer of register b2 and the images in registers
// t6 to t9, after the renderer's own, and returns where the uniforms'
// lanes lie in the cbuffer and its size. It starts compiling the code,
// which takes Direct3D's compiler from a tenth of a second to more than
// one, on a goroutine of its own, which the renderers of all windows then
// share, with a cache on disk for the next launches. What the shader
// reads of a quad of those one draw draws (DrawInstances) comes from the
// instance drawing it, in the slots of an effect's parameters
// (gpu.Builder).
func code(p *shaderir.Program, fx *scene.Effect) ([]int, int) {
	fx.HLSL.Source = hlsl.Fragment(p, "shaderFragment", 2, 6, shader.InstanceUniform) + `
float4 effect(float2 p, Effect e) {
	// p4.z is 1 for a mask glyph, its positions from the box's corner in
	// p4.xy, and 2 for a quad of those one draw draws.
	bool many = e.p4.z > 1.5;
	float4 none = float4(0, 0, 0, 0);
	shaderInstance[0] = many ? e.p0 : none;
	shaderInstance[1] = many ? e.p1 : none;
	shaderInstance[2] = many ? e.p2 : none;
	shaderInstance[3] = many ? e.p3 : none;
	shaderInstance[4] = many ? float4(e.p4.w, e.p4.xy, 1) : none;
	return shaderFragment(float4(p, 0, 1), (p - (e.p4.z > 0.5 && !many ? e.p4.xy : e.rect.xy)) / U2);
}
`
	d3d11.PrecompileEffect(fx.HLSL.Source)
	return hlsl.UniformLanes(p)
}
