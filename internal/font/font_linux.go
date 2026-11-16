package font

// loadFamily opens the font for a family of 'guifont'. It is in a file
// for Linux so that a macOS fork can define its own in font_darwin.go,
// looking some families up under the names macOS gives them.
func (s *Set) loadFamily(family string, style Style, size float64) (*face, error) {
	return s.load(family, style, 0, size)
}
