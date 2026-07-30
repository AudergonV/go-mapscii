// Package mapscii renders vector-tile maps in the terminal, using a
// configurable character set (Unicode braille by default) for
// sub-cell resolution. It is a pure Go library inspired by
// github.com/rastapasta/mapscii: point it at a Mapbox Vector Tile
// source, then pan, zoom, add pins and draw lines over a live-rendered
// map.
package mapscii

import (
	"errors"
	"fmt"
	"sync"

	"github.com/audergonv/go-mapscii/canvas"
	"github.com/audergonv/go-mapscii/geo"
	"github.com/audergonv/go-mapscii/style"
	"github.com/audergonv/go-mapscii/tileprovider"
)

// LatLon is a geographic coordinate expressed in degrees.
type LatLon = geo.LatLon

// Default zoom bounds, matching the range most public vector tile
// sources actually generate data for.
const (
	DefaultMinZoom     = 0.0
	DefaultMaxZoom     = 18.0
	DefaultMaxTileZoom = 14
	DefaultZoom        = 2.0
)

// Options configures a new Map.
type Options struct {
	// Provider fetches and decodes vector tiles. Required.
	Provider tileprovider.Provider

	// Width and Height set the render size in terminal character
	// cells. Both default to 80x24 if left zero.
	Width, Height int

	// Center and Zoom set the initial viewport. Center defaults to
	// (0,0); Zoom defaults to DefaultZoom.
	Center LatLon
	Zoom   float64

	// MinZoom and MaxZoom bound SetZoom. Both default if left zero
	// (MaxZoom defaults to DefaultMaxZoom).
	MinZoom, MaxZoom float64

	// MaxTileZoom is the highest integer zoom level at which tiles are
	// requested from the Provider; zooming in further than this
	// magnifies the same tile data instead of requesting new tiles.
	// Defaults to DefaultMaxTileZoom, matching common public tile
	// sources which stop generating data around z14.
	MaxTileZoom int

	// Style controls the color used for each vector tile layer.
	// Defaults to style.Default().
	Style *style.Style

	// Shape controls the rendering technique: how many sub-pixel dots
	// fit in one terminal cell and which characters represent them.
	// Defaults to canvas.Braille (highest resolution). canvas.Blocks
	// trades resolution for bolder, more widely supported block
	// characters, and canvas.ASCII (or canvas.NewASCIIShape) uses
	// plain ASCII only. Any custom canvas.Shape works too.
	Shape canvas.Shape
}

// Map is a renderable, pannable, zoomable vector-tile map with support
// for overlay pins and lines. It is safe for concurrent use.
type Map struct {
	mu sync.RWMutex

	provider    tileprovider.Provider
	style       *style.Style
	shape       canvas.Shape
	width       int
	height      int
	center      LatLon
	zoom        float64
	minZoom     float64
	maxZoom     float64
	maxTileZoom int

	pins   []*Pin
	lines  []*Line
	nextID int
}

// New creates a Map from the given Options. Provider must be non-nil.
func New(opts Options) (*Map, error) {
	if opts.Provider == nil {
		return nil, errors.New("mapscii: Options.Provider is required")
	}

	m := &Map{
		provider:    opts.Provider,
		style:       opts.Style,
		shape:       opts.Shape,
		width:       opts.Width,
		height:      opts.Height,
		center:      opts.Center,
		zoom:        opts.Zoom,
		minZoom:     opts.MinZoom,
		maxZoom:     opts.MaxZoom,
		maxTileZoom: opts.MaxTileZoom,
	}

	if m.style == nil {
		m.style = style.Default()
	}
	if m.shape.DotsX == 0 || m.shape.DotsY == 0 {
		m.shape = canvas.Braille
	}
	if m.width <= 0 {
		m.width = 80
	}
	if m.height <= 0 {
		m.height = 24
	}
	if m.zoom == 0 {
		m.zoom = DefaultZoom
	}
	if m.maxZoom == 0 {
		m.maxZoom = DefaultMaxZoom
	}
	if m.maxTileZoom == 0 {
		m.maxTileZoom = DefaultMaxTileZoom
	}
	m.zoom = clamp(m.zoom, m.minZoom, m.maxZoom)

	return m, nil
}

// SetCenter moves the viewport to be centered on the given coordinate.
func (m *Map) SetCenter(lat, lon float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.center = LatLon{Lat: lat, Lon: lon}
}

// Center returns the current viewport center.
func (m *Map) Center() LatLon {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.center
}

// Pan shifts the viewport by the given offsets, expressed in terminal
// character cells (positive dx moves the view right, positive dy moves
// it down).
func (m *Map) Pan(dxCells, dyCells float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if dxCells == 0 && dyCells == 0 {
		return
	}
	center := geo.LatLonToPoint(m.center, m.zoom)
	center.X += dxCells * float64(m.shape.DotsX)
	center.Y += dyCells * float64(m.shape.DotsY)
	m.center = geo.PointToLatLon(center, m.zoom)
}

// SetZoom sets the viewport zoom level, clamped to [MinZoom, MaxZoom].
func (m *Map) SetZoom(zoom float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.zoom = clamp(zoom, m.minZoom, m.maxZoom)
}

// ZoomBy adjusts the zoom level by a relative amount.
func (m *Map) ZoomBy(delta float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.zoom = clamp(m.zoom+delta, m.minZoom, m.maxZoom)
}

// Zoom returns the current zoom level.
func (m *Map) Zoom() float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.zoom
}

// Resize changes the render size in terminal character cells.
func (m *Map) Resize(width, height int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if width > 0 {
		m.width = width
	}
	if height > 0 {
		m.height = height
	}
}

// Size returns the current render size in terminal character cells.
func (m *Map) Size() (width, height int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.width, m.height
}

// Shape returns the render Shape currently in use.
func (m *Map) Shape() canvas.Shape {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.shape
}

// SetShape switches the rendering technique (see Options.Shape). It
// takes effect on the next call to Render.
func (m *Map) SetShape(shape canvas.Shape) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if shape.DotsX == 0 || shape.DotsY == 0 {
		shape = canvas.Braille
	}
	m.shape = shape
}

func clamp(v, lo, hi float64) float64 {
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

func (m *Map) String() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return fmt.Sprintf("Map(center=%.5f,%.5f zoom=%.2f size=%dx%d)",
		m.center.Lat, m.center.Lon, m.zoom, m.width, m.height)
}
