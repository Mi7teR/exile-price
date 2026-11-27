package main

import "time"

// hideGrace is how long the line stays after the cursor moves away from it.
const hideGrace = 2 * time.Second

// nextHide returns when the line shown until hideAt hides, given that the
// cursor has moved away, and whether that time has come.
func nextHide(hideAt, now time.Time, movedAway bool) (time.Time, bool) {
	if movedAway && hideAt.After(now.Add(hideGrace)) {
		hideAt = now.Add(hideGrace)
	}
	return hideAt, now.After(hideAt)
}
