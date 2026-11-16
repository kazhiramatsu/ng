package font

// On Linux, fontconfig's names and order are the system's own (see
// font_darwin.go).

func (s *Set) loadFamily(family string, style Style, size float64) (*face, error) {
	return s.load(family, style, size)
}

func (fb *fallbacks) reorder() {}
