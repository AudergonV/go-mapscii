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

	// YScale corrects the map's vertical world-to-dot mapping for
	// shapes whose dot grid isn't naturally square on a standard
	// terminal cell (about twice as tall as wide). Braille's 2x4 grid
	// already matches that aspect (DotsY/DotsX == 2, the same ratio
	// as the cell itself), so it needs no correction. A shape with a
	// squarer dot grid than that - like Blocks' 2x2 or ASCII's 1x1 -
	// renders each dot physically taller than wide, which stretches
	// the map vertically unless compensated by setting YScale below
	// 1 to compress the vertical mapping by the same factor. 0 means
	// "no correction" (equivalent to 1).
	YScale float64
}

// VerticalScale returns YScale, defaulting to 1 (no correction) when
// unset.
func (s Shape) VerticalScale() float64 {
	if s.YScale == 0 {
		return 1
	}
	return s.YScale
}

type cell struct {
	mask     byte
	color    Color
	hasColor bool

	// overlayMask/overlayColor hold the "overlay" dot-plane, drawn
	// with SetOverlay/LineOverlay/LineWidthOverlay. When any overlay
	// dot is lit, the cell displays the overlay's glyph and color
	// only - the base plane's dots and color are completely hidden
	// underneath, rather than merged. This is what lets Map.DrawLine
	// read as a distinct line drawn strictly on top of the base map,
	// even on a Shape (like ASCII) too coarse to show both a base
	// feature's dot and an overlay dot in the same cell at once.
	overlayMask  byte
	overlayColor Color
	hasOverlay   bool

	text    rune
	hasText bool
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

// YScale returns the Shape's vertical aspect-ratio correction; see
// Shape.VerticalScale.
func (c *Canvas) YScale() float64 { return c.shape.VerticalScale() }

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

// SetOverlay lights up a dot on the overlay plane at the given
// sub-pixel coordinate. Any cell touched by an overlay dot displays
// only the overlay's glyph and color, completely hiding whatever the
// base plane (Set/Line/LineWidth) drew in that cell - see the cell
// struct's overlayMask field for why. Coordinates outside the canvas
// are ignored.
func (c *Canvas) SetOverlay(x, y int, color Color) {
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
	c.cells[idx].overlayMask |= c.shape.Bit(dx, dy)
	c.cells[idx].overlayColor = color
	c.cells[idx].hasOverlay = true
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

// LineOverlay draws a straight line on the overlay plane; see
// SetOverlay for how overlay dots take over their cell entirely.
func (c *Canvas) LineOverlay(x0, y0, x1, y1 int, color Color) {
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
		c.SetOverlay(x, y, color)
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
	stackedLines(c.Line, x0, y0, x1, y1, width, color)
}

// LineWidthOverlay draws a line on the overlay plane with the given
// thickness; see SetOverlay for how overlay dots take over their cell
// entirely.
func (c *Canvas) LineWidthOverlay(x0, y0, x1, y1 int, width float64, color Color) {
	stackedLines(c.LineOverlay, x0, y0, x1, y1, width, color)
}

// stackedLines implements the thickness approximation shared by
// LineWidth and LineWidthOverlay: it stacks several 1-dot lines
// (drawn via draw1px) offset perpendicular to the line's direction.
// This has no anti-aliasing, but is cheap and looks reasonable at the
// resolutions a terminal renders.
func stackedLines(draw1px func(x0, y0, x1, y1 int, color Color), x0, y0, x1, y1 int, width float64, color Color) {
	if width <= 1 {
		draw1px(x0, y0, x1, y1, color)
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
		draw1px(x0+ox, y0+oy, x1+ox, y1+oy, color)
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
//
// A cell renders its topmost non-empty layer only, never a blend:
// text (pins/labels) beats the overlay plane (drawn lines), which
// beats the base plane (the map itself) - see the cell struct's
// overlayMask field for why overlay dots hide the base plane outright
// instead of merging into the same glyph.
func (c *Canvas) Frame() string {
	var b strings.Builder
	for y := 0; y < c.rows; y++ {
		var lastColor Color
		haveColor := false
		for x := 0; x < c.cols; x++ {
			idx := y*c.cols + x
			cl := c.cells[idx]

			var r rune
			var color Color
			var hasColor bool
			switch {
			case cl.hasText:
				r, color, hasColor = cl.text, cl.color, cl.hasColor
			case cl.hasOverlay:
				r, color, hasColor = c.shape.Glyph(cl.overlayMask), cl.overlayColor, true
			case cl.mask != 0:
				r, color, hasColor = c.shape.Glyph(cl.mask), cl.color, cl.hasColor
			default:
				r = ' '
			}

			if hasColor && r != ' ' {
				if !haveColor || color != lastColor {
					writeColor(&b, color)
					lastColor = color
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
