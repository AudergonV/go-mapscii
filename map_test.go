package mapscii

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/audergonv/go-mapscii/canvas"
	"github.com/audergonv/go-mapscii/geo"
	"github.com/audergonv/go-mapscii/tileprovider"
	"github.com/audergonv/go-mapscii/vectortile"
)

func newTestMap(t *testing.T, center LatLon, zoom float64) (*Map, *tileprovider.MemoryProvider) {
	t.Helper()
	provider := tileprovider.NewMemoryProvider()

	// Populate every tile touching the requested zoom level with a
	// simple square "water" polygon and a diagonal "road" line, so
	// Render always has something to draw regardless of the exact
	// viewport math.
	tile := &vectortile.Tile{
		Layers: []vectortile.Layer{
			{
				Name:   "water",
				Extent: 4096,
				Features: []vectortile.Feature{
					{
						Type: vectortile.GeomPolygon,
						Geometry: [][]vectortile.Point{
							{{X: 0, Y: 0}, {X: 4096, Y: 0}, {X: 4096, Y: 4096}, {X: 0, Y: 4096}, {X: 0, Y: 0}},
						},
					},
				},
			},
			{
				Name:   "road",
				Extent: 4096,
				Features: []vectortile.Feature{
					{
						Type: vectortile.GeomLineString,
						Tags: map[string]vectortile.Value{},
						Geometry: [][]vectortile.Point{
							{{X: 0, Y: 0}, {X: 4096, Y: 4096}},
						},
					},
				},
			},
		},
	}

	tileZoom := int(zoom)
	if tileZoom > DefaultMaxTileZoom {
		tileZoom = DefaultMaxTileZoom
	}
	centerTile := geo.TileAt(center, tileZoom)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			provider.Set(geo.TileIndex{Z: tileZoom, X: centerTile.X + dx, Y: centerTile.Y + dy}, tile)
		}
	}

	m, err := New(Options{
		Provider: provider,
		Width:    40,
		Height:   20,
		Center:   center,
		Zoom:     zoom,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m, provider
}

func TestNewRequiresProvider(t *testing.T) {
	_, err := New(Options{})
	if err == nil {
		t.Fatal("expected error when Provider is nil")
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	m, err := New(Options{Provider: tileprovider.NewMemoryProvider()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	w, h := m.Size()
	if w != 80 || h != 24 {
		t.Errorf("Size() = %d,%d, want 80,24", w, h)
	}
	if m.Zoom() != DefaultZoom {
		t.Errorf("Zoom() = %v, want %v", m.Zoom(), DefaultZoom)
	}
	if m.Shape().Name != canvas.Braille.Name {
		t.Errorf("Shape() = %v, want braille (the default)", m.Shape().Name)
	}
}

func TestNewHonorsCustomShape(t *testing.T) {
	m, err := New(Options{Provider: tileprovider.NewMemoryProvider(), Shape: canvas.ASCII})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.Shape().Name != canvas.ASCII.Name {
		t.Errorf("Shape() = %v, want ascii", m.Shape().Name)
	}
}

func TestSetShapeChangesRenderOutput(t *testing.T) {
	m, _ := newTestMap(t, LatLon{Lat: 48.8566, Lon: 2.3522}, 10)

	brailleFrame, err := m.Render(context.Background())
	if err != nil {
		t.Fatalf("Render (braille): %v", err)
	}

	m.SetShape(canvas.ASCII)
	if m.Shape().Name != canvas.ASCII.Name {
		t.Fatalf("Shape() after SetShape = %v, want ascii", m.Shape().Name)
	}
	asciiFrame, err := m.Render(context.Background())
	if err != nil {
		t.Fatalf("Render (ascii): %v", err)
	}

	if brailleFrame == asciiFrame {
		t.Error("expected switching Shape to change the rendered output")
	}
	if strings.ContainsRune(asciiFrame, '⣿') {
		t.Error("expected ascii-shaped render to contain no braille glyphs")
	}
}

func TestSetShapeRejectsZeroValue(t *testing.T) {
	m, _ := newTestMap(t, LatLon{}, 5)
	m.SetShape(canvas.Shape{})
	if m.Shape().Name != canvas.Braille.Name {
		t.Errorf("SetShape(zero value) should fall back to braille, got %v", m.Shape().Name)
	}
}

func TestRenderProducesNonEmptyOutput(t *testing.T) {
	m, _ := newTestMap(t, LatLon{Lat: 48.8566, Lon: 2.3522}, 10)
	out, err := m.Render(context.Background())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.TrimSpace(strings.ReplaceAll(out, "\n", "")) == "" {
		t.Fatal("expected non-blank render output")
	}
	lines := strings.Split(out, "\n")
	if len(lines) != 20 {
		t.Errorf("expected 20 rows, got %d", len(lines))
	}
}

func TestRenderIncludesPinLabel(t *testing.T) {
	m, _ := newTestMap(t, LatLon{Lat: 48.8566, Lon: 2.3522}, 10)
	m.AddPin(48.8566, 2.3522, "Paris")
	out, err := m.Render(context.Background())
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "Paris") {
		t.Error("expected rendered output to contain pin label \"Paris\"")
	}
}

func TestSetCenterAndZoomClamping(t *testing.T) {
	m, _ := newTestMap(t, LatLon{}, 5)
	m.SetZoom(1000)
	if m.Zoom() != m.maxZoom {
		t.Errorf("Zoom() = %v, want clamped to maxZoom %v", m.Zoom(), m.maxZoom)
	}
	m.SetZoom(-1000)
	if m.Zoom() != m.minZoom {
		t.Errorf("Zoom() = %v, want clamped to minZoom %v", m.Zoom(), m.minZoom)
	}

	m.SetCenter(10, 20)
	c := m.Center()
	if c.Lat != 10 || c.Lon != 20 {
		t.Errorf("Center() = %+v, want {10 20}", c)
	}
}

func TestAddAndRemovePin(t *testing.T) {
	m, _ := newTestMap(t, LatLon{}, 5)
	p := m.AddPin(1, 2, "test")
	if len(m.Pins()) != 1 {
		t.Fatalf("expected 1 pin, got %d", len(m.Pins()))
	}
	m.RemovePin(p)
	if len(m.Pins()) != 0 {
		t.Fatalf("expected 0 pins after removal, got %d", len(m.Pins()))
	}
}

func TestDrawAndRemoveLine(t *testing.T) {
	m, _ := newTestMap(t, LatLon{}, 5)
	l := m.DrawLine([]LatLon{{Lat: 0, Lon: 0}, {Lat: 1, Lon: 1}})
	if len(m.Lines()) != 1 {
		t.Fatalf("expected 1 line, got %d", len(m.Lines()))
	}
	m.RemoveLine(l)
	if len(m.Lines()) != 0 {
		t.Fatalf("expected 0 lines after removal, got %d", len(m.Lines()))
	}
}

func TestClearOverlays(t *testing.T) {
	m, _ := newTestMap(t, LatLon{}, 5)
	m.AddPin(1, 2, "p")
	m.DrawLine([]LatLon{{Lat: 0, Lon: 0}, {Lat: 1, Lon: 1}})
	m.ClearOverlays()
	if len(m.Pins()) != 0 || len(m.Lines()) != 0 {
		t.Fatal("expected ClearOverlays to remove all pins and lines")
	}
}

func TestResize(t *testing.T) {
	m, _ := newTestMap(t, LatLon{}, 5)
	m.Resize(100, 50)
	w, h := m.Size()
	if w != 100 || h != 50 {
		t.Errorf("Size() = %d,%d, want 100,50", w, h)
	}
}

func TestPanMovesCenter(t *testing.T) {
	m, _ := newTestMap(t, LatLon{Lat: 0, Lon: 0}, 8)
	before := m.Center()
	m.Pan(10, 0)
	after := m.Center()
	if after.Lon <= before.Lon {
		t.Errorf("expected panning right to increase longitude: before=%v after=%v", before.Lon, after.Lon)
	}
}

var ansiEscape = regexp.MustCompile("\x1b\\[[0-9;]*m")

func countGlyphs(frame string) int {
	stripped := ansiEscape.ReplaceAllString(frame, "")
	n := 0
	for _, r := range stripped {
		if r != ' ' && r != '\n' {
			n++
		}
	}
	return n
}

func TestDrawLineWidthThickensRender(t *testing.T) {
	// An empty provider means the only thing Render draws is the
	// overlay line itself, so counting drawn glyphs directly measures
	// how much of the canvas the line covers.
	provider := tileprovider.NewMemoryProvider()

	newLine := func(opts ...LineOption) string {
		m, err := New(Options{
			Provider: provider,
			Width:    60,
			Height:   30,
			Center:   LatLon{Lat: 10, Lon: 10},
			Zoom:     8,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		m.DrawLine([]LatLon{{Lat: 10.05, Lon: 9.9}, {Lat: 9.95, Lon: 10.1}}, opts...)
		frame, err := m.Render(context.Background())
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		return frame
	}

	thin := newLine()
	thick := newLine(WithLineWidth(6))

	if countGlyphs(thick) <= countGlyphs(thin) {
		t.Errorf("expected WithLineWidth(6) to draw more glyphs than the default width: thick=%d thin=%d",
			countGlyphs(thick), countGlyphs(thin))
	}
}

// findMarkerRow returns the 0-indexed row of the first cell in frame
// containing marker, ignoring ANSI escapes.
func findMarkerRow(t *testing.T, frame string, marker rune) int {
	t.Helper()
	for i, line := range strings.Split(frame, "\n") {
		if strings.ContainsRune(ansiEscape.ReplaceAllString(line, ""), marker) {
			return i
		}
	}
	t.Fatalf("marker %q not found in rendered frame:\n%s", marker, ansiEscape.ReplaceAllString(frame, ""))
	return -1
}

// TestVerticalAspectCorrection checks the fix for shapes (like Blocks
// and ASCII) whose dot grid is square even though a terminal cell
// isn't (~1:2 width:height): without correcting for that, geographic
// content renders vertically stretched. A pin placed at an exact,
// known north offset from the map center should land half as many
// dot-rows away under Blocks as under Braille, since Blocks halves
// the vertical world-to-dot mapping to compensate.
func TestVerticalAspectCorrection(t *testing.T) {
	provider := tileprovider.NewMemoryProvider()
	center := LatLon{Lat: 0, Lon: 0}
	const zoom = 10.0

	centerPt := geo.LatLonToPoint(center, zoom)
	const offsetPixels = 80.0
	north := geo.PointToLatLon(geo.Point{X: centerPt.X, Y: centerPt.Y - offsetPixels}, zoom)

	rowOffsetInDots := func(shape canvas.Shape) int {
		m, err := New(Options{
			Provider: provider, Width: 100, Height: 100,
			Center: center, Zoom: zoom, Shape: shape,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		m.AddPin(north.Lat, north.Lon, "")
		frame, err := m.Render(context.Background())
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		row := findMarkerRow(t, frame, shape.PinMarker)
		centerRow := m.height / 2
		return (centerRow - row) * shape.DotsY
	}

	brailleDots := rowOffsetInDots(canvas.Braille)
	blocksDots := rowOffsetInDots(canvas.Blocks)

	if brailleDots <= 0 || blocksDots <= 0 {
		t.Fatalf("expected both shapes to place the pin above center: braille=%d blocks=%d", brailleDots, blocksDots)
	}

	ratio := float64(blocksDots) / float64(brailleDots)
	if ratio < 0.4 || ratio > 0.6 {
		t.Errorf("expected Blocks' dot-row offset to be ~half Braille's for the same geographic offset (YScale=0.5), got ratio %.3f (braille=%d blocks=%d)",
			ratio, brailleDots, blocksDots)
	}
}
