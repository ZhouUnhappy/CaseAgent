package clock

import "time"

// Now returns the single application-side time baseline used for persisted data.
func Now() time.Time {
	return time.Now().UTC()
}
