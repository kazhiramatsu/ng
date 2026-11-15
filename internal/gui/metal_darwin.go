package gui

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Metal -framework QuartzCore -framework AppKit -framework CoreGraphics -framework Foundation
#include <stdlib.h>
#include "sdl.h"
#include "metal.h"
*/
import "C"

import (
	"image"
	"image/draw"
	"os"
	"unsafe"

	"github.com/Shougo/ng/internal/atlas"
	"github.com/Shougo/ng/internal/font"
	"github.com/Shougo/ng/internal/paint"
	"github.com/Shougo/ng/internal/screen"
)

// Sizes of the glyph atlas, one byte of coverage per pixel. It starts at
// 4 MB and doubles its side when a screen needs more glyphs than fit.
const (
	metalAtlasSize    = 2048
	metalAtlasMaxSize = 8192
)

// quad is C.ng_quad: a filled rectangle, or a glyph from the atlas.
type quad struct {
	x, y  int16
	w, h  uint16
	u, v  uint16
	r     uint8
	g     uint8
	b     uint8
	glyph uint8
}

// metal is ng's own renderer on macOS. It keeps no picture of the grid:
// it keeps, for each grid row, the rectangles and glyphs that paint.Row
// drew, and draws all rows into the window's drawable for every frame, as
// one instanced draw call. A changed row replaces its list; a scroll moves
// the lists (MoveRect). The cursor and the preedit text are drawn over
// them for each frame.
type metal struct {
	m         *C.ng_metal
	atlasSize int
	atlasMax  int
	packer    *atlas.Packer
	glyphs    []rect // place in the atlas by font.Glyph.ID; w == 0: none yet
	stale     bool   // the atlas was emptied while painting: the quads painted are stale
	last      bool   // painting again into the largest atlas: it is not emptied again

	cellH, gridW int      // of the rows painted
	rows         [][]quad // by grid row
	pad          []quad   // the padding around the grid
	over         []quad   // the cursor and the preedit text, over the images
	to           *[]quad  // where FillRect and DrawGlyph add
	tmp          []quad

	images   []C.ng_image_draw
	imageTex map[uint32]metalImage
}

// metalImage is the texture of an image placed with vim.ui.img.
type metalImage struct {
	img image.Image // the image the texture was made from
	tex unsafe.Pointer
}

// newMetal returns a renderer for win, or one that draws into memory
// (see read) when win is nil. It returns nil when Metal is not available.
func newMetal(win *C.SDL_Window, atlasSize, atlasMax int) *metal {
	m := C.ng_metal_new(win, C.int(atlasSize))
	if m == nil {
		return nil
	}
	return &metal{
		m:         m,
		atlasSize: atlasSize,
		atlasMax:  atlasMax,
		packer:    atlas.NewPacker(atlasSize, atlasSize),
		imageTex:  map[uint32]metalImage{},
	}
}

// ownRenderer creates the window with ng's Metal renderer, unless the
// environment has NG_RENDERER=sdl. It returns nil when SDL_Renderer is to
// draw; the window is then not created.
func (a *App) ownRenderer() renderer {
	if os.Getenv("NG_RENDERER") == "sdl" {
		return nil
	}
	title := C.CString("ng")
	defer C.free(unsafe.Pointer(title))
	win := C.ng_metal_create_window(title, 800, 600)
	if win == nil {
		return nil
	}
	m := newMetal(win, metalAtlasSize, metalAtlasMaxSize)
	if m == nil {
		C.SDL_DestroyWindow(win)
		return nil
	}
	a.win = win
	live = a
	C.ng_metal_watch_live_resize(m.m)
	return m
}

func (m *metal) name() string { return "ng metal, " + C.GoString(C.ng_metal_device(m.m)) }

func (m *metal) close() {
	for _, it := range m.imageTex {
		C.ng_metal_release_texture(it.tex)
	}
	C.ng_metal_free(m.m)
	m.m = nil
}

// resetAtlas implements renderer. The rows painted with the old glyphs
// are dropped with them.
func (m *metal) resetAtlas() {
	clear(m.glyphs)
	m.packer.Reset()
	for i := range m.rows {
		m.rows[i] = m.rows[i][:0]
	}
}

// FillRect implements paint.Painter.
func (m *metal) FillRect(x, y, w, h int, c screen.Color) {
	if w <= 0 || h <= 0 {
		return
	}
	r, g, b := c.RGB()
	*m.to = append(*m.to, quad{x: int16(x), y: int16(y), w: uint16(w), h: uint16(h), r: r, g: g, b: b})
}

