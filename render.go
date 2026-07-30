package mapscii

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/audergonv/go-mapscii/canvas"
	"github.com/audergonv/go-mapscii/geo"
	"github.com/audergonv/go-mapscii/style"
	"github.com/audergonv/go-mapscii/tileprovider"
	"github.com/audergonv/go-mapscii/vectortile"
)

// dotPt is a coordinate in canvas dot (sub-pixel) space; unlike
// canvas.Canvas's own integer dot coordinates, these stay in floating
// point until the moment they are drawn, so geometry can be positioned
// precisely before rounding.
type dotPt struct {
	X, Y float64
}

type geomKind int

const (
	kindPoint geomKind = iota
	kindLine
	kindPolygon
)

type drawCmd struct {
	priority int
	kind     geomKind
	color    canvas.Color
	width    float64
	parts    [][]dotPt
}

// Render draws the current viewport - base map plus any pins and lines
// - and returns it as a string of terminal rows containing the
// configured Shape's characters and 24-bit ANSI color escapes, ready
// to be printed directly to a terminal.
func (m *Map) Render(ctx context.Context) (string, error) {
	m.mu.RLock()
	center := m.center
	zoom := m.zoom
	width, height := m.width, m.height
	provider := m.provider
	sty := m.style
	shape := m.shape
	maxTileZoom := m.maxTileZoom
	pins := append([]*Pin(nil), m.pins...)
	lines := append([]*Line(nil), m.lines...)
	m.mu.RUnlock()

	cv := canvas.New(shape, width, height)

	tileZoom := int(math.Floor(zoom))
	if tileZoom > maxTileZoom {
		tileZoom = maxTileZoom
	}
	if tileZoom < 0 {
		tileZoom = 0
	}
	scale := math.Pow(2, zoom-float64(tileZoom))

	centerPoint := geo.LatLonToPoint(center, zoom)
	originX := centerPoint.X - float64(cv.Width())/2
	originY := centerPoint.Y - float64(cv.Height())/2

	cmds, err := collectTileDrawCommands(ctx, provider, sty, tileZoom, scale, originX, originY, cv)
	if err != nil {
		return "", err
	}

	sort.SliceStable(cmds, func(i, j int) bool {
		return cmds[i].priority < cmds[j].priority
	})

	w, h := float64(cv.Width()-1), float64(cv.Height()-1)
	for _, cmd := range cmds {
		drawCommand(cv, cmd, w, h)
	}

	drawOverlayLines(cv, lines, sty, zoom, originX, originY, w, h)
	drawOverlayPins(cv, pins, sty, zoom, originX, originY)

	return cv.Frame(), nil
}

