package input

import "testing"

func TestSpecialKey(t *testing.T) {
	cases := []struct {
		k    Key
		m    Mod
		want string
	}{
		{KeyEscape, 0, "<Esc>"},
		{KeyEnter, 0, "<CR>"},
		{KeyTab, ModShift, "<S-Tab>"},
		{KeyUp, ModCtrl, "<C-Up>"},
		{KeyF1, 0, "<F1>"},
		{KeyF1 + 11, ModAlt, "<M-F12>"},
		{KeyBackspace, ModCtrl | ModShift, "<C-S-BS>"},
		{KeySpace, 0, ""},
		{KeySpace, ModCtrl, "<C-Space>"},
		{KeyNone, ModCtrl, ""},
	}
	for _, c := range cases {
		if got := SpecialKey(c.k, c.m); got != c.want {
			t.Errorf("SpecialKey(%d,%d)=%q want %q", c.k, c.m, got, c.want)
		}
	}
}

func TestChordKey(t *testing.T) {
	cases := []struct {
		ch   rune
		m    Mod
		want string
	}{
		{'a', 0, ""},
		{'a', ModShift, ""},
		{'a', ModCtrl, "<C-a>"},
		{'a', ModCtrl | ModShift, "<C-a>"},
		{'x', ModAlt, "<M-x>"},
		{'<', ModCtrl, "<C-lt>"},
		{'\\', ModCtrl, "<C-Bslash>"},
		{'|', ModAlt, "<M-Bar>"},
		{'1', ModCtrl | ModShift, "<C-S-1>"},
		{'^', ModCtrl, "<C-^>"},
		{']', ModCtrl, "<C-]>"},
	}
	for _, c := range cases {
		if got := ChordKey(c.ch, c.m); got != c.want {
			t.Errorf("ChordKey(%q,%d)=%q want %q", c.ch, c.m, got, c.want)
		}
	}
}

func TestText(t *testing.T) {
	if got := Text("a<b>漢"); got != "a<lt>b>漢" {
		t.Errorf("got %q", got)
	}
}

func TestMouseModifier(t *testing.T) {
	if got := MouseModifier(ModCtrl | ModShift | ModAlt); got != "CSA" {
		t.Errorf("got %q", got)
	}
}

func TestIsPaste(t *testing.T) {
	cases := []struct {
		ch   rune
		m    Mod
		want bool
	}{
		{'v', ModCtrl | ModShift, true},
		{'V', ModCtrl | ModShift, true},
		{'v', ModCtrl, false}, // <C-v> stays Neovim's
		{'v', ModCtrl | ModShift | ModAlt, false},
		{'c', ModCtrl | ModShift, false},
	}
	for _, c := range cases {
		if got := IsPaste(c.ch, c.m); got != c.want {
			t.Errorf("IsPaste(%q,%d)=%v", c.ch, c.m, got)
		}
	}
}

func TestIsMacPaste(t *testing.T) {
	cases := []struct {
		ch   rune
		m    Mod
		want bool
	}{
		{'v', ModSuper, true},
		{'v', ModCtrl | ModShift, false}, // <C-S-v> reaches Neovim on macOS
		{'v', ModSuper | ModShift, false},
		{'v', ModSuper | ModAlt, false},
		{'c', ModSuper, false},
	}
	for _, c := range cases {
		if got := IsMacPaste(c.ch, c.m); got != c.want {
			t.Errorf("IsMacPaste(%q,%d)=%v", c.ch, c.m, got)
		}
	}
}

