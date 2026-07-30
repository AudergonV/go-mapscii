// Command mapscii-demo is an interactive terminal viewer built on top
// of the go-mapscii library: it renders a live, pannable, zoomable
// vector-tile map, with optional pins and a drawn line, controlled with
// the keyboard.
//
// go-mapscii ships with no default tile server; point mapscii-demo at
// one you are authorized to use:
//
//	mapscii-demo -tile-url "https://example.com/tiles/{z}/{x}/{y}.pbf"
//
// Controls: arrow keys pan, +/- zoom, s cycles the render shape
// (braille/blocks/ascii), r re-centers on the start location, q or
// Ctrl-C quits.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	mapscii "github.com/audergonv/go-mapscii"
	"github.com/audergonv/go-mapscii/canvas"
	"github.com/audergonv/go-mapscii/tileprovider"
	"golang.org/x/term"
)

// shapeCycle lists the render shapes -s cycles through, in order.
var shapeCycle = []canvas.Shape{canvas.Braille, canvas.Blocks, canvas.ASCII}

func parseShape(name string) (canvas.Shape, error) {
	switch strings.ToLower(name) {
	case "braille":
		return canvas.Braille, nil
	case "blocks", "block":
		return canvas.Blocks, nil
	case "ascii":
		return canvas.ASCII, nil
	default:
		return canvas.Shape{}, fmt.Errorf("unknown shape %q (want braille, blocks or ascii)", name)
	}
}

type headerFlags map[string]string

func (h headerFlags) String() string { return "" }
func (h headerFlags) Set(v string) error {
	parts := strings.SplitN(v, ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("expected Key:Value, got %q", v)
	}
	h[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	return nil
}

type pinFlag struct {
	Lat, Lon float64
	Label    string
}

type pinFlags []pinFlag

func (p *pinFlags) String() string { return "" }
func (p *pinFlags) Set(v string) error {
	parts := strings.SplitN(v, ",", 3)
	if len(parts) != 3 {
		return fmt.Errorf("expected lat,lon,label, got %q", v)
	}
	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return fmt.Errorf("invalid latitude: %w", err)
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return fmt.Errorf("invalid longitude: %w", err)
	}
	*p = append(*p, pinFlag{Lat: lat, Lon: lon, Label: strings.TrimSpace(parts[2])})
	return nil
}

func main() {
	tileURL := flag.String("tile-url", "", `vector tile URL template, e.g. "https://example.com/tiles/{z}/{x}/{y}.pbf" (required)`)
	lat := flag.Float64("lat", 48.8566, "initial center latitude")
	lon := flag.Float64("lon", 2.3522, "initial center longitude")
	zoom := flag.Float64("zoom", 12, "initial zoom level")
	shapeName := flag.String("shape", "braille", "render shape: braille, blocks, or ascii")
	headers := make(headerFlags)
	flag.Var(headers, "header", `extra HTTP header sent with tile requests, as "Key: Value" (repeatable)`)
	var pins pinFlags
	flag.Var(&pins, "pin", "lat,lon,label pin to place on the map (repeatable)")
	flag.Parse()

	if *tileURL == "" {
		fmt.Fprintln(os.Stderr, "mapscii-demo: -tile-url is required")
		flag.Usage()
		os.Exit(2)
	}
	shape, err := parseShape(*shapeName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mapscii-demo:", err)
		os.Exit(2)
	}

	providerOpts := []tileprovider.HTTPProviderOption{}
	for k, v := range headers {
		providerOpts = append(providerOpts, tileprovider.WithHeader(k, v))
	}
	provider := tileprovider.NewHTTPProvider(*tileURL, providerOpts...)

	width, height, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		width, height = 80, 24
	}
	height-- // reserve the last row for a status line

	m, err := mapscii.New(mapscii.Options{
		Provider: provider,
		Width:    width,
		Height:   height,
		Center:   mapscii.LatLon{Lat: *lat, Lon: *lon},
		Zoom:     *zoom,
		Shape:    shape,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "mapscii-demo:", err)
		os.Exit(1)
	}
	for _, p := range pins {
		m.AddPin(p.Lat, p.Lon, p.Label)
	}

	if err := runInteractive(m, *lat, *lon, *zoom); err != nil {
		fmt.Fprintln(os.Stderr, "mapscii-demo:", err)
		os.Exit(1)
	}
}

