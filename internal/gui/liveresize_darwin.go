package gui

/*
#include <unistd.h>
#include "sdl.h"
*/
import "C"

import "time"

// live is the App that draws during a live resize (see liveResize).
var live *App

// liveWait is how long a new window size waits for Neovim to redraw at
// that size before a frame is drawn, during a live resize. Neovim took
// less than 1 ms (docs/macos.md, "Metal renderer").
const liveWait = 16 * time.Millisecond

//export ngLiveResize
func ngLiveResize(sized C.int) {
	if live != nil {
		live.liveResize(sized != 0)
	}
}

// liveResize draws a frame during a live resize on macOS: the user drags
// the window's edge, or macOS animates the window's frame. SDL_WaitEvent
// does not return until that ends, so this is called from inside it (see
// live_resize_watch in metal_darwin.m), on the main thread, between two
// events of the main loop: with sized for each new size of the window, and
// 60 times a second while the user drags.
//
// A frame for a new size is presented with the window's new frame, in one
// Core Animation transaction, so the window never shows a frame of another
// size. It first waits for Neovim's redraw at the new size, up to
// liveWait; later redraws are shown by the next call.
func (a *App) liveResize(sized bool) {
	if a.inLive || a.own == nil || !a.shown || a.fonts == nil {
		return
	}
	a.inLive = true // not again from what this calls (SDL_SetWindowSize)
	defer func() { a.inLive = false }()
	var w, h C.int
	C.SDL_GetWindowSizeInPixels(a.win, &w, &h)
	if int(w) != a.pixW || int(h) != a.pixH {
		a.updatePixelSize(false) // asks Neovim for the new grid size
		if a.trace != nil {
			a.tracef(time.Now(), "window  %dx%d px (live resize)", a.pixW, a.pixH)
		}
	}
	deadline := time.Now().Add(liveWait)
	waiting, any := false, a.drawn
	for {
		// Neovim's answers so far. The main loop sees a closed
		// connection itself; an error must stop it.
		drawn, _, err := a.processRedraws()
		if err != nil {
			a.fatal = err
			return
		}
		any = any || drawn
		fits := a.scr.Width == a.gridCols && a.scr.Height == a.gridRows
		waiting = waiting || !fits
		if !sized || !a.attached || !waiting || fits && drawn || !time.Now().Before(deadline) {
			break
		}
		// Not time.Sleep: in this callback from C it returned 14 ms
		// later, not 0.25 ms.
		C.usleep(200)
	}
	if !sized && !any {
		return // nothing new to show
	}
	if sized && a.trace != nil {
		a.tracef(time.Now(), "live    grid %dx%d of %dx%d after %.2f ms",
			a.scr.Width, a.scr.Height, a.gridCols, a.gridRows, msf(time.Since(deadline.Add(-liveWait))))
	}
	a.paintDirty()
	a.drawn, a.firstFlush, a.final = false, time.Time{}, false
	a.present()
}