func TestMacOption(t *testing.T) {
	cases := []struct {
		m, want Mod
		meta    bool
	}{
		{ModAlt, 0, false},                          // Option+a types "å"
		{ModAlt | ModShift, ModShift, false},        // Option+Shift+2 types "€"
		{ModAlt, ModAlt, true},                      // g:ng_macmeta: <M-a>
		{ModAlt | ModCtrl, ModAlt | ModCtrl, false}, // <C-M-a>
		{ModAlt | ModSuper, ModAlt | ModSuper, false},
		{ModShift, ModShift, false},
	}
	for _, c := range cases {
		if got := MacOption(c.m, c.meta); got != c.want {
			t.Errorf("MacOption(%d, %v) = %d, want %d", c.m, c.meta, got, c.want)
		}
	}
	// Option then types a character, not a chord.
	m := MacOption(ModAlt, false)
	if ChordKey('a', m) != "" || DirectText('å', m) != "å" {
		t.Errorf("Option+a: chord %q, text %q", ChordKey('a', m), DirectText('å', m))
	}
}

func TestMacMenuKey(t *testing.T) {
	for _, k := range []struct {
		ch rune
		m  Mod
	}{{'q', ModSuper}, {'w', ModSuper}, {'h', ModSuper}, {'m', ModSuper},
		{'h', ModSuper | ModAlt}, {'f', ModSuper | ModCtrl}} {
		if !MacMenuKey(k.ch, k.m) {
			t.Errorf("%q with %d is a menu key", k.ch, k.m)
		}
	}
	for _, k := range []struct {
		ch rune
		m  Mod
	}{{'s', ModSuper}, {'q', ModSuper | ModShift}, {'q', ModCtrl}, {'f', ModSuper}, {'q', 0}} {
		if MacMenuKey(k.ch, k.m) {
			t.Errorf("%q with %d goes to Neovim", k.ch, k.m)
		}
	}
}

func TestWheel(t *testing.T) {
	var w Wheel
	if n := w.Add(1); n != 1 {
		t.Errorf("one mouse wheel step: %d", n)
	}
	if n := w.Add(-3); n != -3 {
		t.Errorf("three steps back: %d", n)
	}
	// A touchpad on macOS: 20 events of 3 pixels, a tenth of a step each,
	// make 6 steps, in either direction, however float32 rounds 0.3.
	for _, dir := range []float32{-1, 1, -1} {
		total := 0
		for i := 0; i < 20; i++ {
			n := w.Add(dir * 0.3)
			if n*int(dir) < 0 || n*int(dir) > 1 {
				t.Fatalf("step %d: %d", i, n)
			}
			total += n
		}
		if total != 6*int(dir) {
			t.Errorf("20 x %v: %d steps, want %d", dir*0.3, total, 6*int(dir))
		}
	}
	// What is left over counts.
	w = Wheel{}
	if w.Add(0.6) != 0 || w.Add(0.6) != 1 || w.Add(0.8) != 1 {
		t.Error("fractions do not add up")
	}
}

func TestIMEWanted(t *testing.T) {
	for _, m := range []string{"insert", "replace", "cmdline_normal", "cmdline_insert", "terminal", "showmatch"} {
		if !IMEWanted(m, true) {
			t.Errorf("%s: IME should be on", m)
		}
	}
	for _, m := range []string{"normal", "visual", "operator", "visual_select", "more", ""} {
		if IMEWanted(m, true) {
			t.Errorf("%s: IME should be off", m)
		}
		if !IMEWanted(m, false) {
			t.Errorf("%s: IME should be on with g:ng_ime_insert_only = 0", m)
		}
	}
}

func TestDirectText(t *testing.T) {
	cases := []struct {
		ch   rune
		m    Mod
		want string
	}{
		{'j', 0, "j"}, {'J', ModShift, "J"}, {'<', ModShift, "<lt>"}, {' ', 0, " "},
		{'ü', 0, "ü"}, {'j', ModCtrl, ""}, {'j', ModAlt, ""}, {0x1b, 0, ""},
		{0x40000052, 0, ""}, // an SDL scancode-based key (Up)
	}
	for _, c := range cases {
		if got := DirectText(c.ch, c.m); got != c.want {
			t.Errorf("DirectText(%q,%d) = %q, want %q", c.ch, c.m, got, c.want)
		}
	}
}
