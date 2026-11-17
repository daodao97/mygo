package ui

// lastScrollerForTest returns a container with a pull to refresh.
func (rt *engine) lastScrollerForTest() uint64 {
	for id, s := range rt.states {
		if s.refresh != nil {
			return id
		}
	}
	return 0
}
