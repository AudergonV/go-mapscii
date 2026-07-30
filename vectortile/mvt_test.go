package vectortile

import (
	"bytes"
	"testing"
)

// --- minimal protobuf byte-builders, used only to construct test fixtures ---

func pbTag(field int, wt wireType) []byte {
	return pbVarint(uint64(field)<<3 | uint64(wt))
}

func pbVarint(v uint64) []byte {
	var buf []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			buf = append(buf, b|0x80)
		} else {
			buf = append(buf, b)
			break
		}
	}
	return buf
}

func pbBytesField(field int, data []byte) []byte {
	var buf bytes.Buffer
	buf.Write(pbTag(field, wireBytes))
	buf.Write(pbVarint(uint64(len(data))))
	buf.Write(data)
	return buf.Bytes()
}

func pbVarintField(field int, v uint64) []byte {
	var buf bytes.Buffer
	buf.Write(pbTag(field, wireVarint))
	buf.Write(pbVarint(v))
	return buf.Bytes()
}

func pbPackedVarints(field int, values []uint32) []byte {
	var packed bytes.Buffer
	for _, v := range values {
		packed.Write(pbVarint(uint64(v)))
	}
	return pbBytesField(field, packed.Bytes())
}

func zigzagEncode(v int32) uint32 {
	return uint32((v << 1) ^ (v >> 31))
}

// buildLineStringFeature builds a raw Feature message for a two-point
// line from (2,2) to (2,10) in tile-local coordinates, tagged with
// key index 0 -> value index 0.
func buildLineStringFeature() []byte {
	geomCmds := []uint32{
		(1 << 3) | cmdMoveTo, // MoveTo x1
		zigzagEncode(2), zigzagEncode(2),
		(1 << 3) | cmdLineTo, // LineTo x1
		zigzagEncode(0), zigzagEncode(8),
	}

	var buf bytes.Buffer
	buf.Write(pbVarintField(fieldFeatureID, 42))
	buf.Write(pbPackedVarints(fieldFeatureTags, []uint32{0, 0}))
	buf.Write(pbVarintField(fieldFeatureType, uint64(GeomLineString)))
	buf.Write(pbPackedVarints(fieldFeatureGeometry, geomCmds))
	return buf.Bytes()
}

func buildStringValue(s string) []byte {
	var buf bytes.Buffer
	buf.Write(pbTag(fieldValueString, wireBytes))
	buf.Write(pbVarint(uint64(len(s))))
	buf.WriteString(s)
	return buf.Bytes()
}

func buildLayer() []byte {
	var buf bytes.Buffer
	buf.Write(pbBytesField(fieldLayerName, []byte("roads")))
	buf.Write(pbVarintField(fieldLayerVersion, 2))
	buf.Write(pbBytesField(fieldLayerKeys, []byte("highway")))
	buf.Write(pbBytesField(fieldLayerValues, buildStringValue("primary")))
	buf.Write(pbBytesField(fieldLayerFeatures, buildLineStringFeature()))
	buf.Write(pbVarintField(fieldLayerExtent, 4096))
	return buf.Bytes()
}

func buildTile() []byte {
	return pbBytesField(fieldTileLayers, buildLayer())
}

func TestParseHandCraftedTile(t *testing.T) {
	tile, err := Parse(buildTile())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(tile.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(tile.Layers))
	}
	layer := tile.Layers[0]
	if layer.Name != "roads" {
		t.Errorf("layer.Name = %q, want roads", layer.Name)
	}
	if layer.Version != 2 {
		t.Errorf("layer.Version = %d, want 2", layer.Version)
	}
	if layer.Extent != 4096 {
		t.Errorf("layer.Extent = %d, want 4096", layer.Extent)
	}
	if len(layer.Features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(layer.Features))
	}

	f := layer.Features[0]
	if f.ID != 42 {
		t.Errorf("feature.ID = %d, want 42", f.ID)
	}
	if f.Type != GeomLineString {
		t.Errorf("feature.Type = %v, want LineString", f.Type)
	}
	if got := f.Tags["highway"].String(); got != "primary" {
		t.Errorf(`feature.Tags["highway"] = %q, want "primary"`, got)
	}

	if len(f.Geometry) != 1 {
		t.Fatalf("expected 1 geometry part, got %d", len(f.Geometry))
	}
	line := f.Geometry[0]
	want := []Point{{X: 2, Y: 2}, {X: 2, Y: 10}}
	if len(line) != len(want) {
		t.Fatalf("line = %v, want %v", line, want)
	}
	for i := range want {
		if line[i] != want[i] {
			t.Errorf("line[%d] = %+v, want %+v", i, line[i], want[i])
		}
	}
}

func TestDecodePolygonClosesRing(t *testing.T) {
	geom := []uint32{
		(1 << 3) | cmdMoveTo,
		zigzagEncode(0), zigzagEncode(0),
		(3 << 3) | cmdLineTo,
		zigzagEncode(10), zigzagEncode(0),
		zigzagEncode(0), zigzagEncode(10),
		zigzagEncode(-10), zigzagEncode(-10),
		(1 << 3) | cmdClosePath,
	}
	parts := decodeGeometry(GeomPolygon, geom)
	if len(parts) != 1 {
		t.Fatalf("expected 1 ring, got %d", len(parts))
	}
	ring := parts[0]
	if len(ring) != 5 {
		t.Fatalf("expected 5 points (4 + closing point), got %d: %v", len(ring), ring)
	}
	if ring[0] != ring[len(ring)-1] {
		t.Errorf("ring not closed: first=%+v last=%+v", ring[0], ring[len(ring)-1])
	}
}

func TestDecodeMultiPoint(t *testing.T) {
	geom := []uint32{
		(2 << 3) | cmdMoveTo,
		zigzagEncode(5), zigzagEncode(5),
		zigzagEncode(3), zigzagEncode(-2),
	}
	parts := decodeGeometry(GeomPoint, geom)
	if len(parts) != 2 {
		t.Fatalf("expected 2 points, got %d", len(parts))
	}
	if parts[0][0] != (Point{X: 5, Y: 5}) {
		t.Errorf("parts[0] = %+v, want {5 5}", parts[0][0])
	}
	if parts[1][0] != (Point{X: 8, Y: 3}) {
		t.Errorf("parts[1] = %+v, want {8 3}", parts[1][0])
	}
}

func TestParseEmptyTile(t *testing.T) {
	tile, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse(nil): %v", err)
	}
	if len(tile.Layers) != 0 {
		t.Errorf("expected no layers, got %d", len(tile.Layers))
	}
}

func TestParseTruncatedTileErrors(t *testing.T) {
	full := buildTile()
	if len(full) < 3 {
		t.Fatal("fixture too short")
	}
	_, err := Parse(full[:len(full)-3])
	if err == nil {
		t.Fatal("expected error decoding truncated tile, got nil")
	}
}
