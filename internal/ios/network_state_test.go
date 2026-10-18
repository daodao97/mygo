package ios

import "testing"

func TestNetworkStates(t *testing.T) {
	for _, tc := range []struct {
		mask                         uint32
		known, online, cost, limited bool
		iface                        string
	}{
		{0, false, false, false, false, ""}, {1, true, false, false, false, "none"}, {19, true, true, false, false, "wifi"}, {39, true, true, true, false, "cellular"}, {27, true, true, false, true, "wifi"}, {67, true, true, false, false, "wired"}, {3, true, true, false, false, "other"},
	} {
		s := detailedNetwork(tc.mask)
		if s.Known != tc.known || s.Online != tc.online || s.Expensive != tc.cost || s.Constrained != tc.limited || s.Interface != tc.iface {
			t.Fatal(tc, s)
		}
		b := basicNetwork(tc.mask)
		if b.Online != s.Online || b.Metered != (s.Expensive || s.Constrained) {
			t.Fatal(b, s)
		}
	}
}
