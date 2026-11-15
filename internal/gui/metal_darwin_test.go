package gui

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"os"
	"testing"

	"github.com/Shougo/ng/internal/font"
	"github.com/Shougo/ng/internal/paint"
	"github.com/Shougo/ng/internal/screen"
)

func testMetal(t testing.TB, atlasSize, atlasMax int) (*metal, *font.Set) {
	t.Helper()
	f, err := font.NewSet("Courier 10 Pitch,monospace:h11", "", 96, 0)
	if err != nil {
		t.Skipf("no font: %v", err)
	}
	t.Cleanup(f.Close)
	m := newMetal(nil, atlasSize, atlasMax)
	if m == nil {
		t.Skip("no Metal device")
	}
	t.Cleanup(m.close)
	return m, f
}

func redraw(t testing.TB, s *screen.Screen, events ...[]any) {
	t.Helper()
	params := make([]any, len(events))
	for i, e := range events {
		params[i] = e
	}
	if _, err := s.HandleRedraw(params); err != nil {
		t.Fatal(err)
	}
}

// reference paints what v shows with the CPU painter of package paint,
// from nothing, in the order of App.present.
func reference(v view) *image.RGBA {
	p := paint.NewImagePainter(v.w, v.h)
	paint.Padding(p, v.scr, v.fonts, v.w, v.h)
	for r := 0; r < v.scr.Height; r++ {
		paint.Row(p, v.scr, v.fonts, r)
	}
	paint.Images(p, v.scr, v.fonts)
	if v.cursor {
		paint.Cursor(p, v.scr, v.fonts, v.focused)
	}
	if v.preedit.Text != "" {
		paint.DrawPreedit(p, v.scr, v.fonts, v.preedit)
	}
	return p.Img
}

