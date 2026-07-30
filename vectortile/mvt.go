// Package vectortile implements a pure-Go decoder for Mapbox Vector
// Tiles (MVT), the protobuf-encoded tile format used by OpenMapTiles,
// Mapbox and most other modern vector tile servers.
//
// See https://github.com/mapbox/vector-tile-spec for the wire format
// this package implements (schema version 2).
package vectortile

import (
	"fmt"
	"math"
)

// GeomType is a vector tile feature's geometry type.
type GeomType int

const (
	GeomUnknown    GeomType = 0
	GeomPoint      GeomType = 1
	GeomLineString GeomType = 2
	GeomPolygon    GeomType = 3
)

func (t GeomType) String() string {
	switch t {
	case GeomPoint:
		return "Point"
	case GeomLineString:
		return "LineString"
	case GeomPolygon:
		return "Polygon"
	default:
		return "Unknown"
	}
}

// Value is a single attribute value attached to a feature. Exactly one
// of the accessor methods below is meaningful for a given Value,
// according to which type the tile encoded.
type Value struct {
	kind valueKind
	s    string
	f    float64
	i    int64
	u    uint64
	b    bool
}

type valueKind int

const (
	kindNone valueKind = iota
	kindString
	kindFloat
	kindInt
	kindUint
	kindBool
)

// Interface returns the value using its natural Go type (string,
// float64, int64, uint64 or bool).
func (v Value) Interface() interface{} {
	switch v.kind {
	case kindString:
		return v.s
	case kindFloat:
		return v.f
	case kindInt:
		return v.i
	case kindUint:
		return v.u
	case kindBool:
		return v.b
	default:
		return nil
	}
}

// String renders the value as a human-readable string regardless of its
// underlying type.
func (v Value) String() string {
	switch v.kind {
	case kindString:
		return v.s
	case kindFloat:
		return fmt.Sprintf("%g", v.f)
	case kindInt:
		return fmt.Sprintf("%d", v.i)
	case kindUint:
		return fmt.Sprintf("%d", v.u)
	case kindBool:
		return fmt.Sprintf("%t", v.b)
	default:
		return ""
	}
}

// Point is a coordinate in a feature's local tile space: [0, extent) in
// both axes, with the origin at the tile's top-left corner.
type Point struct {
	X, Y int32
}

// Feature is a single geometry with its attributes.
type Feature struct {
	ID   uint64
	Type GeomType
	Tags map[string]Value

	// Geometry holds the feature's decoded parts: for Point features
	// each part is a single-point slice (MultiPoint); for LineString
	// features each part is one line; for Polygon features each part
	// is one ring (exterior or interior, undistinguished here).
	Geometry [][]Point
}

// Layer is a named collection of features sharing a coordinate extent.
type Layer struct {
	Name     string
	Version  uint32
	Extent   uint32
	Features []Feature
}

// Tile is a fully decoded vector tile.
type Tile struct {
	Layers []Layer
}

// LayerByName returns the named layer, or nil if the tile has none.
func (t *Tile) LayerByName(name string) *Layer {
	for i := range t.Layers {
		if t.Layers[i].Name == name {
			return &t.Layers[i]
		}
	}
	return nil
}

// Field numbers from the vector_tile.proto schema.
const (
	fieldTileLayers = 3

	fieldLayerName     = 1
	fieldLayerFeatures = 2
	fieldLayerKeys     = 3
	fieldLayerValues   = 4
	fieldLayerExtent   = 5
	fieldLayerVersion  = 15

	fieldFeatureID       = 1
	fieldFeatureTags     = 2
	fieldFeatureType     = 3
	fieldFeatureGeometry = 4

	fieldValueString = 1
	fieldValueFloat  = 2
	fieldValueDouble = 3
	fieldValueInt    = 4
	fieldValueUint   = 5
	fieldValueSint   = 6
	fieldValueBool   = 7
)

// Parse decodes a raw Mapbox Vector Tile (as returned by a tile server,
// typically gzip-decompressed already) into a Tile.
func Parse(data []byte) (*Tile, error) {
	r := newReader(data)
	tile := &Tile{}

	for !r.done() {
		field, wt, err := r.readTag()
		if err != nil {
			return nil, err
		}
		if field == fieldTileLayers && wt == wireBytes {
			buf, err := r.readBytes()
			if err != nil {
				return nil, err
			}
			layer, err := parseLayer(buf)
			if err != nil {
				return nil, fmt.Errorf("vectortile: layer: %w", err)
			}
			tile.Layers = append(tile.Layers, *layer)
			continue
		}
		if err := r.skip(wt); err != nil {
			return nil, err
		}
	}

	return tile, nil
}

