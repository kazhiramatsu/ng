package font

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// On macOS fontconfig knows the monospace fonts under other names (see
// loadFamily): "monospace" must be Menlo, not Andale Mono or (fontconfig 2.17
// under a Japanese locale) a proportional Hiragino, and SF Mono, the first
// font of Neovim's default 'guifont' there, is a hidden system font. Each
// loads without warnings, with real bold and italic faces.
func TestMacFamilies(t *testing.T) {
	cases := []struct{ guifont, file string }{
		{"", "Menlo"},
		{"monospace:h11", "Menlo"},
		{"SF Mono:h11", "SF"}, // SFNSMono.ttf, or an installed SF-Mono-Regular.otf
		{"SF Mono,Menlo,Monaco,Courier New,monospace", "SF"},
	}
	for _, c := range cases {
		s, err := NewSet(c.guifont, "", 144, 0)
		if err != nil {
			t.Errorf("%q: %v", c.guifont, err)
			continue
		}
		if len(s.Warnings) > 0 {
			t.Errorf("%q: warnings %q", c.guifont, s.Warnings)
		}
		if f := filepath.Base(s.primaries(0)[0].path); !strings.HasPrefix(f, c.file) {
			t.Errorf("%q: %s, want %s", c.guifont, f, c.file)
		}
		if f := s.primaries(Bold)[0]; f.embolden {
			t.Errorf("%q: bold is synthesized from %s", c.guifont, filepath.Base(f.path))
		}
		if f := s.primaries(Italic)[0]; f.oblique {
			t.Errorf("%q: italic is synthesized from %s", c.guifont, filepath.Base(f.path))
		}
		s.Close()
	}
}

// fontconfig reads the language once per process, so TestMacFamilies runs
// again in a process with a Japanese one, where "monospace" went wrong with
// fontconfig 2.17.
func TestMacFamiliesJapanese(t *testing.T) {
	if os.Getenv("FC_LANG") == "ja" {
		t.Skip("this is the Japanese process")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestMacFamilies$")
	cmd.Env = append(os.Environ(), "FC_LANG=ja")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("with FC_LANG=ja: %v\n%s", err, out)
	}
}

// Characters missing from 'guifont' come from the fonts macOS itself uses
// (Core Text's cascade list): Hiragino Sans for Japanese, Apple SD Gothic
// Neo for Korean, and for Chinese and Japanese characters too in a Korean
// locale; never the Korean Nanum Gothic for kana, nor LastResort's boxes
// for a script that has a font. Each language runs in a process of its
// own, since fontconfig reads it once.
func TestMacFallbacks(t *testing.T) {
	lang := os.Getenv("FC_LANG")
	if lang == "" {
		for _, lang := range []string{"ja", "en", "ko"} {
			cmd := exec.Command(os.Args[0], "-test.run=^TestMacFallbacks$", "-test.v")
			cmd.Env = append(os.Environ(), "FC_LANG="+lang)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("with FC_LANG=%s: %v\n%s", lang, err, out)
			}
		}
		return
	}
	s, err := NewSet("SF Mono,Menlo,Monaco,Courier New,monospace", "", 144, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cjk := "Hiragino Sans"
	if lang == "ko" {
		cjk = "Apple SD Gothic Neo"
	}
	cases := []struct {
		r      rune
		family string // "": any but LastResort
		bold   bool   // with a bold face
	}{
		{'あ', cjk, true}, {'漢', cjk, true}, {'「', cjk, true},
		{'한', "Apple SD Gothic Neo", true},
		{'Ք', "", false},               // an Armenian font (Noto Sans Armenian on macOS 26)
		{'ក', "", false},               // a Khmer font, which fontconfig sorts after LastResort
		{0x0378, ".LastResort", false}, // unassigned: no other font has it
	}
	for _, c := range cases {
		for _, style := range []Style{0, Bold} {
			f, _ := s.faceFor(c.r, style, 2)
			if f == nil {
				t.Errorf("%U (style %d): no font", c.r, style)
				continue
			}
			if got := f.family(); c.family == "" && got == ".LastResort" ||
				c.family != "" && got != c.family {
				t.Errorf("%U (style %d): %s (%s), want %q", c.r, style, got, filepath.Base(f.path), c.family)
			}
			if style == Bold && c.bold && f.embolden {
				t.Errorf("%U: bold is synthesized from %s", c.r, filepath.Base(f.path))
			}
		}
	}
}
