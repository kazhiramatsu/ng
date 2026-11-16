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