// DrawGlyph implements paint.Painter.
func (m *metal) DrawGlyph(g *font.Glyph, x, y int, c screen.Color) {
	var r rect
	if g.ID < len(m.glyphs) {
		r = m.glyphs[g.ID]
	}
	if r.w == 0 { // not in the atlas yet (glyphs drawn are never empty)
		px, py, fits := m.packer.Alloc(g.W, g.H)
		if !fits {
			// Atlas full: start over with a larger one. The quads painted
			// so far are stale; paintGrid and present then paint again.
			// A screen that does not fit in the largest atlas goes
			// without the glyphs that are left over.
			if m.last || !m.growAtlas() {
				return
			}
			m.stale = true
			if px, py, fits = m.packer.Alloc(g.W, g.H); !fits {
				return
			}
		}
		C.ng_metal_upload(m.m, C.int(px), C.int(py), C.int(g.W), C.int(g.H),
			unsafe.Pointer(&g.Pix[0]), C.int(g.Stride))
		r = rect{px, py, g.W, g.H}
		if g.ID >= len(m.glyphs) {
			m.glyphs = append(m.glyphs, make([]rect, g.ID+1-len(m.glyphs))...)
		}
		m.glyphs[g.ID] = r
	}
	cr, cg, cb := c.RGB()
	*m.to = append(*m.to, quad{
		x: int16(x), y: int16(y), w: uint16(r.w), h: uint16(r.h),
		u: uint16(r.x), v: uint16(r.y), r: cr, g: cg, b: cb, glyph: 1,
	})
}

// growAtlas empties the atlas, doubling its side first if it may grow.
func (m *metal) growAtlas() bool {
	if m.atlasSize < m.atlasMax {
		if !C.ng_metal_resize_atlas(m.m, C.int(m.atlasSize*2)) {
			return false
		}
		m.atlasSize *= 2
		m.packer = atlas.NewPacker(m.atlasSize, m.atlasSize)
	}
	clear(m.glyphs)
	m.packer.Reset()
	return true
}

// clipX returns the part of q from x0 to x1, if any.
func clipX(q quad, x0, x1 int) (quad, bool) {
	l, r := max(int(q.x), x0), min(int(q.x)+int(q.w), x1)
	if l >= r {
		return q, false
	}
	if q.glyph != 0 {
		q.u += uint16(l - int(q.x))
	}
	q.x, q.w = int16(l), uint16(r-l)
	return q, true
}

// MoveRect implements paint.Mover for whole rows, as paint.Scroll moves
// them: the quads of a row move to another row. In a region narrower than
// the grid, quads are cut at its edges, so the result is what moving the
// pixels would give.
func (m *metal) MoveRect(x, y, w, h, dy int) {
	ch := m.cellH
	if ch <= 0 || y%ch != 0 || h%ch != 0 || dy%ch != 0 || dy == 0 {
		return
	}
	first, n, d := y/ch, h/ch, dy/ch
	whole := x <= 0 && x+w >= m.gridW
	for i := range n {
		src := first + i
		if d > 0 { // moving down: the last row first
			src = first + n - 1 - i
		}
		dst := src + d
		if src < 0 || dst < 0 || src >= len(m.rows) || dst >= len(m.rows) {
			continue
		}
		if whole {
			moved := m.rows[src]
			for j := range moved {
				moved[j].y += int16(dy)
			}
			m.rows[src], m.rows[dst] = m.rows[dst][:0], moved
			continue
		}
		const far = 1 << 20
		// dst: what it has outside the region, and what src has inside.
		out := m.tmp[:0]
		for _, q := range m.rows[dst] {
			if p, ok := clipX(q, -far, x); ok {
				out = append(out, p)
			}
			if p, ok := clipX(q, x+w, far); ok {
				out = append(out, p)
			}
		}
		for _, q := range m.rows[src] {
			if p, ok := clipX(q, x, x+w); ok {
				p.y += int16(dy)
				out = append(out, p)
			}
		}
		m.rows[dst], m.tmp = out, m.rows[dst]
		// src keeps what it has outside the region.
		out = m.tmp[:0]
		for _, q := range m.rows[src] {
			if p, ok := clipX(q, -far, x); ok {
				out = append(out, p)
			}
			if p, ok := clipX(q, x+w, far); ok {
				out = append(out, p)
			}
		}
		m.rows[src], m.tmp = out, m.rows[src]
	}
}

