package accounting

// Entry is one reporting interval of raw proxy traffic for one panel user.
type Entry struct {
	UserID   int64
	Uplink   int64
	Downlink int64
}
