package font

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// On macOS fontconfig knows the monospace fonts under other names (see
// ng_alias): "monospace" must be Menlo, not Andale Mono or (fontconfig 2.17
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
