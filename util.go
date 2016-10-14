package pushq

import (
	"fmt"
	"time"
)

const (
	// EnqCt is the counter name for Enqueued task
	EnqCt = "Enqueue"

	// ErrCt is the counter name for errors
	ErrCt = "Error"

	// AvgTotalCt is the counter name for average totals
	AvgTotalCt = "AvgTotal"

	// AvgAccumCt is the counter name for average accumulators
	AvgAccumCt = "AvgAccum"
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

func fmtms(ms float32) string {
	return fmt.Sprintf("%.2f", ms)
}
