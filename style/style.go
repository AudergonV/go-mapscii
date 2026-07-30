// Package style maps vector tile layers and feature classes to the
// colors used when rendering a map, similar in spirit to mapscii's own
// style.json. It understands both the OpenMapTiles schema (layers named
// "transportation", "landcover", ...) and the older Mapbox Streets
// schema (layers named "road", "landuse", ...), since either may be
// served by a configured tile source.
package style

import "github.com/audergonv/go-mapscii/braille"

// Rule describes how features from a given vector tile layer should be
// drawn.
type Rule struct {
	// Color is the base color used for features in this layer.
	Color braille.Color
	// Priority controls draw order: layers with a higher priority are
	// drawn after (on top of) layers with a lower one.
	Priority int
	// Width is the line thickness, in canvas dots, used for
	// LineString features in this layer. It is ignored for polygon
	// and point features. A width of 0 or 1 draws a plain 1-dot line.
	Width float64
}

// Style holds the full set of rules used to render a map's base layers,
// plus the colors used for overlays (pins, drawn lines) and empty space.
type Style struct {
	// Layers maps a vector tile layer name to its render rule.
	Layers map[string]Rule
	// RoadClasses refines the color of "road"/"transportation" features
	// based on their "class" or "highway" tag (e.g. "motorway",
	// "footway"). Classes not listed here fall back to the layer's
	// base Rule.Color.
	RoadClasses map[string]braille.Color
	// RoadWidths refines the line thickness (in canvas dots) of
	// "road"/"transportation" features the same way RoadClasses
	// refines their color. Classes not listed here fall back to the
	// layer's base Rule.Width.
	RoadWidths map[string]float64

	// Background is used for cells with no matching feature.
	Background braille.Color
	// PinColor and PinLabelColor style overlay pins added via the
	// top-level Map API.
	PinColor      braille.Color
	PinLabelColor braille.Color
	// DefaultLineColor and DefaultLineWidth style overlay lines drawn
	// via Map.DrawLine that don't specify their own color/width.
	DefaultLineColor braille.Color
	DefaultLineWidth float64
}

// Default returns a reasonable built-in style covering the common
// OpenMapTiles and Mapbox Streets layer names. Callers can freely
// mutate the returned Style's maps to customize or extend it.
func Default() *Style {
	return &Style{
		Layers: map[string]Rule{
			// Water.
			"water":    {Color: braille.Color{R: 90, G: 140, B: 235}, Priority: 10},
			"ocean":    {Color: braille.Color{R: 80, G: 130, B: 225}, Priority: 10},
			"waterway": {Color: braille.Color{R: 90, G: 140, B: 235}, Priority: 11, Width: 1},

			// Land cover / land use.
			"landcover": {Color: braille.Color{R: 195, G: 214, B: 178}, Priority: 5},
			"landuse":   {Color: braille.Color{R: 214, G: 212, B: 184}, Priority: 6},
			"park":      {Color: braille.Color{R: 163, G: 209, B: 154}, Priority: 7},
			"wood":      {Color: braille.Color{R: 142, G: 193, B: 132}, Priority: 7},
			"forest":    {Color: braille.Color{R: 142, G: 193, B: 132}, Priority: 7},

			// Infrastructure.
			"aeroway":        {Color: braille.Color{R: 200, G: 160, B: 200}, Priority: 20},
			"building":       {Color: braille.Color{R: 196, G: 176, B: 145}, Priority: 30},
			"road":           {Color: braille.Color{R: 235, G: 235, B: 235}, Priority: 40, Width: 1},
			"transportation": {Color: braille.Color{R: 235, G: 235, B: 235}, Priority: 40, Width: 1},

			// Boundaries, drawn last so they stay visible.
			"boundary": {Color: braille.Color{R: 224, G: 130, B: 130}, Priority: 50, Width: 1},
			"admin":    {Color: braille.Color{R: 224, G: 130, B: 130}, Priority: 50, Width: 1},
		},
		RoadClasses: map[string]braille.Color{
			"motorway":   {R: 255, G: 170, B: 60},
			"trunk":      {R: 255, G: 190, B: 90},
			"primary":    {R: 250, G: 210, B: 120},
			"secondary":  {R: 235, G: 225, B: 170},
			"tertiary":   {R: 220, G: 220, B: 200},
			"minor":      {R: 200, G: 200, B: 190},
			"service":    {R: 150, G: 150, B: 150},
			"path":       {R: 150, G: 150, B: 150},
			"footway":    {R: 150, G: 150, B: 150},
			"pedestrian": {R: 150, G: 150, B: 150},
			"track":      {R: 150, G: 150, B: 150},
			"rail":       {R: 130, G: 130, B: 140},
		},
		RoadWidths: map[string]float64{
			"motorway":   3,
			"trunk":      2.5,
			"primary":    2,
			"secondary":  1.5,
			"tertiary":   1,
			"minor":      1,
			"service":    1,
			"path":       1,
			"footway":    1,
			"pedestrian": 1,
			"track":      1,
			"rail":       1,
		},
		Background:       braille.Color{R: 30, G: 33, B: 41},
		PinColor:         braille.Color{R: 235, G: 70, B: 70},
		PinLabelColor:    braille.Color{R: 240, G: 240, B: 240},
		DefaultLineColor: braille.Color{R: 250, G: 210, B: 60},
		DefaultLineWidth: 1,
	}
}

// Rule looks up the render rule for a vector tile layer name, reporting
// whether one was found.
func (s *Style) Rule(layerName string) (Rule, bool) {
	r, ok := s.Layers[layerName]
	return r, ok
}

// RoadColor resolves the color for a road/transportation feature based
// on its class tag, falling back to fallback if the class is unknown.
func (s *Style) RoadColor(class string, fallback braille.Color) braille.Color {
	if c, ok := s.RoadClasses[class]; ok {
		return c
	}
	return fallback
}

// RoadWidth resolves the line thickness for a road/transportation
// feature based on its class tag, falling back to fallback if the
// class is unknown.
func (s *Style) RoadWidth(class string, fallback float64) float64 {
	if w, ok := s.RoadWidths[class]; ok {
		return w
	}
	return fallback
}
