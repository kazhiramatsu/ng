// Package paint turns screen state into drawing operations. It is shared by
// the SDL renderer and the image renderer used in tests, so what the tests
// see is exactly what the window shows.
package paint

import (
	"github.com/Shougo/ng/internal/font"
	"github.com/Shougo/ng/internal/screen"
)

// Painter receives drawing operations in pixel coordinates.
type Painter interface {
	FillRect(x, y, w, h int, c screen.Color)
	// DrawGlyph blends a coverage bitmap whose top-left corner is (x, y).
	DrawGlyph(g *font.Glyph, x, y int, c screen.Color)
}

func styleOf(a screen.Attr) font.Style {
	var s font.Style
	if a.Bold {
		s |= font.Bold
	}
	if a.Italic {
		s |= font.Italic
	}
	return s
}

// Row paints one whole grid row: backgrounds, text and decorations. Rows
// are always painted whole; see screen.TakeDirty.
func Row(p Painter, s *screen.Screen, f *font.Set, row int) {
	y := row * f.CellH
	cells := s.Row(row)

	// Backgrounds, merged into runs of equal color.
	start := 0
	_, runBG, _ := s.Colors(cells[0].HL)
	for col := 1; col <= len(cells); col++ {
		var bg screen.Color
		if col < len(cells) {
			_, bg, _ = s.Colors(cells[col].HL)
			if bg == runBG {
				continue
			}
		}
		p.FillRect(start*f.CellW, y, (col-start)*f.CellW, f.CellH, runBG)
		start, runBG = col, bg
	}

	for col, c := range cells {
		w := s.CellWidth(row, col)
		if w == 0 {
			continue
		}
		a := s.Attr(c.HL)
		fg, _, _ := s.Colors(c.HL)
		x := col * f.CellW
		if g := glyph(s, f, row, col, w, styleOf(a)); !g.Empty() {
			p.DrawGlyph(g, x+g.X, y+g.Y, fg)
		}
		decorate(p, f, a, x, y, w*f.CellW, fg)
	}
}

// glyph returns the bitmap for the cell at (row, col), w cells wide. A
// one-cell symbol drawn wider than its cell (a Nerd Font icon) keeps its
// own size and overflows into the next cell when that cell is blank, as in
// a terminal; otherwise it is shrunk into its cell.
func glyph(s *screen.Screen, f *font.Set, row, col, w int, style font.Style) *font.Glyph {
	g, _ := spillGlyph(s, f, row, col, w, style)
	return g
}

// spillGlyph is glyph, also reporting how many cells the bitmap covers.
func spillGlyph(s *screen.Screen, f *font.Set, row, col, w int, style font.Style) (*font.Glyph, int) {
	text := s.Cell(row, col).Text
	g := f.Glyph(text, style, w)
	if g.Wide && col+1 < s.Width && s.CellWidth(row, col+1) == 1 && blank(s.Cell(row, col+1).Text) {
		return f.Spill(text, style), 2
	}
	return g, w
}

func blank(text string) bool { return text == "" || text == " " }

// decorate draws underlines and strikethrough inside the cell box.
func decorate(p Painter, f *font.Set, a screen.Attr, x, y, w int, fg screen.Color) {
	if !(a.Underline || a.Undercurl || a.Underdouble || a.Underdotted ||
		a.Underdashed || a.Strikethrough) {
		return
	}
	sp := a.SP
	if sp == screen.Default {
		sp = fg
	}
	t := max(1, f.CellH/16)
	uy := min(f.Baseline+max(1, f.CellH/14), f.CellH-t)
	switch {
	case a.Underline:
		p.FillRect(x, y+uy, w, t, sp)
	case a.Underdouble:
		p.FillRect(x, y+max(0, uy-t-1), w, t, sp)
		p.FillRect(x, y+uy, w, t, sp)
	case a.Undercurl:
		// A zigzag two thicknesses tall, ending at the underline position.
		amp := 2 * t
		top := min(uy, f.CellH-amp)
		step := max(2, f.CellW/4)
		for i := 0; i < w; i += step {
			dy := 0
			if (i/step)%2 == 1 {
				dy = amp - t
			}
			p.FillRect(x+i, y+top+dy, min(step, w-i), t, sp)
		}
	case a.Underdotted:
		for i := 0; i < w; i += 2 * t {
			p.FillRect(x+i, y+uy, t, t, sp)
		}
	case a.Underdashed:
		dash := max(2, f.CellW/2)
		for i := 0; i < w; i += 2 * dash {
			p.FillRect(x+i, y+uy, min(dash, w-i), t, sp)
		}
	}
	if a.Strikethrough {
		p.FillRect(x, y+f.Baseline*2/3, w, t, fg)
	}
}

