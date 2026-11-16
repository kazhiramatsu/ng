package gui

/*
// Link the Objective-C runtime explicitly: with Go 1.27, nothing else
// brought in objc_msgSend and the link failed.
#cgo LDFLAGS: -lobjc
#include <objc/message.h>
#include "sdl.h"
#include "metal.h"

// ng_low_latency makes Metal keep two drawables instead of three. With
// three, the window server showed a frame two refreshes after it was
// presented (16 to 25 ms at 120 Hz, by the drawable's presentedTime), and
// handing out the next drawable blocked whenever frames came faster than
// the display. With two it takes a frame at once (docs/macos.md,
// "Latency"). A no-op with other renderers.
static void ng_low_latency(SDL_Renderer *r) {
	// The CAMetalLayer, set through the Objective-C runtime from C:
	// layer.maximumDrawableCount = 2.
	id layer = (id)SDL_GetRenderMetalLayer(r);
	if (layer)
		((void (*)(id, SEL, unsigned long))objc_msgSend)(layer, sel_registerName("setMaximumDrawableCount:"), 2);
}
*/
import "C"

import (
	"os"
	"time"
)

// SDL_Renderer draws with Metal on macOS, where two of its habits cost
// latency or left a stale frame on the screen. renderer_linux.go has the
// same functions, doing nothing.

// setUpRenderer is called once the renderer is made: it makes Metal keep
// two drawables (ng_low_latency).
func setUpRenderer(r *C.SDL_Renderer) { C.ng_low_latency(r) }

// representDelay is how long after a change of the window size ng presents
// the window again. When a live resize (the user dragging the window's
// edge) ended, macOS did not show the frames presented in the next 16 ms
// or so, and kept showing the old frame stretched to the new size until
// something else was drawn, such as a key. Frames from 50 ms on were shown
// (docs/macos.md, "Resizing").
const representDelay = 100 * time.Millisecond

// afterResize is called after the window size changed: it presents the
// window again representDelay from now.
func (a *App) afterResize() {
	time.AfterFunc(representDelay, func() { a.post(a.present) })
}

// SDL_Renderer draws only when asked for, or when Metal is not available:
// ng draws with a Metal renderer of its own (metal_darwin.go).
// renderer_linux.go has the same two functions, for SDL_Renderer.

// sdlRenderer reports whether the environment asks for SDL_Renderer
// instead of ng's Metal renderer, with NG_RENDERER=sdl.
func sdlRenderer() bool { return os.Getenv("NG_RENDERER") == "sdl" }

// createWindow creates the hidden window, for Metal unless SDL_Renderer is
// to draw.
func (a *App) createWindow(title *C.char, w, h C.int) *C.SDL_Window {
	if sdlRenderer() {
		return C.ng_create_window(title, w, h)
	}
	return C.ng_metal_create_window(title, w, h)
}

// ownRenderer returns ng's Metal renderer for the window, or nil when
// SDL_Renderer is to draw: it was asked for, or Metal is not available.
func (a *App) ownRenderer() renderer {
	if sdlRenderer() {
		return nil
	}
	m := newMetal(a.win, metalAtlasSize, metalAtlasMaxSize)
	if m == nil {
		return nil
	}
	live = a
	C.ng_metal_watch_live_resize(m.m)
	return m
}
