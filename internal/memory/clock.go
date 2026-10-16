package memory

import "time"

func bumped(previous time.Time) time.Time {
	modified := now()
	if !modified.After(previous) {
		modified = previous.Add(time.Microsecond)
	}
	return modified
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}
