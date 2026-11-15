package msl

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/internal/shaderir"
)

// InstanceVariable is the parameter that the functions read in place of
// the uniform Fragment's instance names.
const InstanceVariable = "shaderInstance"

// instanceParam returns the declaration of the parameter InstanceVariable.
func (c *compileContext) instanceParam(p *shaderir.Program) string {
	return "thread const " + c.varDecl(p, &p.Uniforms[c.instance], InstanceVariable, true)
}

// Fragment returns p's fragment entry point as a Metal function named
// name, for a renderer's fragment shader to call, after the functions it
// reaches and the prelude:
//
//	float4 name(constant Uniforms& uniforms, texture2d<float> T0, ..., float4 position, ...varyings)
//
// The uniforms parameter is left out when p has none, and there is a
// texture parameter for each of p's textures.
//
// instance, unless empty, names a uniform of p (p.UniformNames) that the
// renderer's code sets for each instance it draws rather than the
// uniforms: every function, name too, takes after the textures a
// parameter InstanceVariable of its type (thread const, by reference),
// which they read in its place. Uniforms keeps its room for it.
func Fragment(p *shaderir.Program, name string, instance string) string {
	lib, entry, funcs := p.FragmentLibrary()
	if entry == nil {
		return ""
	}
	c := &compileContext{structNames: map[string]string{}, library: true, instance: -1}
	if instance != "" {
		c.instance = slices.Index(lib.UniformNames, instance)
	}

	var lines []string
	lines = append(lines, strings.Split(Prelude(), "\n")...)
	lines = append(lines, "", "{{.Structs}}")
	if len(lib.Uniforms) > 0 {
		lines = append(lines, "", "struct Uniforms {")
		for i, u := range lib.Uniforms {
			lines = append(lines, fmt.Sprintf("\t%s;", c.varDecl(lib, &u, fmt.Sprintf("U%d", i), false)))
		}
		lines = append(lines, "};")
	}
	lines = append(lines, "")
	for _, f := range funcs {
		lines = append(lines, c.function(lib, f, true)...)
	}
	for _, f := range funcs {
		lines = append(lines, "")
		lines = append(lines, c.function(lib, f, false)...)
	}

	// The entry point, under its name.
	var params, args []string
	if len(lib.Uniforms) > 0 {
		params, args = append(params, "constant Uniforms& uniforms"), append(args, "uniforms")
	}
	for i := 0; i < lib.TextureCount; i++ {
		params, args = append(params, fmt.Sprintf("texture2d<float> T%d", i)), append(args, fmt.Sprintf("T%d", i))
	}
	if c.instance >= 0 {
		params, args = append(params, c.instanceParam(lib)), append(args, InstanceVariable)
	}
	for i, t := range entry.InParams {
		n := "position"
		if i > 0 {
			n = fmt.Sprintf("v%d", i-1)
		}
		params, args = append(params, c.varDecl(lib, &t, n, false)), append(args, n)
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
			st = append(st, fmt.Sprintf("\t%s;", c.varDecl(lib, &m, fmt.Sprintf("M%d", j), false)))
		}
		st = append(st, "};")
	}
	ls = strings.ReplaceAll(ls, "{{.Structs}}", strings.Join(st, "\n"))
	ls = regexp.MustCompile(`\n\n+`).ReplaceAllString(ls, "\n\n")
	return strings.TrimSpace(ls) + "\n"
}
