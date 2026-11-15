//go:build !darwin

package gui

// ownRenderer returns nil: SDL_Renderer draws.
func (a *App) ownRenderer() renderer { return nil }
