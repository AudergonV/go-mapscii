package mapscii

import "github.com/audergonv/go-mapscii/braille"

// Pin marks a labeled point on the map.
type Pin struct {
	id       int
	Pos      LatLon
	Label    string
	Color    braille.Color
	hasColor bool
}

// Line is a path drawn over the map, connecting its Points in order.
type Line struct {
	id       int
	Points   []LatLon
	Color    braille.Color
	hasColor bool
	Width    float64
	hasWidth bool
}

// LineOption configures a Line created by DrawLine.
type LineOption func(*Line)

// WithLineColor sets a custom color for a drawn line, overriding the
// style's DefaultLineColor.
func WithLineColor(c braille.Color) LineOption {
	return func(l *Line) {
		l.Color = c
		l.hasColor = true
	}
}

// WithLineWidth sets a custom thickness, in canvas dots, for a drawn
// line, overriding the style's DefaultLineWidth. A width of 1 (the
// default) draws a plain 1-dot line; larger values approximate a
// thicker stroke by stacking parallel offset lines.
func WithLineWidth(width float64) LineOption {
	return func(l *Line) {
		l.Width = width
		l.hasWidth = true
	}
}

// PinOption configures a Pin created by AddPin.
type PinOption func(*Pin)

// WithPinColor sets a custom color for a pin, overriding the style's
// PinColor.
func WithPinColor(c braille.Color) PinOption {
	return func(p *Pin) {
		p.Color = c
		p.hasColor = true
	}
}

// AddPin adds a labeled pin at the given coordinate and returns it. The
// returned *Pin can later be passed to RemovePin.
func (m *Map) AddPin(lat, lon float64, label string, opts ...PinOption) *Pin {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	p := &Pin{id: m.nextID, Pos: LatLon{Lat: lat, Lon: lon}, Label: label}
	for _, opt := range opts {
		opt(p)
	}
	m.pins = append(m.pins, p)
	return p
}

// RemovePin removes a previously added pin. It is a no-op if the pin
// has already been removed or belongs to a different Map.
func (m *Map) RemovePin(p *Pin) {
	if p == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.pins {
		if existing == p {
			m.pins = append(m.pins[:i], m.pins[i+1:]...)
			return
		}
	}
}

// ClearPins removes every pin from the map.
func (m *Map) ClearPins() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pins = nil
}

// Pins returns a snapshot of the currently added pins.
func (m *Map) Pins() []*Pin {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Pin, len(m.pins))
	copy(out, m.pins)
	return out
}

// DrawLine adds a line connecting the given points, in order, and
// returns it. At least two points are required for the line to be
// visible. The returned *Line can later be passed to RemoveLine.
func (m *Map) DrawLine(points []LatLon, opts ...LineOption) *Line {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextID++
	pts := make([]LatLon, len(points))
	copy(pts, points)
	l := &Line{id: m.nextID, Points: pts}
	for _, opt := range opts {
		opt(l)
	}
	m.lines = append(m.lines, l)
	return l
}

// RemoveLine removes a previously drawn line. It is a no-op if the line
// has already been removed or belongs to a different Map.
func (m *Map) RemoveLine(l *Line) {
	if l == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, existing := range m.lines {
		if existing == l {
			m.lines = append(m.lines[:i], m.lines[i+1:]...)
			return
		}
	}
}

// ClearLines removes every drawn line from the map.
func (m *Map) ClearLines() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines = nil
}

// Lines returns a snapshot of the currently drawn lines.
func (m *Map) Lines() []*Line {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]*Line, len(m.lines))
	copy(out, m.lines)
	return out
}

// ClearOverlays removes every pin and drawn line from the map.
func (m *Map) ClearOverlays() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pins = nil
	m.lines = nil
}
