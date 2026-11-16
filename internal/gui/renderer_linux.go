package gui

// #include "sdl.h"
import "C"

// SDL_Renderer needs no changes on Linux (see renderer_darwin.go).

func lowLatency(*C.SDL_Renderer) {}

func (a *App) presentAgainLater() {}

// createWindow creates the hidden window.
func (a *App) createWindow(title *C.char, w, h C.int) *C.SDL_Window {
	return C.ng_create_window(title, w, h)
}

// ownRenderer returns nil: SDL_Renderer draws.
func (a *App) ownRenderer() renderer { return nil }
