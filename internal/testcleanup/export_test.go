package testcleanup

import "time"

// SetRoundDelay sets how long Run waits before rounds 2 and 3, and returns a function that
// restores the previous wait.
func SetRoundDelay(d time.Duration) (restore func()) {
	previous := roundDelay
	roundDelay = d
	return func() { roundDelay = previous }
}

// SetMaxPages sets how many pages of a list Run reads before it stops, and returns a function
// that restores the previous limit.
func SetMaxPages(n int) (restore func()) {
	previous := maxPages
	maxPages = n
	return func() { maxPages = previous }
}
