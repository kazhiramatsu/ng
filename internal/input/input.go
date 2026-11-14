// Package input translates keyboard and mouse events into Neovim's key
// notation. It knows nothing about SDL; the GUI maps SDL keycodes to Key
// first, which keeps this logic testable.
package input

import (
	"math"
	"strings"
)

// Key identifies a non-text key.
type Key int

const (
	KeyNone Key = iota
	KeyEscape
	KeyEnter
	KeyTab
	KeyBackspace
	KeyDelete
	KeyInsert
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeySpace
	KeyF1 // KeyF1 + n-1 is Fn, up to F12
)

// Mod is a set of modifiers.
type Mod uint8

const (
	ModShift Mod = 1 << iota
	ModCtrl
	ModAlt
	ModSuper
)

var keyNames = map[Key]string{
	KeyEscape:    "Esc",
	KeyEnter:     "CR",
	KeyTab:       "Tab",
	KeyBackspace: "BS",
	KeyDelete:    "Del",
	KeyInsert:    "Insert",
	KeyHome:      "Home",
	KeyEnd:       "End",
	KeyPageUp:    "PageUp",
	KeyPageDown:  "PageDown",
	KeyUp:        "Up",
	KeyDown:      "Down",
	KeyLeft:      "Left",
	KeyRight:     "Right",
	KeySpace:     "Space",
}