// paintGrid implements renderer: scrolled rows move, changed rows are
// painted again.
func (m *metal) paintGrid(s *screen.Screen, f *font.Set) {
	if m.cellH != f.CellH || len(m.rows) != s.Height {
		// Rows of another height or number cannot move.
		m.cellH = f.CellH
		for len(m.rows) < s.Height {
			m.rows = append(m.rows, nil)
		}
		m.rows = m.rows[:s.Height]
		for i := range m.rows {
			m.rows[i] = m.rows[i][:0]
		}
		s.MarkAllDirty()
	}
	m.gridW = s.Width * f.CellW
	for {
		scrolls, rows := s.TakeChanges()
		for _, sc := range scrolls {
			paint.Scroll(m, f, sc)
		}
		for _, r := range rows {
			m.rows[r] = m.rows[r][:0]
			m.to = &m.rows[r]
			paint.Row(m, s, f, r)
		}
		if !m.stale {
			break
		}
		// The atlas was emptied: every row needs its glyphs again.
		m.stale, m.last = false, m.atlasSize >= m.atlasMax
		s.MarkAllDirty()
	}
	m.last = false
}

// present implements renderer.
func (m *metal) present(v view) (area paint.Rect, cursorX int) {
	s, f := v.scr, v.fonts
	for again := false; ; again = true {
		m.pad, m.over, m.images = m.pad[:0], m.over[:0], m.images[:0]
		m.to = &m.pad
		paint.Padding(m, s, f, v.w, v.h)
		m.dropImages(s)
		paint.Images(m, s, f)
		m.to = &m.over
		if v.cursor {
			paint.Cursor(m, s, f, v.focused)
		}
		area, cursorX = paint.CursorRect(s, f), 0
		if v.preedit.Text != "" {
			area, cursorX = paint.DrawPreedit(m, s, f, v.preedit)
			cursorX -= area.X
		}
		if !m.stale || again {
			break
		}
		// The cursor or the preedit text emptied the atlas.
		m.stale = false
		s.MarkAllDirty()
		m.paintGrid(s, f)
		m.last = true
	}
	m.stale, m.last = false, false

	n := len(m.pad) + len(m.over)
	for _, r := range m.rows {
		n += len(r)
	}
	buf := unsafe.Slice((*quad)(unsafe.Pointer(C.ng_metal_quads(m.m, C.int(n)))), n)
	at := copy(buf, m.pad)
	for _, r := range m.rows {
		at += copy(buf[at:], r)
	}
	grid := at
	copy(buf[at:], m.over)
	var images *C.ng_image_draw
	if len(m.images) > 0 {
		images = &m.images[0]
	}
	flags := C.int(0)
	if v.inStep {
		flags |= C.NG_METAL_TRANSACTION
	}
	C.ng_metal_frame(m.m, C.int(v.w), C.int(v.h), C.uint32_t(s.DefaultBG&0xffffff),
		C.int(grid), C.int(n), images, C.int(len(m.images)), flags)
	return area, cursorX
}

// read returns the last frame of a renderer without a window.
func (m *metal) read(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if !C.ng_metal_read(m.m, unsafe.Pointer(&img.Pix[0]), C.int(w), C.int(h)) {
		return nil
	}
	for i := 0; i < len(img.Pix); i += 4 { // BGRA to RGBA
		img.Pix[i], img.Pix[i+2] = img.Pix[i+2], img.Pix[i]
	}
	return img
}

// DrawImage implements paint.ImageDrawer. Textures are made on first use
// and kept until the image is deleted or replaced (see dropImages).
func (m *metal) DrawImage(id uint32, img image.Image, src, dst image.Rectangle) {
	it, ok := m.imageTex[id]
	b := img.Bounds()
	if !ok || it.img != img {
		if ok {
			C.ng_metal_release_texture(it.tex)
			delete(m.imageTex, id)
		}
		rgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)
		tex := C.ng_metal_texture(m.m, C.int(b.Dx()), C.int(b.Dy()), unsafe.Pointer(&rgba.Pix[0]))
		if tex == nil {
			return
		}
		it = metalImage{img, tex}
		m.imageTex[id] = it
	}
	m.images = append(m.images, C.ng_image_draw{
		tex: it.tex,
		sx:  C.float(src.Min.X - b.Min.X), sy: C.float(src.Min.Y - b.Min.Y),
		sw: C.float(src.Dx()), sh: C.float(src.Dy()),
		dx: C.float(dst.Min.X), dy: C.float(dst.Min.Y),
		dw: C.float(dst.Dx()), dh: C.float(dst.Dy()),
	})
}

// dropImages frees the textures of images that were deleted or replaced.
func (m *metal) dropImages(s *screen.Screen) {
	for id, it := range m.imageTex {
		if s.Graphics.Image(id) != it.img {
			C.ng_metal_release_texture(it.tex)
			delete(m.imageTex, id)
		}
	}
}
