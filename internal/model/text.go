package model

import (
	"strings"
	"unicode"
)

// SanitizeText removes terminal-hostile control and zero-width characters.
func SanitizeText(s string) string {
	var b strings.Builder
	prevSpace := false

	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), isVariationSelector(r):
			continue
		case unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}

	return strings.TrimSpace(b.String())
}

func SanitizeTrack(t Track) Track {
	t.Title = SanitizeText(t.Title)
	t.Artist = SanitizeText(t.Artist)
	return t
}

func isVariationSelector(r rune) bool {
	return r == '\uFE0E' || r == '\uFE0F' || (r >= 0xE0100 && r <= 0xE01EF)
}
