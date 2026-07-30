package braille

import "testing"

func TestNewCanvasDimensions(t *testing.T) {
	c := NewCanvas(10, 5)
	if c.Width() != 20 || c.Height() != 20 {
		t.Errorf("Width/Height = %d/%d, want 20/20", c.Width(), c.Height())
	}
}

func TestSetSingleDot(t *testing.T) {
	c := NewCanvas(1, 1)
	c.Set(0, 0, Color{R: 255})
	frame := c.Frame()
	want := "\x1b[38;2;255;0;0m" + string(rune(brailleBase+0x01)) + resetSeq
	if frame != want {
		t.Errorf("Frame() = %q, want %q", frame, want)
	}
}

func TestAllDotsFillCell(t *testing.T) {
	c := NewCanvas(1, 1)
	for y := 0; y < DotsPerCellY; y++ {
		for x := 0; x < DotsPerCellX; x++ {
			c.Set(x, y, Color{R: 1, G: 2, B: 3})
		}
	}
	if c.cells[0].mask != 0xFF {
		t.Errorf("mask = %#x, want 0xFF", c.cells[0].mask)
	}
}

func TestOutOfBoundsIgnored(t *testing.T) {
	c := NewCanvas(2, 2)
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
	c := NewCanvas(2, 2)
	c.Set(0, 0, Color{R: 9})
	c.Clear()
	for _, cl := range c.cells {
		if cl.mask != 0 || cl.hasColor {
			t.Fatalf("expected cleared canvas, got %+v", cl)
		}
	}
}

func TestLineDrawsEndpoints(t *testing.T) {
	c := NewCanvas(4, 4)
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

func TestTextOverridesDots(t *testing.T) {
	c := NewCanvas(5, 1)
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
	c := NewCanvas(2, 2)
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
