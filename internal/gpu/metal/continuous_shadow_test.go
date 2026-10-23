//go:build darwin

package metal

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/gpu/gputest"
)

// Full formula before the unclamped-shadow shortcut; blend is ignored.
const referenceContDist = `float contDist(ContCorner c, float u, float v, bool blend = true) {
	float2 a = max(0.0f, 1.0f - float2(u, v) / c.e);
	float l = length(a);
	float hi = max(a.x, a.y);
	float rho = hi > 0.0f ? min(min(a.x, a.y) / hi, 1.0f) : 0.0f;
	float p = (((-0.926054f * rho + 3.15601f) * rho - 3.64122f) * rho + 1.26803f) * rho + 0.268531f;
	float f1 = l + 1.0f - 1.0f / (1.0f - rho * rho * min(l, 1.0f) * p);
	float f2 = length(max(0.0f, a * contExtent - (contExtent - 1.0f))) * 0.654166f + 0.345834f;
	float s = a.y > a.x ? 1.0f : -1.0f;
	float w = saturate(0.5f - s + s * rho);
	float fx = f1 + (f2 - f1) * c.cx, fy = f1 + (f2 - f1) * c.cy;
	return min(max(c.eff - u, c.eff - v), 0.0f) + c.e * (fx + (fy - fx) * w - 1.0f);
}`

func referenceCornerSource() string {
	a := strings.Index(shaderSource, "float contDist(")
	if a < 0 {
		panic("contDist missing")
	}
	z := strings.Index(shaderSource[a:], "\n}")
	if z < 0 {
		panic("contDist body missing")
	}
	z += a + 2
	return shaderSource[:a] + referenceContDist + shaderSource[z:]
}

// Compare the shortcut against the full pre-optimization formula. A single
// rounding unit is allowed in the final target; no geometry or sample changes.
func TestContinuousShadowUnclampedMatchesReference(t *testing.T) {
	oldSync := syncPipelines
	syncPipelines = true
	defer func() { syncPipelines = oldSync }()
	got, err := newRenderer()
	if err != nil {
		unavailable(t, err)
	}
	defer got.Release()
	ref, err := newRenderer()
	if err != nil {
		unavailable(t, err)
	}
	defer ref.Release()
	ref.source = referenceCornerSource()
	cases := shaderIdentityCases()
	n := len(cases)
	for _, c := range cases[:n] {
		cases = append(cases, gputest.CullCase{Name: c.Name + " continuous", Scene: gputest.Continuously(c.Scene)})
	}
	changed, maxDelta, maxULP := 0, 0, 0
	for _, wide := range []bool{false, true} {
		if wide && (!got.SetWide(true) || !ref.SetWide(true)) {
			t.Log("no wide target")
			continue
		}
		for _, c := range cases {
			a := renderRaw(t, got, c.Scene, wide)
			b := renderRaw(t, ref, c.Scene, wide)
			for i := 0; i < len(a); {
				if !wide {
					d := int(a[i]) - int(b[i])
					if d < 0 {
						d = -d
					}
					maxDelta = max(maxDelta, d)
					if d > 0 {
						changed++
					}
					if d > 1 {
						t.Fatalf("%s: channel delta %d", c.Name, d)
					}
					i++
				} else {
					x, y := binary.LittleEndian.Uint16(a[i:]), binary.LittleEndian.Uint16(b[i:])
					i += 2
					if x&0x7c00 == 0x7c00 || y&0x7c00 == 0x7c00 {
						t.Fatalf("%s: non-finite wide value %x %x", c.Name, x, y)
					}
					if x == y || half(x) == half(y) {
						continue
					}
					changed++
					if x>>15 != y>>15 {
						t.Fatalf("%s: invalid wide value %x %x", c.Name, x, y)
					}
					d := int(x) - int(y)
					if d < 0 {
						d = -d
					}
					maxULP = max(maxULP, d)
					if d > 1 {
						t.Fatalf("%s: wide value differs by %d ULP", c.Name, d)
					}
				}
			}
		}
	}
	t.Logf("%d scenes, %d different components: BGRA max delta %d, float16 max ULP %d", len(cases), changed, maxDelta, maxULP)
}
