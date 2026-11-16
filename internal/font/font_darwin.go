package font

/*
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <ft2build.h>
#include FT_FREETYPE_H
#include <fontconfig/fontconfig.h>

// ng_installed reports whether a font is installed under family.
static int ng_installed(const char *family) {
	FcPattern *pat = FcPatternCreate();
	FcPatternAddString(pat, FC_FAMILY, (const FcChar8 *)family);
	FcObjectSet *os = FcObjectSetBuild(FC_FAMILY, (char *)NULL);
	FcFontSet *set = FcFontList(NULL, pat, os);
	int n = set ? set->nfont : 0;
	if (set)
		FcFontSetDestroy(set);
	FcObjectSetDestroy(os);
	FcPatternDestroy(pat);
	return n > 0;
}

static int ng_family_is(FcPattern *font, const char *family) {
	FcChar8 *fam = NULL;
	for (int i = 0; FcPatternGetString(font, FC_FAMILY, i, &fam) == FcResultMatch; i++) {
		if (FcStrCmpIgnoreCase(fam, (const FcChar8 *)family) == 0)
			return 1;
	}
	return 0;
}

// ng_is_last_resort reports whether font is macOS's LastResort, which has
// a placeholder glyph (a box naming the Unicode block) for every code
// point.
static int ng_is_last_resort(FcPattern *font) {
	FcChar8 *fam;
	for (int i = 0; FcPatternGetString(font, FC_FAMILY, i, &fam) == FcResultMatch; i++) {
		if (strstr((const char *)fam, "LastResort"))
			return 1;
	}
	return 0;
}

// ng_lang_is reports whether lang, as fontconfig writes languages ("ja",
// "zh-tw"), is base or a variant of it.
static int ng_lang_is(const char *lang, const char *base) {
	size_t n = strlen(base);
	return strncasecmp(lang, base, n) == 0 && (lang[n] == '\0' || lang[n] == '-');
}

// ng_fallback_first returns the families that fallback tries first
// (NULL-terminated): the fonts macOS itself uses for Chinese, Japanese and
// Korean, in the order of Core Text's cascade list for the language
// (CTFontCopyDefaultCascadeListForLanguages): Hiragino Sans unless the
// language is Korean or Chinese. fontconfig sorts Nanum Gothic, a Korean
// font that has kana and kanji too, before all of them.
static const char *const *ng_fallback_first(void) {
	static const char *const ja[] = {"Hiragino Sans", "PingFang SC", "PingFang TC", "Apple SD Gothic Neo", NULL};
	static const char *const ko[] = {"Apple SD Gothic Neo", "Hiragino Sans", "PingFang SC", "PingFang TC", NULL};
	static const char *const sc[] = {"PingFang SC", "PingFang TC", "Hiragino Sans", "Apple SD Gothic Neo", NULL};
	static const char *const tc[] = {"PingFang TC", "PingFang SC", "Hiragino Sans", "Apple SD Gothic Neo", NULL};
	static const char *const hk[] = {"PingFang HK", "PingFang SC", "Hiragino Sans", "Apple SD Gothic Neo", NULL};
	// fontconfig's language (FC_LANG, LC_ALL, LC_CTYPE or LANG), as for
	// its own criteria.
	FcStrSet *set = FcGetDefaultLangs(); // fontconfig's own, never freed
	FcStrList *langs = set ? FcStrListCreate(set) : NULL;
	const FcChar8 *lang = langs ? FcStrListNext(langs) : NULL;
	const char *const *out = ja;
	if (lang) {
		const char *l = (const char *)lang;
		if (ng_lang_is(l, "ko"))
			out = ko;
		else if (ng_lang_is(l, "zh-tw"))
			out = tc;
		else if (ng_lang_is(l, "zh-hk") || ng_lang_is(l, "zh-mo"))
			out = hk;
		else if (ng_lang_is(l, "zh"))
			out = sc;
	}
	if (langs)
		FcStrListDone(langs);
	return out;
}

// ng_rank places font in the order of ng_fallback_order: 0 to k-1 for the
// k families of first, k for the rest, k+1 for LastResort.
static int ng_rank(FcPattern *font, const char *const *first, int k) {
	for (int i = 0; i < k; i++) {
		if (ng_family_is(font, first[i]))
			return i;
	}
	return ng_is_last_resort(font) ? k + 1 : k;
}

// ng_fallback_order makes the order that of macOS itself (Core Text), with
// two changes to fontconfig's:
//
//   - The families of ng_fallback_first come first.
//   - LastResort comes last: macOS shows its boxes only when no other
//     font has the character. fontconfig sorts it among the first fonts,
//     since it says it is monospaced.
//
// The order stays fontconfig's if memory runs out.
static void ng_fallback_order(FcFontSet *set) {
	const char *const *first = ng_fallback_first();
	int k = 0;
	while (first[k])
		k++;
	int *rank = malloc((set->nfont + 1) * sizeof *rank);
	FcPattern **sorted = malloc((set->nfont + 1) * sizeof *sorted);
	if (rank && sorted) {
		for (int i = 0; i < set->nfont; i++)
			rank[i] = ng_rank(set->fonts[i], first, k);
		int n = 0;
		for (int r = 0; r <= k + 1; r++) {
			for (int i = 0; i < set->nfont; i++) {
				if (rank[i] == r)
					sorted[n++] = set->fonts[i];
			}
		}
		memcpy(set->fonts, sorted, n * sizeof *sorted);
	}
	free(sorted);
	free(rank);
}
*/
import "C"

