package canvas

import (
	"strings"
	"testing"
)

func TestNewCanvasDimensions(t *testing.T) {
	c := New(Braille, 10, 5)
	if c.Width() != 20 || c.Height() != 20 {
		t.Errorf("Width/Height = %d/%d, want 20/20", c.Width(), c.Height())
	}
	if c.DotsPerCellX() != 2 || c.DotsPerCellY() != 4 {
		t.Errorf("DotsPerCellX/Y = %d/%d, want 2/4", c.DotsPerCellX(), c.DotsPerCellY())
	}
}

func TestSetSingleDotBraille(t *testing.T) {
	c := New(Braille, 1, 1)
	c.Set(0, 0, Color{R: 255})
	frame := c.Frame()
	want := "\x1b[38;2;255;0;0m" + string(rune(brailleBase+0x01)) + resetSeq
	if frame != want {
		t.Errorf("Frame() = %q, want %q", frame, want)
	}
}

func TestAllDotsFillCellBraille(t *testing.T) {
	c := New(Braille, 1, 1)
	for y := 0; y < Braille.DotsY; y++ {
		for x := 0; x < Braille.DotsX; x++ {
			c.Set(x, y, Color{R: 1, G: 2, B: 3})
		}
	}
	if c.cells[0].mask != 0xFF {
		t.Errorf("mask = %#x, want 0xFF", c.cells[0].mask)
	}
	if c.shape.Glyph(c.cells[0].mask) != '⣿' {
		t.Errorf("glyph = %q, want full braille block", c.shape.Glyph(c.cells[0].mask))
	}
}

func TestOutOfBoundsIgnored(t *testing.T) {
	c := New(Braille, 2, 2)
	c.Set(-1, 0, Color{})
	c.Set(0, -1, Color{})
	c.Set(100, 100, Color{})
	for _, cl := range c.cells {
		if cl.mask != 0 {
			t.Fatalf("expected no dots set, got mask %#x", cl.mask)
		}
	}
}

func TestClearResetsCanvas(t *testing.T) {
	c := New(Braille, 2, 2)
	c.Set(0, 0, Color{R: 9})
	c.Clear()
	for _, cl := range c.cells {
		if cl.mask != 0 || cl.hasColor {
			t.Fatalf("expected cleared canvas, got %+v", cl)
		}
	}
}

func TestLineDrawsEndpoints(t *testing.T) {
	c := New(Braille, 4, 4)
	c.Line(0, 0, c.Width()-1, c.Height()-1, Color{R: 1})
	idxStart := 0
	idxEnd := len(c.cells) - 1
	if c.cells[idxStart].mask == 0 {
		t.Error("expected start cell to have a dot set")
	}
	if c.cells[idxEnd].mask == 0 {
		t.Error("expected end cell to have a dot set")
	}
}

func countLitDots(c *Canvas) int {
	n := 0
	for _, cl := range c.cells {
		for b := cl.mask; b != 0; b &= b - 1 {
			n++
		}
	}
	return n
}

func TestLineWidthAtMostOneMatchesLine(t *testing.T) {
	a := New(Braille, 6, 6)
	a.Line(0, 0, a.Width()-1, a.Height()-1, Color{R: 1})

	b := New(Braille, 6, 6)
	b.LineWidth(0, 0, b.Width()-1, b.Height()-1, 1, Color{R: 1})

	if countLitDots(a) != countLitDots(b) {
		t.Errorf("LineWidth(width=1) lit %d dots, Line lit %d dots, want equal", countLitDots(b), countLitDots(a))
	}
}

func TestLineWidthThickerCoversMoreDots(t *testing.T) {
	thin := New(Braille, 10, 10)
	thin.Line(0, thin.Height()/2, thin.Width()-1, thin.Height()/2, Color{R: 1})

	thick := New(Braille, 10, 10)
	thick.LineWidth(0, thick.Height()/2, thick.Width()-1, thick.Height()/2, 4, Color{R: 1})

	if countLitDots(thick) <= countLitDots(thin) {
		t.Errorf("expected thick line to light more dots than thin line: thick=%d thin=%d", countLitDots(thick), countLitDots(thin))
	}
}