func runInteractive(m *mapscii.Map, startLat, startLon, startZoom float64) error {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("entering raw terminal mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	out := os.Stdout
	fmt.Fprint(out, "\x1b[?1049h\x1b[?25l") // alternate screen, hide cursor
	defer fmt.Fprint(out, "\x1b[?25h\x1b[?1049l")

	resized := make(chan os.Signal, 1)
	signal.Notify(resized, syscall.SIGWINCH)
	defer signal.Stop(resized)

	redraw := make(chan struct{}, 1)
	trigger := func() {
		select {
		case redraw <- struct{}{}:
		default:
		}
	}
	trigger()

	go func() {
		for range resized {
			if w, h, err := term.GetSize(fd); err == nil {
				m.Resize(w, h-1)
			}
			trigger()
		}
	}()

	shapeIdx := 0
	for i, s := range shapeCycle {
		if s.Name == m.Shape().Name {
			shapeIdx = i
			break
		}
	}
	cycleShape := func() {
		shapeIdx = (shapeIdx + 1) % len(shapeCycle)
		m.SetShape(shapeCycle[shapeIdx])
	}

	quit := make(chan struct{})
	go readKeys(os.Stdin, m, startLat, startLon, startZoom, trigger, cycleShape, quit)

	ctx := context.Background()
	for {
		select {
		case <-quit:
			return nil
		case <-redraw:
			frame, err := m.Render(ctx)
			if err != nil {
				return err
			}
			w, h := m.Size()
			var b strings.Builder
			b.WriteString("\x1b[H")
			for _, line := range strings.Split(frame, "\n") {
				b.WriteString(line)
				b.WriteString("\x1b[K\r\n")
			}
			c := m.Center()
			fmt.Fprintf(&b, "\x1b[7m %dx%d  %.5f,%.5f  z%.2f  %s  arrows=pan +/-=zoom s=shape r=reset q=quit \x1b[0m\x1b[K",
				w, h, c.Lat, c.Lon, m.Zoom(), m.Shape().Name)
			fmt.Fprint(out, b.String())
		}
	}
}

// readKeys reads raw terminal input, translating arrow keys and a
// handful of single-character commands into map operations. It runs
// until stdin is closed or a quit command is read.
func readKeys(in *os.File, m *mapscii.Map, startLat, startLon, startZoom float64, trigger, cycleShape func(), quit chan<- struct{}) {
	r := bufio.NewReader(in)
	panStep := 6.0
	for {
		b, err := r.ReadByte()
		if err != nil {
			close(quit)
			return
		}
		switch b {
		case 'q', 3: // q or Ctrl-C
			close(quit)
			return
		case '+', '=':
			m.ZoomBy(0.5)
			trigger()
		case '-', '_':
			m.ZoomBy(-0.5)
			trigger()
		case 's':
			cycleShape()
			trigger()
		case 'r':
			m.SetCenter(startLat, startLon)
			m.SetZoom(startZoom)
			trigger()
		case 0x1b: // ESC: possibly the start of an arrow key sequence
			b2, err := r.ReadByte()
			if err != nil || b2 != '[' {
				continue
			}
			b3, err := r.ReadByte()
			if err != nil {
				continue
			}
			switch b3 {
			case 'A': // up
				m.Pan(0, -panStep)
			case 'B': // down
				m.Pan(0, panStep)
			case 'C': // right
				m.Pan(panStep, 0)
			case 'D': // left
				m.Pan(-panStep, 0)
			default:
				continue
			}
			trigger()
		}
	}
}
