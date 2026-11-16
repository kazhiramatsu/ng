package gui

// #include "sdl.h"
import "C"

// ng draws with SDL_Renderer on Linux, which needs no help there. These
// are in a file for Linux so that a fork can define its own in files for
// another system, such as macOS.

// createWindow creates the hidden window.
func (a *App) createWindow(title *C.char, w, h C.int) *C.SDL_Window {
	return C.ng_create_window(title, w, h)
}

// ownRenderer returns nil: SDL_Renderer draws, not a renderer of ng's own
// (renderer.go).
func (a *App) ownRenderer() renderer { return nil }

// setUpRenderer is called once SDL_Renderer is made.
func setUpRenderer(*C.SDL_Renderer) {}

// afterResize is called after the window size changed.
func (a *App) afterResize() {}
