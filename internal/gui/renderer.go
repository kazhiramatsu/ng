package gui

import (
	"github.com/Shougo/ng/internal/font"
	"github.com/Shougo/ng/internal/paint"
	"github.com/Shougo/ng/internal/screen"
)

// renderer draws the window in place of SDL_Renderer. The only one is
// ng's Metal renderer on macOS (metal_darwin.go); elsewhere, and with
// NG_RENDERER=sdl in the environment, App draws with SDL_Renderer.
type renderer interface {
	name() string
	// paintGrid paints the rows of s that changed since the last call.
	paintGrid(s *screen.Screen, f *font.Set)
	// present shows a frame: the rows painted, the images, the cursor
	// and the preedit text. It returns what DrawPreedit and CursorRect
	// return, for the input method.
	present(v view) (area paint.Rect, cursorX int)
	// resetAtlas forgets the glyphs, after a change of fonts.
	resetAtlas()
	close()
}

// view is what a frame shows besides the painted rows.
type view struct {
	scr     *screen.Screen
	fonts   *font.Set
	w, h    int // the window's size in pixels
	focused bool
	cursor  bool // draw the cursor
	preedit paint.Preedit
	// inStep presents the frame together with the window's new size,
	// during a live resize on macOS.
	inStep bool
}
