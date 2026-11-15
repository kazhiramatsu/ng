package gui

import (
	"testing"
	"time"

	"github.com/Shougo/ng/internal/frame"
)

// A frame drawn without a flush waits for quiet; one with a final flush
// is due at once.
func TestFrameDue(t *testing.T) {
	now := time.Now()
	a := &App{}
	if got := a.frameDue(now); !got.Equal(now.Add(frame.Quiet)) {
		t.Errorf("no flush: due %v after now, want %v", got.Sub(now), frame.Quiet)
	}
	// Asking again later does not move it.
	if got := a.frameDue(now.Add(time.Millisecond)); !got.Equal(now.Add(frame.Quiet)) {
		t.Errorf("no flush, asked again: due %v after now, want %v", got.Sub(now), frame.Quiet)
	}
	flush := now.Add(-time.Millisecond)
	a = &App{firstFlush: flush, lastFlush: flush, final: true}
	if got := a.frameDue(now); !got.Equal(flush) {
		t.Errorf("final flush: due %v after it, want 0", got.Sub(flush))
	}
}