// differ returns the first pixel where a channel of the two images
// differs by more than one (the GPU rounds a blend, the CPU painter
// truncates it), or "". Inside loose, where the test's image is, it may
// differ by two: the GPU's sampler is linear, with weights of 8 bits.
func differ(got, want *image.RGBA, loose image.Rectangle) string {
	if got == nil || !got.Rect.Eq(want.Rect) {
		return "no frame of that size"
	}
	for i := range want.Pix {
		x, y := i/4%want.Rect.Dx(), i/4/want.Rect.Dx()
		limit := 1
		if image.Pt(x, y).In(loose) {
			limit = 2
		}
		if d := int(got.Pix[i]) - int(want.Pix[i]); d < -limit || d > limit {
			return fmt.Sprintf("pixel (%d, %d): %v, want %v", x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
		}
	}
	return ""
}

func dumpPNG(t *testing.T, name string, img *image.RGBA) {
	dir := os.Getenv("NG_PAINT_DUMP")
	if dir == "" || img == nil {
		return
	}
	f, err := os.Create(dir + "/" + name + ".png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, img)
}

// The Metal renderer must show what the CPU painter paints from nothing:
// text, double-width characters, decorations, padding, an image, every
// cursor shape and the preedit text, after any sequence of changed rows
// and scrolls (which move its rows instead of repainting them), in
// regions narrower than the grid too.
func TestMetalMatchesImagePainter(t *testing.T) {
	m, f := testMetal(t, metalAtlasSize, metalAtlasMaxSize)
	const cols, rows = 14, 8
	s := screen.New(cols, rows)
	rnd := rand.New(rand.NewPCG(3, 4))
	line := func(r int) []any {
		cells := []any{}
		for c := 0; c < cols; c += 2 {
			hl := int64(rnd.IntN(9))
			switch rnd.IntN(3) {
			case 0:
				cells = append(cells, []any{"漢", hl}, []any{""})
			case 1:
				cells = append(cells, []any{string(rune('a' + rnd.IntN(26))), hl}, []any{"g", hl})
			default:
				cells = append(cells, []any{" ", hl, int64(2)})
			}
		}
		return []any{"grid_line", []any{int64(1), int64(r), int64(0), cells, false}}
	}
	hl := func(id int, attrs map[string]any) []any {
		return []any{"hl_attr_define", []any{int64(id), attrs, map[string]any{}, []any{}}}
	}
	redraw(t, s,
		[]any{"default_colors_set", []any{int64(0xd0d0d0), int64(0x101010), int64(0xff0000), int64(0), int64(0)}},
		hl(1, map[string]any{"background": int64(0x203040)}),
		hl(2, map[string]any{"foreground": int64(0xff8000), "underline": true}),
		hl(3, map[string]any{"reverse": true, "bold": true}),
		hl(4, map[string]any{"undercurl": true, "special": int64(0x00ff00), "italic": true}),
		hl(5, map[string]any{"strikethrough": true, "underdouble": true}),
		hl(6, map[string]any{"underdotted": true, "background": int64(0x402000)}),
		hl(7, map[string]any{"underdashed": true, "special": int64(0x8080ff)}),
		hl(8, map[string]any{"foreground": int64(0x101010), "background": int64(0xe0e0e0)}),
		[]any{"mode_info_set", []any{true, []any{
			map[string]any{"name": "normal", "cursor_shape": "block", "attr_id": int64(0)},
			map[string]any{"name": "insert", "cursor_shape": "vertical", "cell_percentage": int64(25), "attr_id": int64(3)},
			map[string]any{"name": "replace", "cursor_shape": "horizontal", "cell_percentage": int64(20), "attr_id": int64(1)},
		}}},
		[]any{"mode_change", []any{"normal", int64(0)}},
	)
	for r := 0; r < rows; r++ {
		redraw(t, s, line(r))
	}
	// An image with every alpha, at its own size, over the text.
	img := image.NewNRGBA(image.Rect(0, 0, 21, 13))
	for i := range img.Pix {
		img.Pix[i] = uint8(rnd.IntN(256))
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	s.Graphics.Feed([]byte("\x1b_Gf=100,a=t,t=d,i=1,q=2,m=0;" + base64.StdEncoding.EncodeToString(b.Bytes()) + "\x1b\\"))
	s.Graphics.Feed([]byte("\x1b7\x1b[2;3H\x1b_Ga=p,i=1,p=2,C=1,q=2\x1b\\\x1b8"))

	v := view{scr: s, fonts: f, w: cols*f.CellW + 5, h: rows*f.CellH + 7, focused: true, cursor: true}
	imageAt := func() image.Rectangle {
		var at image.Rectangle
		for _, p := range s.Graphics.Placements() {
			at = at.Union(image.Rect(0, 0, 21, 13).Add(image.Pt(p.Col*v.fonts.CellW, p.Row*v.fonts.CellH)))
		}
		return at
	}
	check := func(what string, events [][]any) {
		t.Helper()
		redraw(t, s, append(events, []any{"flush"})...)
		m.paintGrid(s, f)
		m.present(v)
		got, want := m.read(v.w, v.h), reference(v)
		if d := differ(got, want, imageAt()); d != "" {
			dumpPNG(t, "metal", got)
			dumpPNG(t, "metal-want", want)
			t.Fatalf("%s: the Metal frame differs from the CPU painter's after %v: %s", what, events, d)
		}
	}
	scroll := func(top, bot, left, right, n int) []any {
		return []any{"grid_scroll", []any{int64(1), int64(top), int64(bot), int64(left), int64(right), int64(n), int64(0)}}
	}
	check("first frame", nil)
	check("changed then scrolled", [][]any{line(3), scroll(0, rows, 0, cols, 1)})
	check("changed then scrolled in a split", [][]any{line(5), scroll(2, 7, 4, 10, -2)})
	preedits := []paint.Preedit{{}, {Text: "にほんご", Cursor: 4}, {Text: "漢字kana", Cursor: 0, SelLen: 2}}
	for step := 0; step < 300; step++ {
		var events [][]any
		for i := rnd.IntN(4); i >= 0; i-- {
			if rnd.IntN(2) == 0 {
				events = append(events, line(rnd.IntN(rows)))
				continue
			}
			top := rnd.IntN(rows - 1)
			bot := top + 1 + rnd.IntN(rows-top)
			left, right := 0, cols
			if rnd.IntN(2) == 0 {
				left = 2 * rnd.IntN(cols/2)
				right = left + 2 + 2*rnd.IntN((cols-left)/2)
			}
			n := 1 + rnd.IntN(bot-top+1)
			if rnd.IntN(2) == 0 {
				n = -n
			}
			events = append(events, scroll(top, bot, left, right, n))
		}
		events = append(events,
			[]any{"grid_cursor_goto", []any{int64(1), int64(rnd.IntN(rows)), int64(rnd.IntN(cols))}},
			[]any{"mode_change", []any{"x", int64(rnd.IntN(3))}})
		v.focused, v.cursor = rnd.IntN(4) != 0, rnd.IntN(8) != 0
		v.preedit = preedits[rnd.IntN(len(preedits))]
		check(fmt.Sprintf("step %d", step), events)
	}
	dumpPNG(t, "metal", m.read(v.w, v.h))
	// Other fonts: the atlas and the rows start over.
	big, err := font.NewSet("Courier 10 Pitch,monospace:h17", "", 96, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer big.Close()
	f = big
	m.resetAtlas()
	s.MarkAllDirty()
	v.fonts, v.w, v.h = f, cols*f.CellW+3, rows*f.CellH+1
	check("larger font", nil)
	// A window smaller and larger than the grid, as during a resize.
	v.w, v.h = 5*f.CellW+2, 3*f.CellH+4
	check("small window", nil)
	v.w, v.h = 20*f.CellW, 12*f.CellH
	check("large window", [][]any{line(0)})
}

// A screen with more glyphs than the atlas holds makes the atlas grow and
// is painted again; one that does not fit in the largest atlas is still
// drawn, without the glyphs left over.
func TestMetalAtlasGrows(t *testing.T) {
	m, f := testMetal(t, 64, 256)
	const cols, rows = 16, 6
	s := screen.New(cols, rows)
	fill := func(first rune) {
		for r := 0; r < rows; r++ {
			cells := []any{}
			for c := 0; c < cols; c++ {
				cells = append(cells, []any{string(first + rune(r*cols+c))})
			}
			redraw(t, s, []any{"grid_line", []any{int64(1), int64(r), int64(0), cells, false}})
		}
	}
	v := view{scr: s, fonts: f, w: cols * f.CellW, h: rows * f.CellH, focused: true, cursor: true}
	fill('!') // 96 glyphs: more than 64x64 pixels hold
	m.paintGrid(s, f)
	m.present(v)
	if m.atlasSize <= 64 {
		t.Fatalf("atlas of %d pixels after 96 glyphs", m.atlasSize)
	}
	if d := differ(m.read(v.w, v.h), reference(v), image.Rectangle{}); d != "" {
		t.Errorf("after the atlas grew to %d: %s", m.atlasSize, d)
	}
	// With the preedit text the atlas overflows while a frame is drawn.
	v.preedit = paint.Preedit{Text: "ÀÁÂÃÄÅÆÇÈÉÊËÌÍÎÏ", Cursor: 3}
	m.atlasMax = m.atlasSize * 2
	for m.present(v); m.atlasSize < m.atlasMax; m.present(v) {
		fill(rune(0x100 + m.atlasSize)) // other glyphs, until it grows
		m.paintGrid(s, f)
	}
	if d := differ(m.read(v.w, v.h), reference(v), image.Rectangle{}); d != "" {
		t.Errorf("after the atlas grew to %d for a frame: %s", m.atlasSize, d)
	}
	// Too many for the largest atlas: nothing to compare, but it ends.
	m.atlasMax = m.atlasSize
	for i := range 4 {
		fill(rune(0x400 + 96*i))
		m.paintGrid(s, f)
		m.present(v)
	}
	if m.read(v.w, v.h).RGBAAt(v.w-1, v.h-1) == (color.RGBA{}) {
		t.Error("no frame")
	}
}

// fullScreen returns redraw params that fill a cols x rows screen with
// mixed ASCII and Japanese text in several highlights, as in package
// paint's benchmark.
func fullScreen(cols, rows, shift int) []any {
	params := []any{}
	for r := 0; r < rows; r++ {
		cells := []any{}
		col := 0
		for col < cols {
			switch {
			case col%20 < 10 && col+2 <= cols:
				cells = append(cells, []any{"漢", int64(col % 3)}, []any{""})
				col += 2
			default:
				cells = append(cells, []any{string(rune('a' + (col+r+shift)%26)), int64(col % 3)})
				col++
			}
		}
		params = append(params, []any{"grid_line", []any{int64(1), int64(r), int64(0), cells, false}})
	}
	return append(params, []any{"flush"})
}

// BenchmarkMetalFrame measures a frame of a 200x60 grid with the Metal
// renderer, drawn into memory. The cases with "paint" and "build" are ng's
// own work for a frame, before it is handed to Metal; the others also wait
// for the GPU to finish the frame, which a window does not.
func BenchmarkMetalFrame(b *testing.B) {
	m, f := testMetal(b, metalAtlasSize, metalAtlasMaxSize)
	const cols, rows = 200, 60
	s := screen.New(cols, rows)
	v := view{scr: s, fonts: f, w: cols * f.CellW, h: rows * f.CellH, focused: true, cursor: true}
	full := [2][]any{fullScreen(cols, rows, 0), fullScreen(cols, rows, 1)}
	s.HandleRedraw(full[0])
	m.paintGrid(s, f)
	m.present(v)
	b.Run("full-repaint", func(b *testing.B) { // every row changes: <C-f>
		for i := 0; i < b.N; i++ {
			s.HandleRedraw(full[i%2])
			m.paintGrid(s, f)
			m.present(v)
		}
	})
	b.Run("full-repaint-paint-build", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.HandleRedraw(full[i%2])
			m.paintGrid(s, f)
			m.build(v)
		}
	})
	scroll := []any{
		[]any{"grid_scroll", []any{int64(1), int64(0), int64(rows), int64(0), int64(cols), int64(1), int64(0)}},
		full[0][rows-1],
		[]any{"flush"},
	}
	b.Run("scroll-one-line", func(b *testing.B) { // <C-e>
		for i := 0; i < b.N; i++ {
			s.HandleRedraw(scroll)
			m.paintGrid(s, f)
			m.present(v)
		}
	})
	b.Run("scroll-one-line-paint-build", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			s.HandleRedraw(scroll)
			m.paintGrid(s, f)
			m.build(v)
		}
	})
	b.Run("present-only", func(b *testing.B) { // the cursor moved
		for i := 0; i < b.N; i++ {
			m.present(v)
		}
	})
	b.Run("present-only-build", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			m.build(v)
		}
	})
	n := 0
	for _, r := range m.rows {
		n += len(r)
	}
	b.Logf("%dx%d pixels, %d quads", v.w, v.h, n)
}
