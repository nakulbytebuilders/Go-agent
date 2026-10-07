package input

import (
	"math/rand"
	"testing"
	"time"

	"github.com/monitoring-agent/agent/internal/models"
)

var t0 = time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)

// run feeds the tracker one observation per second from `from` (exclusive) up
// to and including `to`, returning every period it reported.
func run(p *presenceTracker, from, to time.Time, locked bool) []models.IdlePeriod {
	var out []models.IdlePeriod
	for now := from.Add(time.Second); !now.After(to); now = now.Add(time.Second) {
		out = append(out, p.observe(now, locked)...)
	}
	return out
}

func last(periods []models.IdlePeriod) models.IdlePeriod {
	return periods[len(periods)-1]
}

func TestNoIdleBelowThreshold(t *testing.T) {
	p := newPresenceTracker(IdleThreshold, t0)

	if got := run(p, t0, t0.Add(IdleThreshold-time.Second), false); len(got) != 0 {
		t.Fatalf("expected no periods before the threshold, got %+v", got)
	}
}

func TestIdleStartsAtLastInputOnceThresholdIsCrossed(t *testing.T) {
	p := newPresenceTracker(IdleThreshold, t0)
	lastInput := t0.Add(10 * time.Second)
	p.noteInput(lastInput)

	got := run(p, t0, lastInput.Add(IdleThreshold), false)

	if len(got) != 1 {
		t.Fatalf("expected exactly one report at the crossing, got %+v", got)
	}
	if got[0].Name != PeriodIdle || !got[0].StartTime.Equal(lastInput) {
		t.Fatalf("idle must start at the last input, got %+v", got[0])
	}
	if got[0].EndTime.Sub(got[0].StartTime) < IdleThreshold {
		t.Fatalf("first report must already cover the threshold, got %+v", got[0])
	}
}

func TestIdleIsReReportedWhileOpenAndClosedWhenInputResumes(t *testing.T) {
	p := newPresenceTracker(IdleThreshold, t0)

	crossing := t0.Add(IdleThreshold)
	run(p, t0, crossing, false)

	progress := run(p, crossing, crossing.Add(3*progressInterval), false)
	if len(progress) != 3 {
		t.Fatalf("expected one progress report per interval, got %d: %+v", len(progress), progress)
	}
	for _, period := range progress {
		if !period.StartTime.Equal(t0) {
			t.Fatalf("progress reports must keep the same start_time, got %+v", period)
		}
	}

	resumed := crossing.Add(3*progressInterval + 20*time.Second)
	p.noteInput(resumed)
	closing := p.observe(resumed.Add(time.Second), false)

	if len(closing) != 1 || !closing[0].StartTime.Equal(t0) || !closing[0].EndTime.Equal(resumed) {
		t.Fatalf("idle must close at the moment input resumed, got %+v", closing)
	}
	if got := run(p, resumed.Add(time.Second), resumed.Add(time.Minute), false); len(got) != 0 {
		t.Fatalf("nothing more expected once the user is back, got %+v", got)
	}
}

func TestLockReportsAtTransitionMoment(t *testing.T) {
	p := newPresenceTracker(IdleThreshold, t0)
	lockAt := t0.Add(30 * time.Second)

	got := p.observe(lockAt, true)
	if len(got) != 1 || got[0].Name != PeriodLocked || !got[0].StartTime.Equal(lockAt) || !got[0].EndTime.Equal(lockAt) {
		t.Fatalf("away must be reported at the exact lock timestamp, got %+v", got)
	}
}

func TestLockClosesIdleAndIsReportedAsAway(t *testing.T) {
	p := newPresenceTracker(IdleThreshold, t0)
	run(p, t0, t0.Add(5*time.Minute), false) // idle since t0

	lockAt := t0.Add(5*time.Minute + time.Second)
	got := p.observe(lockAt, true)

	if len(got) != 2 {
		t.Fatalf("idle close and immediate lock start should both be reported, got %+v", got)
	}
	if got[0].Name != PeriodIdle || !got[0].EndTime.Equal(lockAt) {
		t.Fatalf("idle must end at lock time, got %+v", got)
	}
	if got[1].Name != PeriodLocked || !got[1].StartTime.Equal(lockAt) || !got[1].EndTime.Equal(lockAt) {
		t.Fatalf("lock must start immediately at the transition, got %+v", got)
	}

	locked := run(p, lockAt, lockAt.Add(10*time.Minute), true)
	if len(locked) == 0 {
		t.Fatal("expected away progress reports while locked")
	}
	for _, period := range locked {
		if period.Name != PeriodLocked || !period.StartTime.Equal(lockAt) {
			t.Fatalf("only locked periods starting at the lock are expected, got %+v", period)
		}
	}
}