func TestTextOverridesDots(t *testing.T) {
	c := New(Braille, 5, 1)
	c.Set(0, 0, Color{R: 1})
	c.Text(0, 0, "hi", Color{G: 1})
	frame := c.Frame()
	if len(frame) == 0 {
		t.Fatal("expected non-empty frame")
	}
	if c.cells[0].text != 'h' || c.cells[1].text != 'i' {
		t.Errorf("text not written into cells: %+v %+v", c.cells[0], c.cells[1])
	}
}

func TestResize(t *testing.T) {
	c := New(Braille, 2, 2)
	c.Set(0, 0, Color{R: 1})
	c.Resize(3, 3)
	if c.Cols() != 3 || c.Rows() != 3 {
		t.Errorf("Cols/Rows after resize = %d/%d, want 3/3", c.Cols(), c.Rows())
	}
	for _, cl := range c.cells {
		if cl.mask != 0 {
			t.Fatal("expected resize to clear contents")
		}
	}
}

func TestBlocksShapeResolutionAndGlyphs(t *testing.T) {
	c := New(Blocks, 1, 1)
	if c.Width() != 2 || c.Height() != 2 {
		t.Fatalf("Width/Height = %d/%d, want 2/2", c.Width(), c.Height())
	}
	c.Set(0, 0, Color{R: 1}) // top-left
	c.Set(1, 1, Color{R: 1}) // bottom-right
	got := c.shape.Glyph(c.cells[0].mask)
	if got != '▚' {
		t.Errorf("glyph for top-left+bottom-right = %q, want ▚", got)
	}

	full := New(Blocks, 1, 1)
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			full.Set(x, y, Color{R: 1})
		}
	}
	if g := full.shape.Glyph(full.cells[0].mask); g != '█' {
		t.Errorf("glyph for fully-lit block cell = %q, want █", g)
	}
}

func TestASCIIShapeUsesConfiguredChar(t *testing.T) {
	c := New(NewASCIIShape('*'), 1, 1)
	if c.Width() != 1 || c.Height() != 1 {
		t.Fatalf("Width/Height = %d/%d, want 1/1", c.Width(), c.Height())
	}
	c.Set(0, 0, Color{R: 1})
	frame := c.Frame()
	if want := "\x1b[38;2;1;0;0m*" + resetSeq; frame != want {
		t.Errorf("Frame() = %q, want %q", frame, want)
	}
}

func TestASCIIDefaultShapeUsesHash(t *testing.T) {
	c := New(ASCII, 1, 1)
	c.Set(0, 0, Color{R: 1})
	frame := c.Frame()
	if want := "\x1b[38;2;1;0;0m#" + resetSeq; frame != want {
		t.Errorf("Frame() = %q, want %q", frame, want)
	}
}

func TestPinMarkerDefaultsAndOverrides(t *testing.T) {
	if got := New(Braille, 1, 1).PinMarker(); got != '●' {
		t.Errorf("Braille PinMarker = %q, want ●", got)
	}
	if got := New(ASCII, 1, 1).PinMarker(); got != '*' {
		t.Errorf("ASCII PinMarker = %q, want *", got)
	}
	custom := Shape{Name: "custom", DotsX: 1, DotsY: 1, Bit: func(int, int) byte { return 1 }, Glyph: func(byte) rune { return '.' }}
	if got := New(custom, 1, 1).PinMarker(); got != '●' {
		t.Errorf("zero-value PinMarker should fall back to ●, got %q", got)
	}
}

