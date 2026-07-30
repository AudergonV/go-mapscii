package vectortile

// Geometry command IDs, per the vector tile spec.
const (
	cmdMoveTo    = 1
	cmdLineTo    = 2
	cmdClosePath = 7
)

// decodeGeometry interprets a feature's raw packed geometry commands
// into a set of parts (points, lines or rings) in tile-local integer
// coordinates. See:
// https://github.com/mapbox/vector-tile-spec/blob/master/2.1/README.md#43-geometry-encoding
func decodeGeometry(typ GeomType, geom []uint32) [][]Point {
	var parts [][]Point
	var cx, cy int32
	i := 0
	for i < len(geom) {
		cmdInt := geom[i]
		i++
		id := cmdInt & 0x7
		count := int(cmdInt >> 3)

		switch id {
		case cmdMoveTo:
			for c := 0; c < count; c++ {
				if i+1 >= len(geom) {
					break
				}
				dx := zigzagDecode(geom[i])
				dy := zigzagDecode(geom[i+1])
				i += 2
				cx += dx
				cy += dy
				parts = append(parts, []Point{{X: cx, Y: cy}})
			}

		case cmdLineTo:
			if len(parts) == 0 {
				parts = append(parts, []Point{{X: cx, Y: cy}})
			}
			cur := &parts[len(parts)-1]
			for c := 0; c < count; c++ {
				if i+1 >= len(geom) {
					break
				}
				dx := zigzagDecode(geom[i])
				dy := zigzagDecode(geom[i+1])
				i += 2
				cx += dx
				cy += dy
				*cur = append(*cur, Point{X: cx, Y: cy})
			}

		case cmdClosePath:
			if typ == GeomPolygon && len(parts) > 0 {
				cur := &parts[len(parts)-1]
				if len(*cur) > 0 {
					*cur = append(*cur, (*cur)[0])
				}
			}
			// ClosePath carries no parameters.

		default:
			// Unknown command: stop decoding rather than risk
			// misinterpreting the remaining stream.
			return parts
		}
	}
	return parts
}
