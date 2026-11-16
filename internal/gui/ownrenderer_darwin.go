package gui

// #include "sdl.h"
import "C"

// On macOS, too, SDL_Renderer draws here, in a window made as on Linux.
// The branch metal-renderer draws with a Metal renderer of ng's own
// instead: it replaces this file with metal_darwin.go.

// createWindow creates the hidden window.
func (a *App) createWindow(title *C.char, w, h C.int) *C.SDL_Window {
	return C.ng_create_window(title, w, h)
}

// ownRenderer returns nil: SDL_Renderer draws.
func (a *App) ownRenderer() renderer { return nil }
