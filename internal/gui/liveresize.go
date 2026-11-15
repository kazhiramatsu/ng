package gui

import "time"

// representDelay is how long after a change of the window size ng presents
// the window again on macOS. When a live resize (the user dragging the
// window's edge) ended, macOS did not show the frames presented in the
// next 16 ms or so, and kept showing the old frame stretched to the new
// size until something else was drawn, such as a key. Frames from 50 ms
// on were shown (docs/macos.md, "Resizing").
const representDelay = 100 * time.Millisecond

// presentAgainLater presents the window again representDelay from now,
// on macOS.
func (a *App) presentAgainLater() {
	if macOS {
		time.AfterFunc(representDelay, func() { a.post(a.present) })
	}
}
