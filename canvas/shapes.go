package canvas

// Braille renders using Unicode Braille Patterns (U+2800-U+28FF),
// giving each terminal cell a 2x4 sub-pixel dot grid - 2x horizontal
// and 4x vertical resolution over plain character-cell drawing. This
// is the technique used by drawille and by mapscii's own Canvas.js,
// and is the default Shape used by this library.
var Braille = Shape{
	Name:      "braille",
	DotsX:     2,
	DotsY:     4,
	Bit:       func(dx, dy int) byte { return brailleDotMask[dy][dx] },
	Glyph:     brailleGlyph,
	PinMarker: '●',
}

// brailleDotMask maps a dot's position within a cell (row 0-3, col 0-1)
// to the bit that must be set in the braille pattern byte to light it
// up. This is the standard braille/drawille dot numbering:
//
//	0 3
//	1 4
//	2 5
//	6 7
var brailleDotMask = [4][2]byte{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// brailleBase is the Unicode code point of the "all dots off" braille
// pattern; individual dots are turned on by OR-ing a bit mask onto it.
const brailleBase = 0x2800

func brailleGlyph(mask byte) rune {
	if mask == 0 {
		return ' '
	}
	return rune(brailleBase + int(mask))
}

// Blocks renders using the 16 Unicode "quadrant" block characters
// (U+2580-U+259F), giving each terminal cell a 2x2 sub-pixel grid.
// It has less resolution than Braille but is often visually bolder,
// and relies on a smaller, very widely supported set of characters.
var Blocks = Shape{
	Name:      "blocks",
	DotsX:     2,
	DotsY:     2,
	Bit:       func(dx, dy int) byte { return blockDotMask[dy][dx] },
	Glyph:     func(mask byte) rune { return blockGlyphs[mask] },
	PinMarker: '●',
}

var blockDotMask = [2][2]byte{
	{1, 2},
	{4, 8},
}

// blockGlyphs is indexed by a 4-bit mask, bit 0 = top-left, bit 1 =
// top-right, bit 2 = bottom-left, bit 3 = bottom-right.
var blockGlyphs = [16]rune{
	' ', '▘', '▝', '▀',
	'▖', '▌', '▞', '▛',
	'▗', '▚', '▐', '▜',
	'▄', '▙', '▟', '█',
}

// ASCII is a plain-ASCII Shape with 1x1 dot resolution (one "pixel"
// per terminal cell): no Unicode is required to render it, at the
// cost of resolution and nuance. It uses '#' for lit cells; use
// NewASCIIShape to pick a different character.
var ASCII = NewASCIIShape('#')

// NewASCIIShape returns a plain-ASCII Shape (1x1 dot resolution) that
// renders a lit cell using the given rune.
func NewASCIIShape(on rune) Shape {
	return Shape{
		Name:  "ascii",
		DotsX: 1,
		DotsY: 1,
		Bit:   func(dx, dy int) byte { return 1 },
		Glyph: func(mask byte) rune {
			if mask == 0 {
				return ' '
			}
			return on
		},
		PinMarker: '*',
	}
}
