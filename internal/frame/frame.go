// Package frame defines when ng presents a frame after Neovim flushes.
//
// Neovim often sends several flushes for one change. With 'showcmd' a
// Normal mode command first flushes the command shown in the corner, then
// its result; while command line completion is updated, one flush redraws
// the popup menu and blanks the command line row, and the next one redraws
// the command line. A terminal hides most such intermediate screens because
// it repaints at its refresh rate, merging updates that arrive within one
// frame.
//
// Neovim tells ng when nothing is pending any more and it will wait for
// input (SafeState, see nvim.IdleNotification). The flush that follows is
// the last one for what was typed: it is final, and ng presents it at once.
// Without that (an operator or a mapping is pending, completion is active,
// Neovim is still busy), ng waits until Neovim has been quiet for Quiet
// before presenting, and never longer than MaxDelay (one 60 Hz frame)
// after the first flush.
package frame

import "time"

const (
	Quiet    = 4 * time.Millisecond
	MaxDelay = 16 * time.Millisecond
)

// Due returns when to present a frame that shows the flushes that arrived
// from first to last: at once if last is final, else when Neovim has been
// quiet for Quiet, but no later than MaxDelay after first.
func Due(first, last time.Time, final bool) time.Time {
	if final {
		return last
	}
	quiet, latest := last.Add(Quiet), first.Add(MaxDelay)
	if latest.Before(quiet) {
		return latest
	}
	return quiet
}

// Flush is the arrival of a flush, final or not.
type Flush struct {
	At    time.Duration
	Final bool
}

// Schedule simulates the policy: given flushes (sorted by arrival), it
// returns the times frames are presented. Each frame shows every flush
// that arrived before it. It ignores the time spent painting.
func Schedule(flushes []Flush) []time.Duration {
	var base time.Time // the arrival times are offsets from it
	var out []time.Duration
	for i := 0; i < len(flushes); {
		first, last := flushes[i], flushes[i]
		for i++; i < len(flushes); i++ {
			due := Due(base.Add(first.At), base.Add(last.At), last.Final)
			if !base.Add(flushes[i].At).Before(due) {
				break // presented before this flush arrives
			}
			last = flushes[i]
		}
		out = append(out, Due(base.Add(first.At), base.Add(last.At), last.Final).Sub(base))
	}
	return out
}