// collectTileDrawCommands fetches every tile intersecting the viewport
// and converts their features into draw commands positioned in canvas
// dot space, without yet drawing them (drawing happens after a global
// priority sort, so e.g. water is always drawn under roads regardless
// of tile fetch order).
func collectTileDrawCommands(
	ctx context.Context,
	provider tileprovider.Provider,
	sty *style.Style,
	tileZoom int,
	scale, originX, originY float64,
	cv *canvas.Canvas,
) ([]drawCmd, error) {
	worldOriginX := originX / scale
	worldOriginY := originY / scale
	worldW := float64(cv.Width()) / scale
	worldH := float64(cv.Height()) / scale

	n := geo.TileCount(tileZoom)
	minTileX := clampInt(int(math.Floor(worldOriginX/geo.TileSize)), 0, n-1)
	maxTileX := clampInt(int(math.Floor((worldOriginX+worldW)/geo.TileSize)), 0, n-1)
	minTileY := clampInt(int(math.Floor(worldOriginY/geo.TileSize)), 0, n-1)
	maxTileY := clampInt(int(math.Floor((worldOriginY+worldH)/geo.TileSize)), 0, n-1)

	var cmds []drawCmd
	var lastErr error
	fetched := 0

	for ty := minTileY; ty <= maxTileY; ty++ {
		for tx := minTileX; tx <= maxTileX; tx++ {
			idx := geo.TileIndex{Z: tileZoom, X: tx, Y: ty}
			tile, err := provider.FetchTile(ctx, idx)
			if err != nil {
				lastErr = err
				continue
			}
			fetched++

			tileOrigin := geo.TileOrigin(idx)
			toDot := func(p vectortile.Point, extent uint32) dotPt {
				if extent == 0 {
					extent = 4096
				}
				wx := tileOrigin.X + float64(p.X)/float64(extent)*geo.TileSize
				wy := tileOrigin.Y + float64(p.Y)/float64(extent)*geo.TileSize
				return dotPt{X: wx*scale - originX, Y: wy*scale - originY}
			}

			for _, layer := range tile.Layers {
				rule, ok := sty.Rule(layer.Name)
				if !ok {
					continue
				}
				isRoad := layer.Name == "road" || layer.Name == "transportation"

				for _, feature := range layer.Features {
					color := rule.Color
					width := rule.Width
					if isRoad {
						class := roadClass(feature)
						color = sty.RoadColor(class, color)
						width = sty.RoadWidth(class, width)
					}

					parts := make([][]dotPt, 0, len(feature.Geometry))
					for _, part := range feature.Geometry {
						dp := make([]dotPt, len(part))
						for i, p := range part {
							dp[i] = toDot(p, layer.Extent)
						}
						parts = append(parts, dp)
					}
					if len(parts) == 0 {
						continue
					}

					var kind geomKind
					switch feature.Type {
					case vectortile.GeomPolygon:
						kind = kindPolygon
					case vectortile.GeomLineString:
						kind = kindLine
					default:
						kind = kindPoint
					}

					cmds = append(cmds, drawCmd{
						priority: rule.Priority,
						kind:     kind,
						color:    color,
						width:    width,
						parts:    parts,
					})
				}
			}
		}
	}

	if fetched == 0 && lastErr != nil {
		return nil, fmt.Errorf("mapscii: fetching tiles: %w", lastErr)
	}
	return cmds, nil
}

func roadClass(f vectortile.Feature) string {
	for _, key := range []string{"class", "highway", "type"} {
		if v, ok := f.Tags[key]; ok {
			return v.String()
		}
	}
	return ""
}

func drawCommand(cv *canvas.Canvas, cmd drawCmd, maxX, maxY float64) {
	switch cmd.kind {
	case kindPolygon:
		fillPolygon(cv, cmd.parts, cmd.color)
	case kindLine:
		for _, part := range cmd.parts {
			for i := 0; i+1 < len(part); i++ {
				drawClippedLine(cv, part[i], part[i+1], cmd.color, cmd.width, maxX, maxY)
			}
		}
	case kindPoint:
		for _, part := range cmd.parts {
			for _, p := range part {
				cv.Set(int(math.Round(p.X)), int(math.Round(p.Y)), cmd.color)
			}
		}
	}
}

func drawOverlayLines(cv *canvas.Canvas, lines []*Line, sty *style.Style, zoom, originX, originY, maxX, maxY float64) {
	for _, line := range lines {
		color := sty.DefaultLineColor
		if line.hasColor {
			color = line.Color
		}
		width := sty.DefaultLineWidth
		if line.hasWidth {
			width = line.Width
		}
		var prev dotPt
		has := false
		for _, ll := range line.Points {
			p := geo.LatLonToPoint(ll, zoom)
			cur := dotPt{X: p.X - originX, Y: p.Y - originY}
			if has {
				drawClippedLine(cv, prev, cur, color, width, maxX, maxY)
			}
			prev, has = cur, true
		}
	}
}

func drawOverlayPins(cv *canvas.Canvas, pins []*Pin, sty *style.Style, zoom, originX, originY float64) {
	for _, pin := range pins {
		markerColor := sty.PinColor
		if pin.hasColor {
			markerColor = pin.Color
		}
		p := geo.LatLonToPoint(pin.Pos, zoom)
		dotX, dotY := p.X-originX, p.Y-originY
		cellX := int(math.Round(dotX)) / cv.DotsPerCellX()
		cellY := int(math.Round(dotY)) / cv.DotsPerCellY()

		cv.Text(cellX, cellY, string(cv.PinMarker()), markerColor)
		if pin.Label != "" {
			cv.Text(cellX+1, cellY, " "+pin.Label, sty.PinLabelColor)
		}
	}
}

