# go-mapscii

A pure Go library for rendering interactive vector maps in the terminal,
inspired by [mapscii](https://github.com/rastapasta/mapscii). It draws
[Mapbox Vector Tiles](https://github.com/mapbox/vector-tile-spec) with
24-bit color using a pluggable rendering technique - Unicode braille
(default, highest resolution), Unicode block-mosaic quadrants, or plain
ASCII - and exposes a small API to pan, zoom, add labeled pins and draw
lines over the map.

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
fetching/decoding, projection, rendering, styling and overlays.
Label placement for POIs/place names and antialiased polygon edges are
not implemented yet.

## Packages

| Package | Purpose |
|---|---|
| `github.com/audergonv/go-mapscii` (root) | Top-level `Map` API: viewport, pins, lines, `Render()`. |
| `geo` | Web Mercator projection: lat/lon ⟷ world pixel ⟷ tile math. |
| `canvas` | Generic terminal drawing surface driven by a `Shape` (dot resolution + glyph set); ships `Braille`, `Blocks` and `ASCII` shapes, with per-cell truecolor. |
| `vectortile` | Pure Go decoder for Mapbox Vector Tiles (protobuf), no codegen or protobuf runtime dependency. |
| `tileprovider` | Fetches and caches vector tiles over HTTP; also a `MemoryProvider` for tests/offline use. |
| `style` | Maps vector tile layers (water, landuse, road, building, ...) to colors and line widths. |

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
	"github.com/audergonv/go-mapscii/canvas"
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
		mapscii.WithLineColor(canvas.Color{R: 255, G: 80, B: 80}),
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
the configured Shape's characters and 24-bit ANSI color escapes - print
it directly, or diff it against the previous frame for smoother
redraws in an interactive app.

### Render shapes: braille, blocks, ASCII, or your own

`Options.Shape` (or `Map.SetShape` at runtime) picks the rendering
technique. Three are built in:

| Shape | Resolution per cell | Character set | Notes |
|---|---|---|---|
| `canvas.Braille` (default) | 2x4 dots | Unicode Braille Patterns | Highest resolution; needs a font with braille glyphs (most modern terminal fonts have one). |
| `canvas.Blocks` | 2x2 dots | Unicode block-mosaic quadrants (`▘▝▀▖▌▞▛▗▚▐▜▄▙▟█`) | Half the resolution of braille, bolder look, uses a smaller/older Unicode block that's close to universally supported. |
| `canvas.ASCII` | 1x1 dot | Plain ASCII (`#` by default) | No Unicode required at all; use `canvas.NewASCIIShape('*')` etc. to pick a different character. |

```go
m, err := mapscii.New(mapscii.Options{
    Provider: provider,
    Shape:    canvas.Blocks, // or canvas.ASCII, canvas.NewASCIIShape('@'), ...
})

// Switch at runtime, e.g. bound to a keypress:
m.SetShape(canvas.ASCII)
```

A Shape is just a small struct (dot grid size, a bit-mask function, a
glyph-lookup function, a pin marker rune, and an optional `YScale`), so
defining your own - sextants, a custom glyph ramp, whatever - only
takes implementing `canvas.Shape`; see `canvas/shapes.go` for the
reference implementations.

Terminal cells are roughly twice as tall as wide, and Braille's 2x4 dot
grid already accounts for that (its dots come out square on screen).
Blocks and ASCII use square dot grids (2x2 and 1x1), which would
stretch the map vertically if left uncorrected - both set `YScale: 0.5`
to compress the vertical world-to-dot mapping and keep geography
proportioned correctly. Set `YScale` on a custom Shape too if its
`DotsY/DotsX` ratio isn't 2.

### Customizing colors and line widths

```go
import "github.com/audergonv/go-mapscii/style"

sty := style.Default()
sty.Layers["water"] = style.Rule{Color: canvas.Color{R: 20, G: 60, B: 120}, Priority: 10}
sty.PinColor = canvas.Color{R: 255, G: 200, B: 0}

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
arrow keys pan, `+`/`-` zoom, `s` cycles the render shape
(braille → blocks → ASCII), `r` resets, `q` quits.

```sh
go run ./cmd/mapscii-demo -tile-url "https://your-tile-server/tiles/{z}/{x}/{y}.pbf" \
    -lat 48.8566 -lon 2.3522 -zoom 12 \
    -pin "48.8584,2.2945,Eiffel Tower" \
    -shape blocks # or braille (default), ascii
```

## Testing

```sh
go test ./...
```

The `vectortile` decoder is tested against hand-built protobuf fixtures
(no live tile server required), and the top-level `Map`/`Render`
pipeline is tested against `tileprovider.MemoryProvider` with synthetic
tiles.