import (
	"strings"
	"sync"
	"unsafe"
)

// fontconfig knows the fonts of macOS but not its conventions: the names
// of its monospace fonts, and the order of its fallback fonts
// (docs/macos.md, "The default font" and "Fallback fonts"). font_linux.go
// has the same functions, keeping fontconfig's.

// loadFamily opens the font for a family of 'guifont', under macOS's names
// where fontconfig's differ. fontconfig's "monospace" lists only fonts
// found on Linux: it gives Andale Mono (fontconfig 2.17 under a Japanese
// or Chinese locale: a proportional CJK font), where Menlo is the macOS
// monospace font (and MacVim's default). SF Mono, the first font of
// Neovim's default 'guifont' on macOS, is installed as the hidden system
// font ".SF NS Mono".
func (s *Set) loadFamily(family string, style Style, size float64) (*face, error) {
	switch {
	case strings.EqualFold(family, "monospace"):
		// load takes any font for "monospace", so Menlo goes first.
		if f, err := s.load("Menlo", style, size); err == nil {
			return f, nil
		}
	case strings.EqualFold(family, "SF Mono"):
		// A font installed as "SF Mono" goes first. Asking load for it
		// costs as much as loading a font, 1.5 ms with the 2,800 fonts
		// of a Mac, so ng looks it up once.
		if sfMonoInstalled() {
			if f, err := s.load(family, style, size); err == nil {
				return f, nil
			}
		}
		if f, err := s.load(".SF NS Mono", style, size); err == nil {
			return f, nil
		}
	}
	return s.load(family, style, size)
}

// familyInstalled reports whether a font is installed under family.
func familyInstalled(family string) bool {
	cfam := C.CString(family)
	defer C.free(unsafe.Pointer(cfam))
	return C.ng_installed(cfam) != 0
}

var sfMonoInstalled = sync.OnceValue(func() bool { return familyInstalled("SF Mono") })

// reorder puts the fallback fonts in macOS's order (ng_fallback_order).
func (fb *fallbacks) reorder() { C.ng_fallback_order(fb.c.fonts) }

// family returns the face's family name, as the font itself gives it
// (TestMacFallbacks).
func (f *face) family() string { return C.GoString((*C.char)(unsafe.Pointer(f.ft.family_name))) }
