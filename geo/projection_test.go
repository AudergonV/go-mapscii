package geo

import (
	"math"
	"testing"
)

func almostEqual(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}

func TestLatLonPointRoundTrip(t *testing.T) {
	cases := []LatLon{
		{Lat: 0, Lon: 0},
		{Lat: 48.8566, Lon: 2.3522},    // Paris
		{Lat: -33.8688, Lon: 151.2093}, // Sydney
		{Lat: 60.1699, Lon: 24.9384},   // Helsinki
	}
	for _, ll := range cases {
		for _, zoom := range []float64{0, 4, 10, 16} {
			p := LatLonToPoint(ll, zoom)
			got := PointToLatLon(p, zoom)
			if !almostEqual(got.Lat, ll.Lat, 1e-6) || !almostEqual(got.Lon, ll.Lon, 1e-6) {
				t.Errorf("zoom=%v: round trip mismatch: in=%+v out=%+v", zoom, ll, got)
			}
		}
	}
}

func TestWorldSize(t *testing.T) {
	if got := WorldSize(0); got != TileSize {
		t.Errorf("WorldSize(0) = %v, want %v", got, TileSize)
	}
	if got := WorldSize(1); got != TileSize*2 {
		t.Errorf("WorldSize(1) = %v, want %v", got, TileSize*2)
	}
}

func TestTileAt(t *testing.T) {
	// Null island falls in the tile at the very center of the world.
	tile := TileAt(LatLon{Lat: 0, Lon: 0}, 2)
	n := TileCount(2)
	if tile.X != n/2 || tile.Y != n/2 {
		t.Errorf("TileAt(null island, z=2) = %+v, want center tile (%d,%d)", tile, n/2, n/2)
	}
}

func TestTileAtClampsToValidRange(t *testing.T) {
	tile := TileAt(LatLon{Lat: 89, Lon: 200}, 3)
	n := TileCount(3)
	if tile.X < 0 || tile.X >= n || tile.Y < 0 || tile.Y >= n {
		t.Errorf("TileAt out of range: %+v (n=%d)", tile, n)
	}
}

func TestTileOrigin(t *testing.T) {
	origin := TileOrigin(TileIndex{Z: 3, X: 2, Y: 5})
	want := Point{X: 2 * TileSize, Y: 5 * TileSize}
	if origin != want {
		t.Errorf("TileOrigin = %+v, want %+v", origin, want)
	}
}
