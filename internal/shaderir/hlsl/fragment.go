package hlsl

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/internal/shaderir"
)

// InstanceVariable is the global variable that the functions read in
// place of the uniform Fragment's instance names.
const InstanceVariable = "shaderInstance"

// Fragment returns p's fragment entry point as an HLSL (shader model 5)
// function named name, for a renderer's pixel shader to call, after the
// helpers, the uniforms (a cbuffer in register b<cbuffer>, laid out as
// UniformVariableOffsetsInDwords says), the textures (Texture2D T0, T1,
// ... in registers t<texture> on) and the functions it reaches:
//
//	float4 name(float4 position, ...varyings)
//
// instance, unless empty, names a uniform of p (p.UniformNames) that the
// renderer's code sets for each instance it draws rather than the
// cbuffer: the functions read in its place the static global variable
// InstanceVariable, of its type, which the renderer assigns before
// calling name. The other uniforms keep their offsets.
func Fragment(p *shaderir.Program, name string, cbuffer, texture int, instance string) string {
	lib, entry, funcs := p.FragmentLibrary()
	if entry == nil {
		return ""
	}
	c := &compileContext{library: true, instance: -1}
	if instance != "" {
		c.instance = slices.Index(lib.UniformNames, instance)
	}
	offsets := UniformVariableOffsetsInDwords(lib)

	lines := append(strings.Split(utilFuncs, "\n"), "", "{{.Structs}}")
	if len(lib.Uniforms) > 0 {
		lines = append(lines, "", fmt.Sprintf("cbuffer Uniforms : register(b%d) {", cbuffer))
		for i, t := range lib.Uniforms {
			if i == c.instance {
				continue
			}
			offset := fmt.Sprintf("c%d", offsets[i]/UniformVariableBoundaryInDwords)
			if k := offsets[i] % UniformVariableBoundaryInDwords; k > 0 {
				offset += "." + string("xyzw"[k])
			}
			lines = append(lines, fmt.Sprintf("\t%s : packoffset(%s);", c.varDecl(lib, &t, fmt.Sprintf("U%d", i)), offset))
		}
		lines = append(lines, "}")
	}
	if c.instance >= 0 {
		lines = append(lines, "", fmt.Sprintf("static %s;", c.varDecl(lib, &lib.Uniforms[c.instance], InstanceVariable)))
	}
	lines = append(lines, "")
	for i := 0; i < lib.TextureCount; i++ {
		lines = append(lines, fmt.Sprintf("Texture2D T%d : register(t%d);", i, texture+i))
	}
	lines = append(lines, "")
	for _, f := range funcs {
		lines = append(lines, c.function(lib, f, true)...)
	}
	for _, f := range funcs {
		lines = append(lines, "")
		lines = append(lines, c.function(lib, f, false)...)
	}

	var params, args []string
	for i, t := range entry.InParams {
		n := "position"
		if i > 0 {
			n = fmt.Sprintf("v%d", i-1)
		}
		params, args = append(params, c.varDecl(lib, &t, n)), append(args, n)
	}
	lines = append(lines, "",
		fmt.Sprintf("float4 %s(%s) {", name, strings.Join(params, ", ")),
		fmt.Sprintf("\treturn F%d(%s);", entry.Index, strings.Join(args, ", ")),
		"}")

	ls := strings.Join(lines, "\n")
	var st []string
	for i, t := range c.structTypes {
		st = append(st, fmt.Sprintf("struct S%d {", i))
		for j, m := range t.Sub {
			st = append(st, fmt.Sprintf("\t%s;", c.varDecl(lib, &m, fmt.Sprintf("M%d", j))))
		}
		st = append(st, "};")
	}
	ls = strings.ReplaceAll(ls, "{{.Structs}}", strings.Join(st, "\n"))
	ls = regexp.MustCompile(`\n\n+`).ReplaceAllString(ls, "\n\n")
	return strings.TrimSpace(ls) + "\n"
}
