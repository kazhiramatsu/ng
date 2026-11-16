package gui

// #include "sdl.h"
import "C"

// SDL_Renderer needs no help on Linux. These are in a file for Linux so
// that a macOS fork can define its own in renderer_darwin.go, for what
// macOS needs.

// setUpRenderer is called once the renderer is made.
func setUpRenderer(*C.SDL_Renderer) {}

// afterResize is called after the window size changed.
func (a *App) afterResize() {}

// createWindow creates the hidden window.
func (a *App) createWindow(title *C.char, w, h C.int) *C.SDL_Window {
	return C.ng_create_window(title, w, h)
}

// ownRenderer returns nil: SDL_Renderer draws.
func (a *App) ownRenderer() renderer { return nil }
