//go:build !darwin

package gui

/*
#include <stdlib.h>
#include "sdl.h"
*/
import "C"

import "unsafe"

// createWindow creates the hidden window.
func (a *App) createWindow() *C.SDL_Window {
	title := C.CString("ng")
	defer C.free(unsafe.Pointer(title))
	return C.ng_create_window(title, 800, 600)
}

// ownRenderer returns nil: SDL_Renderer draws.
func (a *App) ownRenderer() renderer { return nil }
