package gui

// #include "sdl.h"
import "C"

// SDL_Renderer needs no changes on Linux (see renderer_darwin.go).

func lowLatency(*C.SDL_Renderer) {}

func (a *App) presentAgainLater() {}
