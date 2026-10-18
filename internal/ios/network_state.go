package ios

import "github.com/egoist/mygo/internal/platform"

func basicNetwork(mask uint32) platform.NetworkStatus {
	return platform.NetworkStatus{Online: mask&2 != 0, Metered: mask&12 != 0}
}
func detailedNetwork(mask uint32) platform.NetworkPathStatus {
	s := platform.NetworkPathStatus{Known: mask&1 != 0, Online: mask&2 != 0, Expensive: mask&4 != 0, Constrained: mask&8 != 0}
	if !s.Known {
		return s
	}
	s.Interface = "none"
	if s.Online {
		switch {
		case mask&16 != 0:
			s.Interface = "wifi"
		case mask&32 != 0:
			s.Interface = "cellular"
		case mask&64 != 0:
			s.Interface = "wired"
		default:
			s.Interface = "other"
		}
	}
	return s
}
