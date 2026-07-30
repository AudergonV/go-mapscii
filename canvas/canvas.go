// Package canvas implements a generic terminal drawing surface. Its
// sub-pixel resolution and glyph set are supplied by a Shape, so the
// same drawing algorithms (dots, lines, text overlays) can render as
// Unicode braille, block-mosaic quadrants, plain ASCII, or any other
// character set a caller defines - see shapes.go for the built-ins.
package canvas

import (
	"math"
	"strings"
)

// Color is an RGB truecolor value used to paint canvas cells.
type Color struct {
	R, G, B uint8
}

// Shape defines a terminal cell's sub-pixel dot grid: how many dot
// columns and rows it holds, which mask bit a given dot position sets,
// how a mask renders as a single display rune, and which rune marks a
// pin. DotsX*DotsY must not exceed 8, since a cell's dot mask is a
// single byte.
type Shape struct {
	// Name identifies the shape, e.g. for flag values or logging.
	Name string
	// DotsX and DotsY are the sub-pixel resolution of one terminal
	// cell.
	DotsX, DotsY int
	// Bit returns the mask bit lit by the dot at position (dx,dy)
	// within a cell, where 0<=dx<DotsX and 0<=dy<DotsY. Every
	// (dx,dy) pair must map to a distinct bit.
	Bit func(dx, dy int) byte
	// Glyph renders a cell's dot mask (0 meaning an empty cell) as a
	// display rune.
	Glyph func(mask byte) rune
	// PinMarker is the rune used to mark a pin placed on the map.
	PinMarker rune
}

type cell struct {
	mask     byte
	color    Color
	hasColor bool
	text     rune
	hasText  bool
}

// Canvas is a drawing surface addressed in sub-pixel ("dot")
// coordinates, rendered through the rules of a Shape.
type Canvas struct {
	shape      Shape
	cols, rows int
	cells      []cell
}

// New creates a canvas sized to fit cols x rows terminal character
// cells, using the given Shape's sub-pixel resolution and glyph set.
func New(shape Shape, cols, rows int) *Canvas {
	if cols < 0 {
		cols = 0
	}
	if rows < 0 {
		rows = 0
	}
	return &Canvas{
		shape: shape,
		cols:  cols,
		rows:  rows,
		cells: make([]cell, cols*rows),
	}
}

// Shape returns the Shape this canvas was created with.
func (c *Canvas) Shape() Shape { return c.shape }

// Cols and Rows report the canvas size in terminal character cells.
func (c *Canvas) Cols() int { return c.cols }
func (c *Canvas) Rows() int { return c.rows }

// Width and Height report the canvas size in dot (sub-pixel) coordinates.
func (c *Canvas) Width() int  { return c.cols * c.shape.DotsX }
func (c *Canvas) Height() int { return c.rows * c.shape.DotsY }

// DotsPerCellX and DotsPerCellY report the Shape's sub-pixel resolution.
func (c *Canvas) DotsPerCellX() int { return c.shape.DotsX }
func (c *Canvas) DotsPerCellY() int { return c.shape.DotsY }

// PinMarker returns the rune this canvas's Shape uses to mark a pin,
// falling back to "●" if the Shape didn't set one.
func (c *Canvas) PinMarker() rune {
	if c.shape.PinMarker == 0 {
		return '●'
	}
	return c.shape.PinMarker
}

// Clear resets every dot, color and text override on the canvas.
func (c *Canvas) Clear() {
	for i := range c.cells {
		c.cells[i] = cell{}
	}
}

// Resize changes the canvas dimensions, discarding its contents.
func (c *Canvas) Resize(cols, rows int) {
	if cols < 0 {
		cols = 0
	}
	if rows < 0 {
		rows = 0
	}
	c.cols, c.rows = cols, rows
	c.cells = make([]cell, cols*rows)
}

func (c *Canvas) cellIndex(cellX, cellY int) (int, bool) {
	if cellX < 0 || cellY < 0 || cellX >= c.cols || cellY >= c.rows {
		return 0, false
	}
	return cellY*c.cols + cellX, true
}

// Set lights up the dot at the given sub-pixel coordinate, painting its
// containing cell with color. Coordinates outside the canvas are ignored.
func (c *Canvas) Set(x, y int, color Color) {
	if x < 0 || y < 0 || x >= c.Width() || y >= c.Height() {
		return
	}
	dotsX, dotsY := c.shape.DotsX, c.shape.DotsY
	cellX, cellY := x/dotsX, y/dotsY
	dx, dy := x%dotsX, y%dotsY
	idx, ok := c.cellIndex(cellX, cellY)
	if !ok {
		return
	}
	c.cells[idx].mask |= c.shape.Bit(dx, dy)
	c.cells[idx].color = color
	c.cells[idx].hasColor = true
}