func TestVerticalScale(t *testing.T) {
	cases := []struct {
		name  string
		shape Shape
		want  float64
	}{
		{"braille (square 2x4 dots, no correction needed)", Braille, 1},
		{"blocks (square 2x2 dots, needs halving)", Blocks, 0.5},
		{"ascii (square 1x1 dots, needs halving)", ASCII, 0.5},
		{"zero-value Shape defaults to 1", Shape{}, 1},
	}
	for _, c := range cases {
		if got := c.shape.VerticalScale(); got != c.want {
			t.Errorf("%s: VerticalScale() = %v, want %v", c.name, got, c.want)
		}
		if got := New(c.shape, 1, 1).YScale(); got != c.want {
			t.Errorf("%s: Canvas.YScale() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOverlayHidesBaseGlyphAndColor(t *testing.T) {
	c := New(Braille, 1, 1)
	// Light every base dot, as a "fully filled" base-map cell (e.g. a
	// water polygon) would.
	for y := 0; y < Braille.DotsY; y++ {
		for x := 0; x < Braille.DotsX; x++ {
			c.Set(x, y, Color{B: 200})
		}
	}
	// A single overlay dot should completely replace the display for
	// this cell: only the overlay's (much sparser) glyph and its own
	// color, nothing blended in from the base plane.
	c.SetOverlay(0, 0, Color{R: 200})

	frame := c.Frame()
	want := "\x1b[38;2;200;0;0m" + string(rune(brailleBase+0x01)) + resetSeq
	if frame != want {
		t.Errorf("Frame() = %q, want %q (overlay should fully hide the base plane)", frame, want)
	}
}

func TestOverlayLineOnly(t *testing.T) {
	c := New(Braille, 4, 4)
	c.LineOverlay(0, 0, c.Width()-1, c.Height()-1, Color{R: 1})
	for _, cl := range c.cells {
		if cl.mask != 0 {
			t.Errorf("expected LineOverlay to leave the base plane untouched, got base mask %#x", cl.mask)
		}
	}
	if c.cells[0].overlayMask == 0 {
		t.Error("expected LineOverlay to light the start cell's overlay plane")
	}
	if c.cells[len(c.cells)-1].overlayMask == 0 {
		t.Error("expected LineOverlay to light the end cell's overlay plane")
	}
}

func TestLineWidthOverlayThickerCoversMoreDots(t *testing.T) {
	countOverlayDots := func(c *Canvas) int {
		n := 0
		for _, cl := range c.cells {
			for b := cl.overlayMask; b != 0; b &= b - 1 {
				n++
			}
		}
		return n
	}

	thin := New(Braille, 10, 10)
	thin.LineWidthOverlay(0, thin.Height()/2, thin.Width()-1, thin.Height()/2, 1, Color{R: 1})

	thick := New(Braille, 10, 10)
	thick.LineWidthOverlay(0, thick.Height()/2, thick.Width()-1, thick.Height()/2, 4, Color{R: 1})

	if countOverlayDots(thick) <= countOverlayDots(thin) {
		t.Errorf("expected thicker overlay line to light more dots: thick=%d thin=%d", countOverlayDots(thick), countOverlayDots(thin))
	}
}

func TestTextBeatsOverlayBeatsBase(t *testing.T) {
	c := New(Braille, 1, 1)
	c.Set(0, 0, Color{B: 1})
	c.SetOverlay(0, 0, Color{R: 1})
	if c.Frame() == New(Braille, 1, 1).Frame() {
		t.Fatal("sanity check failed: expected a non-blank frame")
	}
	beforeText := c.Frame()
	if !strings.Contains(beforeText, "38;2;1;0;0") {
		t.Errorf("expected overlay color (red) to win over base color (blue), got %q", beforeText)
	}

	c.Text(0, 0, "X", Color{G: 1})
	afterText := c.Frame()
	if !strings.Contains(afterText, "X") || !strings.Contains(afterText, "38;2;0;1;0") {
		t.Errorf("expected text to win over both overlay and base, got %q", afterText)
	}
}
