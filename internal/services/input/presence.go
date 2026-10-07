package input

import (
	"time"

	"github.com/monitoring-agent/agent/internal/models"
)

const (
	// IdleThreshold is how long the user can go without a click or keystroke
	// before that stretch counts as idle (neutral hours on the dashboard).
	IdleThreshold = 2 * time.Minute

	// Period names understood by the cloud backend.
	PeriodIdle   = "idle"
	PeriodLocked = "locked"

	// presenceTick is how often idle/locked state is evaluated.
	presenceTick = time.Second

	// progressInterval is how often a still-open period is re-reported with an
	// extended end time, so a crash or hard shutdown loses at most this much.
	progressInterval = time.Minute

	// suspendGap is the largest gap between two observations that is treated as
	// normal. Anything longer means the machine slept or the process froze.
	suspendGap = 30 * time.Second

	// inputJitter absorbs the small error in last-input times derived from an
	// OS idle counter, so it is not mistaken for the user coming back.
	inputJitter = 5 * time.Second
)

// presenceTracker turns observations (time of the last click/keystroke, screen
// locked or not) into idle and locked periods. It does no I/O and takes the
// clock as an argument, so it can be tested without sleeping.
//
// Idle periods start at the last click/keystroke, not when the threshold is
// crossed, so the whole no-input stretch counts once it exceeds the threshold.
// Idle is never tracked while the screen is locked: that time is away.
//
// A period is reported while it is still open (see progressInterval) and once
// more when it closes. The backend upserts on (name, start_time), so repeated
// reports only ever extend the period.
type presenceTracker struct {
	threshold time.Duration

	lastTick  time.Time
	lastInput time.Time

	idle      bool
	idleStart time.Time
	idleSent  time.Time

	locked    bool
	lockStart time.Time
	lockSent  time.Time
}

func newPresenceTracker(threshold time.Duration, now time.Time) *presenceTracker {
	return &presenceTracker{threshold: threshold, lastTick: now, lastInput: now}
}

// noteInput records the time of a click or keystroke. It never moves backwards.
func (p *presenceTracker) noteInput(t time.Time) {
	if t.After(p.lastInput) {
		p.lastInput = t
	}
}

// observe advances the tracker to now and returns the periods to report.
func (p *presenceTracker) observe(now time.Time, locked bool) []models.IdlePeriod {
	var out []models.IdlePeriod

	// A long gap means the machine slept (or the process froze). Idle time must
	// not stretch across it, and coming back counts as the user being present.
	// A locked period is left open on purpose: a machine that slept while
	// locked was still away.
	if now.Sub(p.lastTick) > suspendGap {
		out = p.closeIdle(out, p.lastTick)
		p.noteInput(now)
	}
	p.lastTick = now

	if locked && !p.locked {
		out = p.closeIdle(out, now)
		p.locked, p.lockStart, p.lockSent = true, now, now
		out = appendPeriod(out, PeriodLocked, p.lockStart, now)
	} else if !locked && p.locked {
		out = p.closeLocked(out, now)
		p.noteInput(now) // unlocking is the user coming back: restart the idle clock
	}

	if !p.locked {
		if p.idle && p.lastInput.Sub(p.idleStart) > inputJitter {
			out = p.closeIdle(out, p.lastInput)
		}
		if !p.idle && now.Sub(p.lastInput) >= p.threshold {
			p.idle, p.idleStart, p.idleSent = true, p.lastInput, time.Time{}
		}
	}

	if p.idle && (p.idleSent.IsZero() || now.Sub(p.idleSent) >= progressInterval) {
		out = appendPeriod(out, PeriodIdle, p.idleStart, now)
		p.idleSent = now
	}
	if p.locked && (p.lockSent.IsZero() || now.Sub(p.lockSent) >= progressInterval) {
		out = appendPeriod(out, PeriodLocked, p.lockStart, now)
		p.lockSent = now
	}

	return out
}

// close ends any open period at now (service shutdown).
func (p *presenceTracker) close(now time.Time) []models.IdlePeriod {
	out := p.closeIdle(nil, now)
	return p.closeLocked(out, now)
}

func (p *presenceTracker) closeIdle(out []models.IdlePeriod, end time.Time) []models.IdlePeriod {
	if !p.idle {
		return out
	}
	out = appendPeriod(out, PeriodIdle, p.idleStart, end)
	p.idle, p.idleStart, p.idleSent = false, time.Time{}, time.Time{}
	return out
}

func (p *presenceTracker) closeLocked(out []models.IdlePeriod, end time.Time) []models.IdlePeriod {
	if !p.locked {
		return out
	}
	out = appendPeriod(out, PeriodLocked, p.lockStart, end)
	p.locked, p.lockStart, p.lockSent = false, time.Time{}, time.Time{}
	return out
}

// appendPeriod adds a period at whole-second precision, skipping empty ones.
func appendPeriod(out []models.IdlePeriod, name string, start, end time.Time) []models.IdlePeriod {
	period := models.IdlePeriod{
		Name:      name,
		StartTime: start.Truncate(time.Second),
		EndTime:   end.Truncate(time.Second),
	}
	if period.StartTime.IsZero() {
		return out
	}
	if period.EndTime.Before(period.StartTime) {
		return out
	}
	return append(out, period)
}