// drawClippedLine clips a segment to the canvas bounds before handing
// it to the canvas's Bresenham line drawer, so that segments which
// mostly lie far outside the viewport don't cost time proportional to
// their (potentially huge) off-screen length.
func drawClippedLine(cv *canvas.Canvas, a, b dotPt, color canvas.Color, width, maxX, maxY float64) {
	x0, y0, x1, y1, ok := clipSegment(a.X, a.Y, b.X, b.Y, 0, 0, maxX, maxY)
	if !ok {
		return
	}
	cv.LineWidth(int(math.Round(x0)), int(math.Round(y0)), int(math.Round(x1)), int(math.Round(y1)), width, color)
}

// clipSegment implements Cohen-Sutherland line clipping against the
// rectangle [minX,maxX] x [minY,maxY].
func clipSegment(x0, y0, x1, y1, minX, minY, maxX, maxY float64) (cx0, cy0, cx1, cy1 float64, ok bool) {
	const (
		inside = 0
		left   = 1
		right  = 2
		bottom = 4
		top    = 8
	)
	code := func(x, y float64) int {
		c := inside
		switch {
		case x < minX:
			c |= left
		case x > maxX:
			c |= right
		}
		switch {
		case y < minY:
			c |= bottom
		case y > maxY:
			c |= top
		}
		return c
	}

	c0, c1 := code(x0, y0), code(x1, y1)
	for {
		if c0 == 0 && c1 == 0 {
			return x0, y0, x1, y1, true
		}
		if c0&c1 != 0 {
			return 0, 0, 0, 0, false
		}

		out := c0
		if out == 0 {
			out = c1
		}

		var x, y float64
		switch {
		case out&top != 0:
			x = x0 + (x1-x0)*(maxY-y0)/(y1-y0)
			y = maxY
		case out&bottom != 0:
			x = x0 + (x1-x0)*(minY-y0)/(y1-y0)
			y = minY
		case out&right != 0:
			y = y0 + (y1-y0)*(maxX-x0)/(x1-x0)
			x = maxX
		case out&left != 0:
			y = y0 + (y1-y0)*(minX-x0)/(x1-x0)
			x = minX
		}

		if out == c0 {
			x0, y0 = x, y
			c0 = code(x0, y0)
		} else {
			x1, y1 = x, y
			c1 = code(x1, y1)
		}
	}
}

// fillPolygon fills the given rings using an even-odd scanline rule at
// full canvas dot resolution, so it also correctly leaves holes (e.g.
// a lake's interior ring within a landuse polygon) unfilled.
func fillPolygon(cv *canvas.Canvas, rings [][]dotPt, color canvas.Color) {
	if len(rings) == 0 {
		return
	}

	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, ring := range rings {
		for _, p := range ring {
			if p.Y < minY {
				minY = p.Y
			}
			if p.Y > maxY {
				maxY = p.Y
			}
		}
	}
	if math.IsInf(minY, 1) {
		return
	}

	startY := clampInt(int(math.Floor(minY)), 0, cv.Height()-1)
	endY := clampInt(int(math.Ceil(maxY)), 0, cv.Height()-1)

	var xs []float64
	for y := startY; y <= endY; y++ {
		fy := float64(y)
		xs = xs[:0]
		for _, ring := range rings {
			n := len(ring)
			for i := 0; i < n; i++ {
				p1 := ring[i]
				p2 := ring[(i+1)%n]
				if p1.Y == p2.Y {
					continue
				}
				if (p1.Y <= fy && p2.Y > fy) || (p2.Y <= fy && p1.Y > fy) {
					t := (fy - p1.Y) / (p2.Y - p1.Y)
					xs = append(xs, p1.X+t*(p2.X-p1.X))
				}
			}
		}
		sort.Float64s(xs)
		for i := 0; i+1 < len(xs); i += 2 {
			x0 := clampInt(int(math.Round(xs[i])), 0, cv.Width()-1)
			x1 := clampInt(int(math.Round(xs[i+1])), 0, cv.Width()-1)
			for x := x0; x <= x1; x++ {
				cv.Set(x, y, color)
			}
		}
	}
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		lo, hi = hi, lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