// Unset turns off the dot at the given sub-pixel coordinate without
// affecting the cell's color.
func (c *Canvas) Unset(x, y int) {
	if x < 0 || y < 0 || x >= c.Width() || y >= c.Height() {
		return
	}
	dotsX, dotsY := c.shape.DotsX, c.shape.DotsY
	cellX, cellY := x/dotsX, y/dotsY
	dx, dy := x%dotsX, y%dotsY
	idx, ok := c.cellIndex(cellX, cellY)
	if !ok {
		return
	}
	c.cells[idx].mask &^= c.shape.Bit(dx, dy)
}

// Line draws a straight line between two sub-pixel coordinates using
// Bresenham's algorithm.
func (c *Canvas) Line(x0, y0, x1, y1 int, color Color) {
	dx := abs(x1 - x0)
	dy := -abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy

	x, y := x0, y0
	for {
		c.Set(x, y, color)
		if x == x1 && y == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x += sx
		}
		if e2 <= dx {
			err += dx
			y += sy
		}
	}
}

// LineWidth draws a straight line between two sub-pixel coordinates
// with the given thickness, expressed in dots. A width of 1 (or less)
// behaves exactly like Line. Thicker lines are approximated by
// stacking several 1-dot Bresenham lines offset perpendicular to the
// line's direction; this has no anti-aliasing, but is cheap and looks
// reasonable at the resolutions a terminal renders.
func (c *Canvas) LineWidth(x0, y0, x1, y1 int, width float64, color Color) {
	if width <= 1 {
		c.Line(x0, y0, x1, y1, color)
		return
	}

	dx, dy := float64(x1-x0), float64(y1-y0)
	length := math.Hypot(dx, dy)
	var nx, ny float64
	if length == 0 {
		nx, ny = 1, 0
	} else {
		nx, ny = -dy/length, dx/length
	}

	steps := int(math.Ceil(width))
	half := width / 2
	for i := 0; i < steps; i++ {
		offset := 0.0
		if steps > 1 {
			offset = -half + half*2*float64(i)/float64(steps-1)
		}
		ox := int(math.Round(nx * offset))
		oy := int(math.Round(ny * offset))
		c.Line(x0+ox, y0+oy, x1+ox, y1+oy, color)
	}
}

// Text writes a literal string starting at the given cell coordinate,
// overriding any dots at those cells. Text is drawn on top of the dot
// grid and is not affected by it.
func (c *Canvas) Text(cellX, cellY int, s string, color Color) {
	for i, r := range []rune(s) {
		idx, ok := c.cellIndex(cellX+i, cellY)
		if !ok {
			continue
		}
		c.cells[idx].text = r
		c.cells[idx].hasText = true
		c.cells[idx].color = color
		c.cells[idx].hasColor = true
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// Frame renders the canvas to a string of terminal rows separated by
// newlines, using 24-bit ANSI escape sequences to colorize cells that
// have a color set. Cells with no dots and no text render as spaces.
func (c *Canvas) Frame() string {
	var b strings.Builder
	for y := 0; y < c.rows; y++ {
		var lastColor Color
		haveColor := false
		for x := 0; x < c.cols; x++ {
			idx := y*c.cols + x
			cl := c.cells[idx]

			var r rune
			switch {
			case cl.hasText:
				r = cl.text
			case cl.mask != 0:
				r = c.shape.Glyph(cl.mask)
			default:
				r = ' '
			}

			if cl.hasColor && r != ' ' {
				if !haveColor || cl.color != lastColor {
					writeColor(&b, cl.color)
					lastColor = cl.color
					haveColor = true
				}
			} else if haveColor {
				b.WriteString(resetSeq)
				haveColor = false
			}

			b.WriteRune(r)
		}
		if haveColor {
			b.WriteString(resetSeq)
		}
		if y < c.rows-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

const resetSeq = "\x1b[0m"

func writeColor(b *strings.Builder, c Color) {
	b.WriteString("\x1b[38;2;")
	writeUint8(b, c.R)
	b.WriteByte(';')
	writeUint8(b, c.G)
	b.WriteByte(';')
	writeUint8(b, c.B)
	b.WriteByte('m')
}

func writeUint8(b *strings.Builder, v uint8) {
	if v >= 100 {
		b.WriteByte('0' + v/100)
		v %= 100
		b.WriteByte('0' + v/10)
		b.WriteByte('0' + v%10)
	} else if v >= 10 {
		b.WriteByte('0' + v/10)
		b.WriteByte('0' + v%10)
	} else {
		b.WriteByte('0' + v)
	}
}
