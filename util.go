package pushq

import "time"

const (
	// EnqCt is the counter name for Enqueued task
	EnqCt = "Enqueue"
)

// getTodayf formats "now" as a string for use in storing counters
func getTodayf(now time.Time) string {

	localt := now
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		localt = now.In(loc)
	}
	return localt.Format(ISO8601D)
}
