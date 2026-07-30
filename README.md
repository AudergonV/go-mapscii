# go-mapscii

A pure Go library for rendering interactive vector maps in the terminal,
inspired by [mapscii](https://github.com/rastapasta/mapscii). It draws
[Mapbox Vector Tiles](https://github.com/mapbox/vector-tile-spec) as
Unicode braille art with 24-bit color, and exposes a small API to pan,
zoom, add labeled pins and draw lines over the map.

No cgo, no Node.js, no external rendering dependencies - just Go.

```
⣿⣿⣿⣿⣿⣿⣿⣿⣿⡇                                                  ⢀⠔⠁
⣿⣿⣿⣿⣿⣿⣿⣿⣿⡇                                                ⢀⠔⠁
⣿⣿⣿⣿⣿⣿⣿⣿⣿⡇                              ⡠⠒⢁⠔⠁
⣿⣿⣿⣿⣿⣿⣿⣿⣿⡇                           ⣀⠔●  Paris
⣿⣿⣿⣿⣿⣿⣿⣿⣿⡇                        ⢀⠤⠊ ⢀⠔⠁
```

## Status

This is a young library covering the core rendering pipeline: tile
fetching/decoding, projection, braille rendering, styling and overlays.
Label placement for POIs/place names and antialiased polygon edges are
not implemented yet.

## Packages

| Package | Purpose |
|---|---|
| `github.com/audergonv/go-mapscii` (root) | Top-level `Map` API: viewport, pins, lines, `Render()`. |
| `geo` | Web Mercator projection: lat/lon ⟷ world pixel ⟷ tile math. |
| `braille` | Terminal drawing surface using Unicode braille patterns (2x4 sub-pixel resolution per cell), with per-cell truecolor. |
| `vectortile` | Pure Go decoder for Mapbox Vector Tiles (protobuf), no codegen or protobuf runtime dependency. |
| `tileprovider` | Fetches and caches vector tiles over HTTP; also a `MemoryProvider` for tests/offline use. |
| `style` | Maps vector tile layers (water, landuse, road, building, ...) to colors. |

## Tile source

go-mapscii does not ship with a default tile server - point it at any
vector tile source you're authorized to use: a self-hosted
[tileserver-gl](https://github.com/maptiler/tileserver-gl) /
[OpenMapTiles](https://openmaptiles.org/) instance, or a commercial
provider (MapTiler, Mapbox, etc.), usually with an API key.

```go
provider := tileprovider.NewHTTPProvider(
    "https://your-tile-server/tiles/{z}/{x}/{y}.pbf",
    tileprovider.WithHeader("Authorization", "Bearer "+apiKey), // if needed
)
```

The decoder understands both the OpenMapTiles schema (`transportation`,
`landcover`, ...) and the older Mapbox Streets schema (`road`,
`landuse`, ...) out of the box.

## Usage

```go
package main

import (
	"context"
	"fmt"

	mapscii "github.com/audergonv/go-mapscii"
	"github.com/audergonv/go-mapscii/braille"
	"github.com/audergonv/go-mapscii/tileprovider"
)

func main() {
	provider := tileprovider.NewHTTPProvider("https://your-tile-server/tiles/{z}/{x}/{y}.pbf")

	m, err := mapscii.New(mapscii.Options{
		Provider: provider,
		Width:    100, // terminal columns
		Height:   40,  // terminal rows
		Center:   mapscii.LatLon{Lat: 48.8566, Lon: 2.3522}, // Paris
		Zoom:     12,
	})
	if err != nil {
		panic(err)
	}

	// Move around.
	m.SetCenter(48.8606, 2.3376)
	m.SetZoom(14)
	m.Pan(10, -5) // pan by terminal cells

	// Mark a point.
	m.AddPin(48.8584, 2.2945, "Eiffel Tower")

	// Draw a path over the map.
	m.DrawLine([]mapscii.LatLon{
		{Lat: 48.8566, Lon: 2.3522},
		{Lat: 48.8584, Lon: 2.2945},
	},
		mapscii.WithLineColor(braille.Color{R: 255, G: 80, B: 80}),
		mapscii.WithLineWidth(3), // thickness in canvas dots, default 1
	)

	frame, err := m.Render(context.Background())
	if err != nil {
		panic(err)
	}
	fmt.Println(frame)
}
```

`Render` returns a string of terminal rows joined by `\n`, containing
braille characters and 24-bit ANSI color escapes - print it directly, or
diff it against the previous frame for smoother redraws in an
interactive app.

### Customizing colors and line widths

```go
import "github.com/audergonv/go-mapscii/style"

sty := style.Default()
sty.Layers["water"] = style.Rule{Color: braille.Color{R: 20, G: 60, B: 120}, Priority: 10}
sty.PinColor = braille.Color{R: 255, G: 200, B: 0}

// Line thickness (in canvas dots) is just as configurable as color.
sty.Layers["road"] = style.Rule{Color: sty.Layers["road"].Color, Priority: 40, Width: 2}
sty.RoadWidths["motorway"] = 4    // refines "road"/"transportation" width by class/highway tag
sty.DefaultLineWidth = 2          // default thickness for Map.DrawLine overlays

m, err := mapscii.New(mapscii.Options{Provider: provider, Style: sty})
```

Overlay lines drawn with `Map.DrawLine` can also set their own width
per line via `mapscii.WithLineWidth(dots)`, overriding
`Style.DefaultLineWidth` (see the earlier example). Widths are
approximated by stacking parallel 1-dot lines - there's no
anti-aliasing, but it reads well at terminal resolutions.

## Interactive demo

`cmd/mapscii-demo` is a small terminal app built on the library:
arrow keys pan, `+`/`-` zoom, `r` resets, `q` quits.

```sh
go run ./cmd/mapscii-demo -tile-url "https://your-tile-server/tiles/{z}/{x}/{y}.pbf" \
    -lat 48.8566 -lon 2.3522 -zoom 12 \
    -pin "48.8584,2.2945,Eiffel Tower"
```

## Testing

```sh
go test ./...
```

The `vectortile` decoder is tested against hand-built protobuf fixtures
(no live tile server required), and the top-level `Map`/`Render`
pipeline is tested against `tileprovider.MemoryProvider` with synthetic
tiles.
