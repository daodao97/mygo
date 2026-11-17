package wire

import (
	"slices"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	m := CreateWindow{ID: 3, Surface: true, Background: 0xff8000ff, Schemes: []string{"mygo", "app"},
		UserAgent: "ünïcode", Inspectable: true}
	var w Writer
	m.Encode(&w)
	r := Reader{Buf: w.Buf}
	if op := r.Uvarint(); op != CallCreateWindow {
		t.Fatalf("op = %d", op)
	}
	got := CreateWindow{ID: r.Int(), Surface: r.Bool(), OwnScene: r.Bool(), Background: r.Int(), HasBackground: r.Bool(), Schemes: r.Strings(),
		Scripts: r.Strings(), ScriptFlags: r.Strings(), UserAgent: r.String(), UserAgentSuffix: r.String(), Inspectable: r.Bool()}
	if r.Err != nil || got.ID != 3 || !got.Surface || got.Background != 0xff8000ff || !slices.Equal(got.Schemes, m.Schemes) ||
		got.UserAgent != m.UserAgent || !got.Inspectable {
		t.Errorf("decoded %+v (%v)", got, r.Err)
	}
	var f Writer
	f.Float(-1.5)
	f.Int(-7)
	fr := Reader{Buf: f.Buf}
	if v, n := fr.Float(), fr.Int(); v != -1.5 || n != -7 {
		t.Errorf("Float, Int = %v, %v", v, n)
	}
}

// A message cut short reads as zeros and reports it.
func TestShort(t *testing.T) {
	r := Reader{Buf: []byte{5}}
	if s := r.String(); s != "" || r.Err != ErrShort {
		t.Errorf("String = %q, %v", s, r.Err)
	}
}
