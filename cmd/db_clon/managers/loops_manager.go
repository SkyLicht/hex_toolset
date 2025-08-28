package managers

import (
	"context"
	"sync"
	"time"
)

// LoopsManager runs aligned periodic tasks and supports graceful shutdown via context.
type LoopsManager struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewLoopsManager creates a new loops manager bound to a parent context.
func NewLoopsManager(parent context.Context) *LoopsManager {
	ctx, cancel := context.WithCancel(parent)
	return &LoopsManager{ctx: ctx, cancel: cancel}
}

// Stop cancels all loops and waits for them to finish.
func (lm *LoopsManager) Stop() {
	lm.cancel()
	lm.wg.Wait()
}

// StartEveryMinute runs fn every minute, aligned to next exact minute + 2s,
// and passes the minute being processed to fn (no overlap; catch-up if behind).
func (lm *LoopsManager) StartEveryMinute(fn func(context.Context, time.Time)) {
	start := nextMinutePlus(2 * time.Second)

	lm.wg.Add(1)
	go func() {
		defer lm.wg.Done()

		// wait for the first aligned tick
		if !lm.waitUntil(start) {
			return
		}

		next := start
		for {
			// Check cancellation before executing
			select {
			case <-lm.ctx.Done():
				return
			default:
			}

			// The minute to process is the previous full minute boundary
			// relative to the aligned tick time (next).
			minuteToProcess := next.Add(-time.Minute).Truncate(time.Minute)

			safeCall(func(ctx context.Context) {
				fn(ctx, minuteToProcess)
			}, lm.ctx)

			// advance schedule and keep alignment (catch-up if behind)
			next = next.Add(time.Minute)
			if !lm.waitUntil(next) {
				return
			}
		}
	}()
}

// StartEveryHour runs fn every hour exactly at hh:00:02 (e.g., 01:00:02, 14:00:02).
func (lm *LoopsManager) StartEveryHour(fn func(context.Context)) {
	start := nextHourAtSecond(2)
	lm.startAlignedPeriodic(fn, start, time.Hour)
}

// StartDailyAt runs fn every 24h at the given local time-of-day (hour:min:sec).
// Example: StartDailyAt(17, 0, 0, fn) to run daily at 17:00:00.
func (lm *LoopsManager) StartDailyAt(hour, min, sec int, fn func(context.Context)) {
	start := nextDailyAt(hour, min, sec, time.Local)
	lm.startAlignedPeriodic(fn, start, 24*time.Hour)
}

// Internal runner: waits until start, runs fn, then repeats every period.
// It maintains alignment by computing the next run from the last scheduled time.
func (lm *LoopsManager) startAlignedPeriodic(fn func(context.Context), start time.Time, period time.Duration) {
	lm.wg.Add(1)
	go func() {
		defer lm.wg.Done()

		// Initial wait until aligned start
		if !lm.waitUntil(start) {
			return
		}

		next := start
		for {
			// Check cancellation before executing
			select {
			case <-lm.ctx.Done():
				return
			default:
			}

			safeCall(fn, lm.ctx)

			// Schedule next run based on the aligned clock
			next = next.Add(period)
			if !lm.waitUntil(next) {
				return
			}
		}
	}()
}

// waitUntil sleeps until t or returns false if context is canceled.
func (lm *LoopsManager) waitUntil(t time.Time) bool {
	for {
		d := time.Until(t)
		if d <= 0 {
			return true
		}
		// Sleep in chunks to be responsive to cancellation
		chunk := d
		if chunk > time.Second {
			chunk = time.Second
		}
		select {
		case <-lm.ctx.Done():
			return false
		case <-time.After(chunk):
		}
	}
}

// Helpers for alignment

func nextMinutePlus(extra time.Duration) time.Time {
	now := time.Now()
	t := now.Truncate(time.Minute).Add(time.Minute).Add(extra)
	if !t.After(now) {
		t = t.Add(time.Minute)
	}
	return t
}

// nextHourAtSecond returns the next occurrence of the top-of-hour at :second.
func nextHourAtSecond(second int) time.Time {
	if second < 0 || second > 59 {
		second = 2
	}
	now := time.Now()
	aligned := now.Truncate(time.Hour).Add(time.Hour).Add(time.Duration(second) * time.Second)
	if !aligned.After(now) {
		aligned = aligned.Add(time.Hour)
	}
	return aligned
}

func nextDailyAt(hour, min, sec int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, min, sec, 0, loc)
	if !t.After(now) {
		t = t.Add(24 * time.Hour)
	}
	return t
}

// safeCall runs fn with recover protection.
func safeCall(fn func(context.Context), ctx context.Context) {
	defer func() { _ = recover() }()
	fn(ctx)
}