// Mover is a painter that keeps what it painted and can move part of it.
type Mover interface {
	// MoveRect moves the w x h area at (x, y) by dy pixels down (up when
	// dy is negative). What it leaves behind is repainted afterwards.
	MoveRect(x, y, w, h, dy int)
}

// Scroll moves painted rows the way a grid_scroll moved their cells. Rows
// are painted independently of each other (glyphs never leave their row),
// so the moved pixels are exactly what repainting the rows would give, at
// a fraction of the cost: scrolling one line repaints one row instead of
// the whole window. See screen.TakeChanges.
func Scroll(m Mover, f *font.Set, sc screen.Scroll) {
	n := sc.Rows
	if n < 0 {
		n = -n
	}
	if n >= sc.Bot-sc.Top {
		return // nothing stays in view
	}
	x, w := sc.Left*f.CellW, (sc.Right-sc.Left)*f.CellW
	h := (sc.Bot - sc.Top - n) * f.CellH
	if sc.Rows > 0 {
		m.MoveRect(x, (sc.Top+n)*f.CellH, w, h, -n*f.CellH)
	} else {
		m.MoveRect(x, sc.Top*f.CellH, w, h, n*f.CellH)
	}
}

// Padding fills the area right of and below the grid with the default
// background, for windows that are not an exact multiple of the cell size.
func Padding(p Painter, s *screen.Screen, f *font.Set, winW, winH int) {
	gw, gh := s.Width*f.CellW, s.Height*f.CellH
	if winW > gw {
		p.FillRect(gw, 0, winW-gw, winH, s.DefaultBG)
	}
	if winH > gh {
		p.FillRect(0, gh, gw, winH-gh, s.DefaultBG)
	}
}

// Cursor paints the cursor for the current mode. focused selects a solid
// block or a hollow one, like GVim. Whether a busy Neovim hides the cursor
// is the caller's decision (see gui.busyDelay).
func Cursor(p Painter, s *screen.Screen, f *font.Set, focused bool) {
	if s.Height == 0 || s.Width == 0 {
		return
	}
	row, col := s.CursorRow, s.CursorCol
	if s.CellWidth(row, col) == 0 && col > 0 {
		col-- // on the right half of a double-width character
	}
	w := max(1, s.CellWidth(row, col))
	c := s.Cell(row, col)
	fg, bg := s.CursorColors(c.HL)
	m := s.CurrentMode()
	x, y := col*f.CellW, row*f.CellH
	cw := w * f.CellW

	switch m.Shape {
	case screen.CursorVertical:
		p.FillRect(x, y, max(1, f.CellW*m.CellPercentage/100), f.CellH, bg)
	case screen.CursorHorizontal:
		h := max(1, f.CellH*m.CellPercentage/100)
		p.FillRect(x, y+f.CellH-h, cw, h, bg)
	default:
		if !focused {
			p.FillRect(x, y, cw, 1, bg)
			p.FillRect(x, y+f.CellH-1, cw, 1, bg)
			p.FillRect(x, y, 1, f.CellH, bg)
			p.FillRect(x+cw-1, y, 1, f.CellH, bg)
			return
		}
		// A symbol overflowing into the next cell gets a cursor as wide as
		// its drawing, so that none of it is hidden or half recolored.
		a := s.Attr(c.HL)
		g, covered := spillGlyph(s, f, row, col, w, styleOf(a))
		p.FillRect(x, y, covered*f.CellW, f.CellH, bg)
		if !g.Empty() {
			p.DrawGlyph(g, x+g.X, y+g.Y, fg)
		}
	}
}