func keyName(k Key) string {
	if k >= KeyF1 && k < KeyF1+12 {
		return "F" + itoa(int(k-KeyF1)+1)
	}
	return keyNames[k]
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func modPrefix(m Mod, withShift bool) string {
	var b strings.Builder
	if m&ModCtrl != 0 {
		b.WriteString("C-")
	}
	if withShift && m&ModShift != 0 {
		b.WriteString("S-")
	}
	if m&ModAlt != 0 {
		b.WriteString("M-")
	}
	if m&ModSuper != 0 {
		b.WriteString("D-")
	}
	return b.String()
}

// SpecialKey returns the notation for a non-text key, e.g. "<C-Up>". It
// returns "" for KeyNone. A plain space is sent as text instead.
func SpecialKey(k Key, m Mod) string {
	if k == KeyNone {
		return ""
	}
	if k == KeySpace && m&(ModCtrl|ModAlt|ModSuper|ModShift) == 0 {
		return "" // arrives as text input
	}
	name := keyName(k)
	if name == "" {
		return ""
	}
	return "<" + modPrefix(m, true) + name + ">"
}

// ChordKey returns the notation for a printable key pressed together with
// Ctrl, Alt or Super, e.g. "<C-a>" or "<M-x>". ch is the character the key
// produces without modifiers (the SDL keycode for printable keys). It
// returns "" when no such modifier is held, in which case the text input
// event carries the character.
func ChordKey(ch rune, m Mod) string {
	if m&(ModCtrl|ModAlt|ModSuper) == 0 || ch < 0x20 || ch == 0x7f {
		return ""
	}
	// Shift is folded into the character for letters: Ctrl-Shift-a is
	// "<C-S-a>" only when the user really wants the distinction; Vim
	// treats <C-A> and <C-a> the same, so keep the base letter.
	name := string(ch)
	switch ch {
	case '<':
		name = "lt"
	case '\\':
		name = "Bslash"
	case '|':
		name = "Bar"
	case ' ':
		name = "Space"
	}
	return "<" + modPrefix(m, m&ModShift != 0 && !isLetter(ch)) + name + ">"
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

// Text escapes typed text for nvim_input: only "<" is special.
func Text(s string) string {
	return strings.ReplaceAll(s, "<", "<lt>")
}

// Wheel adds up scrolling into whole steps of a mouse wheel, Neovim's unit
// of scrolling. A touchpad scrolls in fractions of a step (SDL makes ten
// pixels of a macOS touchpad one step).
type Wheel struct{ acc float64 }

// Add adds v steps and returns the whole steps to send now, negative in the
// other direction. What is left over counts towards the next call.
func (w *Wheel) Add(v float32) int {
	w.acc += float64(v)
	// Ten float32 steps of 0.1 do not add up to exactly 1.
	if r := math.Round(w.acc); math.Abs(w.acc-r) < 1e-3 {
		w.acc = r
	}
	n := int(w.acc)
	w.acc -= float64(n)
	return n
}

// MouseModifier returns the modifier string for nvim_input_mouse.
func MouseModifier(m Mod) string {
	var b strings.Builder
	if m&ModCtrl != 0 {
		b.WriteString("C")
	}
	if m&ModShift != 0 {
		b.WriteString("S")
	}
	if m&ModAlt != 0 {
		b.WriteString("A")
	}
	return b.String()
}

// IsPaste reports whether the key is Ctrl+Shift+V, which pastes the
// clipboard as in terminal emulators. ch is the unshifted character.
func IsPaste(ch rune, m Mod) bool {
	return (ch == 'v' || ch == 'V') && m&(ModCtrl|ModShift|ModAlt|ModSuper) == ModCtrl|ModShift
}

// IsMacPaste reports whether the key is Cmd+V, which pastes the clipboard
// in macOS terminal emulators. ch is the unshifted character.
func IsMacPaste(ch rune, m Mod) bool {
	return (ch == 'v' || ch == 'V') && m&(ModCtrl|ModShift|ModAlt|ModSuper) == ModSuper
}

// MacOption returns the modifiers of a printable key on macOS, where Option
// (Alt) types characters, as AltGr does elsewhere: "@", "[" or "\" on many
// layouts. Alone or with Shift it is therefore not Alt, and the key types
// what the layout gives it. With meta (g:ng_macmeta, like MacVim's
// 'macmeta') Option is Alt and the key is a <M-...> chord. With Ctrl or Cmd
// the key is a chord anyway and Option stays Alt.
func MacOption(m Mod, meta bool) Mod {
	if meta || m&(ModCtrl|ModSuper) != 0 {
		return m
	}
	return m &^ ModAlt
}

// MacMenuKey reports whether a key is a shortcut of the application menu
// that SDL creates on macOS: Cmd+Q (Quit), Cmd+W (Close), Cmd+H and
// Cmd+Option+H (Hide), Cmd+M (Minimize) and Ctrl+Cmd+F (Full Screen). The
// menu acts on it, so it is not also sent to Neovim. ch is the unshifted
// character.
func MacMenuKey(ch rune, m Mod) bool {
	switch m & (ModShift | ModCtrl | ModAlt | ModSuper) {
	case ModSuper:
		return ch == 'q' || ch == 'w' || ch == 'h' || ch == 'm'
	case ModSuper | ModAlt:
		return ch == 'h'
	case ModSuper | ModCtrl:
		return ch == 'f'
	}
	return false
}

// IMEWanted reports whether the input method should be active in a Neovim
// mode (a name from mode_info_set, as in mode_change). With insertOnly
// (the default, g:ng_ime_insert_only) it is active only where text is
// typed: Insert and Replace mode, the command line and Terminal mode. In
// Normal, Visual and Operator-pending mode keys then reach Neovim
// directly, even if the input method is switched on.
func IMEWanted(mode string, insertOnly bool) bool {
	if !insertOnly {
		return true
	}
	switch mode {
	case "insert", "replace", "showmatch", "terminal",
		"cmdline_normal", "cmdline_insert", "cmdline_replace":
		return true
	}
	return false
}

// DirectText returns the text a key types when the input method is off,
// or "" if the key types nothing by itself: ch is the character the key
// types with Shift, CapsLock and AltGr applied (SDL_GetKeyFromScancode;
// the keycode in SDL key events does not include Shift), m the modifiers. Keys with Ctrl, Alt
// or Super are chords (ChordKey) and special keys go through SpecialKey.
func DirectText(ch rune, m Mod) string {
	if m&(ModCtrl|ModAlt|ModSuper) != 0 || ch < 0x20 || ch == 0x7f || ch >= 0x40000000 {
		return ""
	}
	return Text(string(ch))
}
