// Package braille implements a terminal drawing surface built on top of
// Unicode Braille Patterns (U+2800-U+28FF). Each terminal character cell
// holds a 2x4 grid of dots, giving 2x horizontal and 4x vertical
// resolution compared to plain character-cell drawing. This is the same
// technique used by drawille and by mapscii's own Canvas.js.
package braille

import "strings"

// brailleBase is the Unicode code point of the "all dots off" braille
// pattern; individual dots are turned on by OR-ing a bit mask onto it.
const brailleBase = 0x2800

// dotMask maps a dot's position within a cell (row 0-3, col 0-1) to the
// bit that must be set in the braille pattern byte to light it up. This
// is the standard braille/drawille dot numbering:
//
//	0 3
//	1 4
//	2 5
//	6 7
var dotMask = [4][2]byte{
	{0x01, 0x08},
	{0x02, 0x10},
	{0x04, 0x20},
	{0x40, 0x80},
}

// DotsPerCellX and DotsPerCellY describe the sub-pixel resolution of a
// single terminal character cell.
const (
	DotsPerCellX = 2
	DotsPerCellY = 4
)

// Color is an RGB truecolor value used to paint canvas cells.
type Color struct {
	R, G, B uint8
}

type cell struct {
	mask     byte
	color    Color
	hasColor bool
	text     rune
	hasText  bool
}

// Canvas is a braille drawing surface addressed in sub-pixel ("dot")
// coordinates. A canvas created with NewCanvas(cols, rows) exposes a dot
// grid of cols*DotsPerCellX by rows*DotsPerCellY points.
type Canvas struct {
	cols, rows int
	cells      []cell
}

// NewCanvas creates a canvas sized to fit cols x rows terminal character
// cells.
func NewCanvas(cols, rows int) *Canvas {
	if cols < 0 {
		cols = 0
	}
	if rows < 0 {
		rows = 0
	}
	return &Canvas{
		cols:  cols,
		rows:  rows,
		cells: make([]cell, cols*rows),
	}
}

// Cols and Rows report the canvas size in terminal character cells.
func (c *Canvas) Cols() int { return c.cols }
func (c *Canvas) Rows() int { return c.rows }

// Width and Height report the canvas size in dot (sub-pixel) coordinates.
func (c *Canvas) Width() int  { return c.cols * DotsPerCellX }
func (c *Canvas) Height() int { return c.rows * DotsPerCellY }

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
	cellX, cellY := x/DotsPerCellX, y/DotsPerCellY
	dotX, dotY := x%DotsPerCellX, y%DotsPerCellY
	idx, ok := c.cellIndex(cellX, cellY)
	if !ok {
		return
	}
	c.cells[idx].mask |= dotMask[dotY][dotX]
	c.cells[idx].color = color
	c.cells[idx].hasColor = true
}

// Unset turns off the dot at the given sub-pixel coordinate without
// affecting the cell's color.
func (c *Canvas) Unset(x, y int) {
	if x < 0 || y < 0 || x >= c.Width() || y >= c.Height() {
		return
	}
	cellX, cellY := x/DotsPerCellX, y/DotsPerCellY
	dotX, dotY := x%DotsPerCellX, y%DotsPerCellY
	idx, ok := c.cellIndex(cellX, cellY)
	if !ok {
		return
	}
	c.cells[idx].mask &^= dotMask[dotY][dotX]
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

// Text writes a literal string starting at the given cell coordinate,
// overriding any braille dots at those cells. Text is drawn on top of
// the dot grid and is not affected by it.
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
				r = rune(brailleBase + int(cl.mask))
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
