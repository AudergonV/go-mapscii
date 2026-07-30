// Package geo implements the Web Mercator projection math shared by the
// tile fetching, vector tile decoding and rendering packages.
package geo

import "math"

// TileSize is the size in pixels of a single map tile at its native
// resolution, as used by the Web Mercator / Slippy Map tiling scheme.
const TileSize = 256

// LatLon is a geographic coordinate expressed in degrees.
type LatLon struct {
	Lat float64
	Lon float64
}

// Point is a location in "world pixel space" at a given zoom level: the
// origin (0,0) is the north-west corner of the map, and the map is
// TileSize*2^zoom pixels wide and tall.
type Point struct {
	X float64
	Y float64
}

// clampLat keeps latitudes within the range representable by the Web
// Mercator projection (the projection diverges at the poles).
func clampLat(lat float64) float64 {
	const maxLat = 85.05112878
	if lat > maxLat {
		return maxLat
	}
	if lat < -maxLat {
		return -maxLat
	}
	return lat
}

// WorldSize returns the width/height in pixels of the whole world map at
// the given (fractional) zoom level.
func WorldSize(zoom float64) float64 {
	return TileSize * math.Pow(2, zoom)
}

// LatLonToPoint projects a geographic coordinate to world pixel space at
// the given zoom level.
func LatLonToPoint(ll LatLon, zoom float64) Point {
	lat := clampLat(ll.Lat)
	size := WorldSize(zoom)

	x := (ll.Lon + 180) / 360 * size

	latRad := lat * math.Pi / 180
	y := (0.5 - math.Log(math.Tan(math.Pi/4+latRad/2))/(2*math.Pi)) * size

	return Point{X: x, Y: y}
}

// PointToLatLon converts a world pixel space coordinate at the given zoom
// level back into a geographic coordinate.
func PointToLatLon(p Point, zoom float64) LatLon {
	size := WorldSize(zoom)

	lon := p.X/size*360 - 180

	n := math.Pi - 2*math.Pi*p.Y/size
	lat := 180 / math.Pi * math.Atan(0.5*(math.Exp(n)-math.Exp(-n)))

	return LatLon{Lat: lat, Lon: lon}
}

// TileIndex identifies a single Slippy Map / Web Mercator tile.
type TileIndex struct {
	Z int
	X int
	Y int
}

// TileCount returns the number of tiles along one edge of the world at
// the given integer zoom level (2^zoom).
func TileCount(zoom int) int {
	return 1 << uint(zoom)
}

// TileAt returns the tile containing the given geographic coordinate at
// the given integer zoom level.
func TileAt(ll LatLon, zoom int) TileIndex {
	p := LatLonToPoint(ll, float64(zoom))
	n := TileCount(zoom)
	x := int(p.X / TileSize)
	y := int(p.Y / TileSize)
	if x < 0 {
		x = 0
	}
	if x >= n {
		x = n - 1
	}
	if y < 0 {
		y = 0
	}
	if y >= n {
		y = n - 1
	}
	return TileIndex{Z: zoom, X: x, Y: y}
}

// TileOrigin returns the world pixel space coordinate of the north-west
// corner of the given tile.
func TileOrigin(t TileIndex) Point {
	return Point{
		X: float64(t.X) * TileSize,
		Y: float64(t.Y) * TileSize,
	}
}
