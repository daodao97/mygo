//go:build !darwin && !windows

package shader

import (
	"github.com/egoist/mygo/internal/scene"
	"github.com/egoist/mygo/internal/shader"
	"github.com/egoist/mygo/internal/shaderir"
	"github.com/egoist/mygo/internal/shaderir/glsl"
)

// code gives fx p's code for the OpenGL and WebGL 2 renderer, as an
// effect's, with the uniforms in the std140 block ShaderUniforms and the
// images in the samplers T0 to T3, and returns where the uniforms' lanes
// lie in the block and its size. The renderer draws with GL's y going up,
// which negates dFdy, and keeps canvases' rows going up, which it tells
// the shader in uShaderFlipRows. What it reads of a quad of those one
// draw draws (DrawInstances) comes from the instance drawing it, in the
// slots of an effect's parameters (gpu.Builder).
func code(p *shaderir.Program, fx *scene.Effect) ([]int, int) {
	fx.GLSL = glsl.Fragment(p, "shaderFragment", glsl.Options{FlipY: "true", Block: "ShaderUniforms", FlipRows: "uShaderFlipRows", Instance: shader.InstanceUniform}) + `
vec4 effect(vec2 p, Effect e) {
	// p4.z is 1 for a mask glyph, its positions from the box's corner in
	// p4.xy, and 2 for a quad of those one draw draws.
	bool many = e.p4.z > 1.5;
	vec4 none = vec4(0.0);
	shaderInstance[0] = many ? e.p0 : none;
	shaderInstance[1] = many ? e.p1 : none;
	shaderInstance[2] = many ? e.p2 : none;
	shaderInstance[3] = many ? e.p3 : none;
	shaderInstance[4] = many ? vec4(e.p4.w, e.p4.xy, 1.0) : none;
	return shaderFragment(vec4(p, 0.0, 1.0), (p - (e.p4.z > 0.5 && !many ? e.p4.xy : e.rect.xy)) / U2);
}
`
	return glsl.UniformLanes(p)
}