func TestUnlockClosesAwayAndRestartsIdleClock(t *testing.T) {
	p := newPresenceTracker(IdleThreshold, t0)
	lockAt := t0.Add(30 * time.Second)
	run(p, t0, lockAt.Add(-time.Second), false)
	p.observe(lockAt, true)

	unlockAt := lockAt.Add(time.Hour)
	run(p, lockAt, unlockAt.Add(-time.Second), true)
	closing := p.observe(unlockAt, false)

	if len(closing) != 1 || closing[0].Name != PeriodLocked ||
		!closing[0].StartTime.Equal(lockAt) || !closing[0].EndTime.Equal(unlockAt) {
		t.Fatalf("away must span the whole lock, got %+v", closing)
	}

	// The hour spent locked must not turn into idle time on unlock.
	if got := run(p, unlockAt, unlockAt.Add(IdleThreshold-time.Second), false); len(got) != 0 {
		t.Fatalf("idle must not start before the threshold after unlock, got %+v", got)
	}
	got := run(p, unlockAt.Add(IdleThreshold-time.Second), unlockAt.Add(IdleThreshold), false)
	if len(got) != 1 || !got[0].StartTime.Equal(unlockAt) {
		t.Fatalf("idle must start at the unlock time, got %+v", got)
	}
}

func TestSleepCutsIdleShortButKeepsLockedOpen(t *testing.T) {
	// Idle across a sleep: only the time before the machine went to sleep counts.
	p := newPresenceTracker(IdleThreshold, t0)
	run(p, t0, t0.Add(5*time.Minute), false)
	wake := t0.Add(10 * time.Hour)

	got := p.observe(wake, false)

	if len(got) != 1 || got[0].Name != PeriodIdle || !got[0].EndTime.Equal(t0.Add(5*time.Minute)) {
		t.Fatalf("idle must end at the last observation before sleep, got %+v", got)
	}

	// Locked across a sleep: the machine was still away, so the period stays open.
	p = newPresenceTracker(IdleThreshold, t0)
	run(p, t0, t0.Add(10*time.Second), true)
	got = p.observe(wake, true)

	if len(got) != 1 || got[0].Name != PeriodLocked || !got[0].EndTime.Equal(wake) {
		t.Fatalf("locked period must extend across the sleep, got %+v", got)
	}
}

func TestOSIdleCounterJitterDoesNotFragmentIdle(t *testing.T) {
	// Linux/macOS derive the last-input time as now minus a whole-second idle
	// counter, so the derived time lands anywhere within a second of the real
	// one and keeps inching past the value idle started from. That must not
	// read as the user returning.
	p := newPresenceTracker(IdleThreshold, t0)
	rng := rand.New(rand.NewSource(1))
	var idleStart time.Time

	for i := 1; i <= 900; i++ {
		now := t0.Add(time.Duration(i) * time.Second)
		p.noteInput(t0.Add(time.Duration(rng.Float64() * float64(time.Second))))
		p.observe(now, false)

		if idleStart.IsZero() {
			idleStart = p.idleStart // still zero until idle begins
			continue
		}
		if !p.idle || !p.idleStart.Equal(idleStart) {
			t.Fatalf("idle was closed or restarted at observation %d (idle=%v, start moved %s)",
				i, p.idle, p.idleStart.Sub(idleStart))
		}
	}

	if idleStart.IsZero() {
		t.Fatal("idle never began")
	}
}

func TestCloseEndsOpenPeriods(t *testing.T) {
	p := newPresenceTracker(IdleThreshold, t0)
	run(p, t0, t0.Add(3*time.Minute), false)

	end := t0.Add(3*time.Minute + 30*time.Second)
	got := p.close(end)

	if len(got) != 1 || got[0].Name != PeriodIdle || !got[0].EndTime.Equal(end) {
		t.Fatalf("shutdown must close the open idle period, got %+v", got)
	}
	if again := p.close(end.Add(time.Minute)); len(again) != 0 {
		t.Fatalf("closing twice must report nothing, got %+v", again)
	}
}
