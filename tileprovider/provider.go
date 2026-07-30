// Package tileprovider fetches and decodes Mapbox Vector Tiles from a
// remote tile server, with in-memory caching of decoded tiles.
package tileprovider

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/audergonv/go-mapscii/geo"
	"github.com/audergonv/go-mapscii/vectortile"
)

// Provider fetches and decodes a single vector tile.
type Provider interface {
	FetchTile(ctx context.Context, tile geo.TileIndex) (*vectortile.Tile, error)
}

// HTTPProvider fetches tiles over HTTP(S) from a server that serves
// Mapbox Vector Tiles following the {z}/{x}/{y} Slippy Map convention,
// caching decoded tiles in memory.
type HTTPProvider struct {
	urlTemplate string
	client      *http.Client
	headers     map[string]string
	cache       *lruCache
}

// HTTPProviderOption configures an HTTPProvider.
type HTTPProviderOption func(*HTTPProvider)

// WithHTTPClient overrides the default http.Client used to fetch tiles.
func WithHTTPClient(client *http.Client) HTTPProviderOption {
	return func(p *HTTPProvider) { p.client = client }
}

// WithHeader sets an additional HTTP header sent with every tile
// request, useful for API keys required by some tile providers.
func WithHeader(key, value string) HTTPProviderOption {
	return func(p *HTTPProvider) { p.headers[key] = value }
}

// WithCacheSize sets how many decoded tiles are kept in memory. The
// default is 256 tiles.
func WithCacheSize(n int) HTTPProviderOption {
	return func(p *HTTPProvider) { p.cache = newLRUCache(n) }
}

// NewHTTPProvider creates a Provider that fetches tiles from
// urlTemplate, a URL containing the literal placeholders "{z}", "{x}"
// and "{y}", e.g. "https://example.com/tiles/{z}/{x}/{y}.pbf".
//
// go-mapscii intentionally ships with no default tile server: point
// this at a vector tile source you are authorized to use, such as a
// self-hosted OpenMapTiles/tileserver-gl instance, or a commercial
// provider's vector tile endpoint (usually requiring an API key set via
// WithHeader or embedded in urlTemplate).
func NewHTTPProvider(urlTemplate string, opts ...HTTPProviderOption) *HTTPProvider {
	p := &HTTPProvider{
		urlTemplate: urlTemplate,
		client:      &http.Client{Timeout: 15 * time.Second},
		headers:     make(map[string]string),
		cache:       newLRUCache(256),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *HTTPProvider) tileURL(t geo.TileIndex) string {
	url := p.urlTemplate
	url = strings.ReplaceAll(url, "{z}", strconv.Itoa(t.Z))
	url = strings.ReplaceAll(url, "{x}", strconv.Itoa(t.X))
	url = strings.ReplaceAll(url, "{y}", strconv.Itoa(t.Y))
	return url
}

// FetchTile fetches and decodes the given tile, returning a cached copy
// if one has already been fetched.
func (p *HTTPProvider) FetchTile(ctx context.Context, t geo.TileIndex) (*vectortile.Tile, error) {
	if cached, ok := p.cache.Get(t); ok {
		return cached.(*vectortile.Tile), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.tileURL(t), nil)
	if err != nil {
		return nil, fmt.Errorf("tileprovider: building request: %w", err)
	}
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tileprovider: fetching tile %d/%d/%d: %w", t.Z, t.X, t.Y, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// A missing tile (e.g. open ocean with no vector data on some
		// servers) is a valid, empty result rather than an error.
		empty := &vectortile.Tile{}
		p.cache.Put(t, empty)
		return empty, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tileprovider: tile %d/%d/%d: unexpected status %s", t.Z, t.X, t.Y, resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tileprovider: reading tile %d/%d/%d: %w", t.Z, t.X, t.Y, err)
	}
	body, err = maybeGunzip(body)
	if err != nil {
		return nil, fmt.Errorf("tileprovider: decompressing tile %d/%d/%d: %w", t.Z, t.X, t.Y, err)
	}

	tile, err := vectortile.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("tileprovider: decoding tile %d/%d/%d: %w", t.Z, t.X, t.Y, err)
	}

	p.cache.Put(t, tile)
	return tile, nil
}

// maybeGunzip transparently decompresses gzip-compressed tile bodies.
// Some tile servers set Content-Encoding: gzip and rely on the HTTP
// client to decompress automatically; others serve pre-gzipped bytes
// with no such header, or sit behind a proxy that strips it. Detecting
// the gzip magic bytes directly handles both cases.
func maybeGunzip(body []byte) ([]byte, error) {
	if len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
		return body, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	return io.ReadAll(zr)
}

// MemoryProvider serves pre-decoded tiles from memory. It is useful for
// tests, and for embedding a fixed set of tiles without any network
// dependency.
type MemoryProvider struct {
	tiles map[geo.TileIndex]*vectortile.Tile
}

// NewMemoryProvider creates an empty MemoryProvider; use Set to
// populate it with tiles.
func NewMemoryProvider() *MemoryProvider {
	return &MemoryProvider{tiles: make(map[geo.TileIndex]*vectortile.Tile)}
}

// Set stores the given decoded tile under the given index.
func (p *MemoryProvider) Set(t geo.TileIndex, tile *vectortile.Tile) {
	p.tiles[t] = tile
}

// FetchTile implements Provider, returning an empty tile for any index
// that was not explicitly set.
func (p *MemoryProvider) FetchTile(_ context.Context, t geo.TileIndex) (*vectortile.Tile, error) {
	if tile, ok := p.tiles[t]; ok {
		return tile, nil
	}
	return &vectortile.Tile{}, nil
}
