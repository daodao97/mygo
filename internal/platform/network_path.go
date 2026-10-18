package platform

type NetworkPathStatus struct {
	Known  bool `json:"known"`
	Online bool `json:"online"`
	// Interface is wifi, cellular, wired, other, none, or empty when unknown.
	Interface   string `json:"interface"`
	Expensive   bool   `json:"expensive"`
	Constrained bool   `json:"constrained"`
}

// Optional detailed path source. Methods and callbacks run on the UI thread.
type NetworkPathSource interface {
	PathStatus() NetworkPathStatus
	WatchPath(func(NetworkPathStatus)) func()
}