func parseLayer(data []byte) (*Layer, error) {
	r := newReader(data)
	layer := &Layer{Extent: 4096, Version: 1}

	var keys []string
	var values []Value
	var rawFeatures []parsedFeature

	for !r.done() {
		field, wt, err := r.readTag()
		if err != nil {
			return nil, err
		}
		switch {
		case field == fieldLayerName && wt == wireBytes:
			s, err := r.readString()
			if err != nil {
				return nil, err
			}
			layer.Name = s

		case field == fieldLayerExtent && wt == wireVarint:
			v, err := r.readVarint()
			if err != nil {
				return nil, err
			}
			layer.Extent = uint32(v)

		case field == fieldLayerVersion && wt == wireVarint:
			v, err := r.readVarint()
			if err != nil {
				return nil, err
			}
			layer.Version = uint32(v)

		case field == fieldLayerKeys && wt == wireBytes:
			s, err := r.readString()
			if err != nil {
				return nil, err
			}
			keys = append(keys, s)

		case field == fieldLayerValues && wt == wireBytes:
			buf, err := r.readBytes()
			if err != nil {
				return nil, err
			}
			v, err := parseValue(buf)
			if err != nil {
				return nil, fmt.Errorf("value: %w", err)
			}
			values = append(values, v)

		case field == fieldLayerFeatures && wt == wireBytes:
			buf, err := r.readBytes()
			if err != nil {
				return nil, err
			}
			f, err := parseRawFeature(buf)
			if err != nil {
				return nil, fmt.Errorf("feature: %w", err)
			}
			rawFeatures = append(rawFeatures, *f)

		default:
			if err := r.skip(wt); err != nil {
				return nil, err
			}
		}
	}

	layer.Features = make([]Feature, 0, len(rawFeatures))
	for _, rf := range rawFeatures {
		feature := Feature{ID: rf.id, Type: rf.typ}
		if len(rf.tags) > 0 {
			feature.Tags = make(map[string]Value, len(rf.tags)/2)
			for i := 0; i+1 < len(rf.tags); i += 2 {
				ki, vi := int(rf.tags[i]), int(rf.tags[i+1])
				if ki < 0 || ki >= len(keys) || vi < 0 || vi >= len(values) {
					continue
				}
				feature.Tags[keys[ki]] = values[vi]
			}
		}
		feature.Geometry = decodeGeometry(rf.typ, rf.geometry)
		layer.Features = append(layer.Features, feature)
	}

	return layer, nil
}

type parsedFeature struct {
	id       uint64
	hasID    bool
	tags     []uint32
	typ      GeomType
	geometry []uint32
}

func parseRawFeature(data []byte) (*parsedFeature, error) {
	r := newReader(data)
	f := &parsedFeature{}

	for !r.done() {
		field, wt, err := r.readTag()
		if err != nil {
			return nil, err
		}
		switch {
		case field == fieldFeatureID && wt == wireVarint:
			v, err := r.readVarint()
			if err != nil {
				return nil, err
			}
			f.id, f.hasID = v, true

		case field == fieldFeatureType && wt == wireVarint:
			v, err := r.readVarint()
			if err != nil {
				return nil, err
			}
			f.typ = GeomType(v)

		case field == fieldFeatureTags:
			tags, err := readPackedOrSingle(r, wt)
			if err != nil {
				return nil, err
			}
			f.tags = append(f.tags, tags...)

		case field == fieldFeatureGeometry:
			geom, err := readPackedOrSingle(r, wt)
			if err != nil {
				return nil, err
			}
			f.geometry = append(f.geometry, geom...)

		default:
			if err := r.skip(wt); err != nil {
				return nil, err
			}
		}
	}

	return f, nil
}

// readPackedOrSingle reads a repeated uint32 field's next occurrence,
// accepting both the packed (length-delimited) encoding mandated by the
// vector tile spec and a defensive fallback for bare varint encoding.
func readPackedOrSingle(r *pbReader, wt wireType) ([]uint32, error) {
	if wt == wireBytes {
		return r.readPackedVarints()
	}
	if wt == wireVarint {
		v, err := r.readVarint()
		if err != nil {
			return nil, err
		}
		return []uint32{uint32(v)}, nil
	}
	return nil, r.skip(wt)
}

func parseValue(data []byte) (Value, error) {
	r := newReader(data)
	var v Value

	for !r.done() {
		field, wt, err := r.readTag()
		if err != nil {
			return v, err
		}
		switch {
		case field == fieldValueString && wt == wireBytes:
			s, err := r.readString()
			if err != nil {
				return v, err
			}
			v = Value{kind: kindString, s: s}

		case field == fieldValueFloat && wt == wireFixed32:
			bits, err := r.readFixed32()
			if err != nil {
				return v, err
			}
			v = Value{kind: kindFloat, f: float64(math.Float32frombits(bits))}

		case field == fieldValueDouble && wt == wireFixed64:
			bits, err := r.readFixed64()
			if err != nil {
				return v, err
			}
			v = Value{kind: kindFloat, f: math.Float64frombits(bits)}

		case field == fieldValueInt && wt == wireVarint:
			n, err := r.readVarint()
			if err != nil {
				return v, err
			}
			v = Value{kind: kindInt, i: int64(n)}

		case field == fieldValueUint && wt == wireVarint:
			n, err := r.readVarint()
			if err != nil {
				return v, err
			}
			v = Value{kind: kindUint, u: n}

		case field == fieldValueSint && wt == wireVarint:
			n, err := r.readVarint()
			if err != nil {
				return v, err
			}
			v = Value{kind: kindInt, i: int64(zigzagDecode64(n))}

		case field == fieldValueBool && wt == wireVarint:
			n, err := r.readVarint()
			if err != nil {
				return v, err
			}
			v = Value{kind: kindBool, b: n != 0}

		default:
			if err := r.skip(wt); err != nil {
				return v, err
			}
		}
	}

	return v, nil
}

func zigzagDecode64(v uint64) int64 {
	return int64(v>>1) ^ -int64(v&1)
}
