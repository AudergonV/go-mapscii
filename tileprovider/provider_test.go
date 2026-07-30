package tileprovider

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/audergonv/go-mapscii/geo"
)

func TestLRUCacheEvictsOldest(t *testing.T) {
	c := newLRUCache(2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // evicts "a"

	if _, ok := c.Get("a"); ok {
		t.Error("expected \"a\" to be evicted")
	}
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Errorf("Get(b) = %v, %v", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Errorf("Get(c) = %v, %v", v, ok)
	}
}

func TestLRUCacheTouchOnGet(t *testing.T) {
	c := newLRUCache(2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Get("a")    // "a" is now most recently used
	c.Put("c", 3) // should evict "b", not "a"

	if _, ok := c.Get("b"); ok {
		t.Error("expected \"b\" to be evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("expected \"a\" to survive eviction")
	}
}

func TestMemoryProviderReturnsEmptyForUnknownTile(t *testing.T) {
	p := NewMemoryProvider()
	tile, err := p.FetchTile(context.Background(), geo.TileIndex{Z: 1, X: 2, Y: 3})
	if err != nil {
		t.Fatalf("FetchTile: %v", err)
	}
	if len(tile.Layers) != 0 {
		t.Errorf("expected empty tile, got %d layers", len(tile.Layers))
	}
}

func TestHTTPProviderFetchesAndCaches(t *testing.T) {
	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/2/3/4.pbf" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Write(buildEmptyTileBytes())
	}))
	defer srv.Close()

	p := NewHTTPProvider(srv.URL + "/{z}/{x}/{y}.pbf")
	idx := geo.TileIndex{Z: 2, X: 3, Y: 4}

	if _, err := p.FetchTile(context.Background(), idx); err != nil {
		t.Fatalf("FetchTile: %v", err)
	}
	if _, err := p.FetchTile(context.Background(), idx); err != nil {
		t.Fatalf("FetchTile (cached): %v", err)
	}
	if requests != 1 {
		t.Errorf("expected 1 HTTP request (second call should hit cache), got %d", requests)
	}
}

func TestHTTPProviderHandlesGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		gw.Write(buildEmptyTileBytes())
		gw.Close()
		w.Write(buf.Bytes())
	}))
	defer srv.Close()

	p := NewHTTPProvider(srv.URL + "/{z}/{x}/{y}.pbf")
	tile, err := p.FetchTile(context.Background(), geo.TileIndex{Z: 0, X: 0, Y: 0})
	if err != nil {
		t.Fatalf("FetchTile: %v", err)
	}
	if len(tile.Layers) != 0 {
		t.Errorf("expected empty decoded tile, got %d layers", len(tile.Layers))
	}
}

func TestHTTPProviderReturnsEmptyTileOn404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	p := NewHTTPProvider(srv.URL + "/{z}/{x}/{y}.pbf")
	tile, err := p.FetchTile(context.Background(), geo.TileIndex{Z: 0, X: 0, Y: 0})
	if err != nil {
		t.Fatalf("FetchTile: %v", err)
	}
	if len(tile.Layers) != 0 {
		t.Errorf("expected empty tile for 404, got %d layers", len(tile.Layers))
	}
}

func TestHTTPProviderErrorsOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewHTTPProvider(srv.URL + "/{z}/{x}/{y}.pbf")
	_, err := p.FetchTile(context.Background(), geo.TileIndex{Z: 0, X: 0, Y: 0})
	if err == nil {
		t.Fatal("expected error for 500 response, got nil")
	}
}

// buildEmptyTileBytes returns a minimal, valid but empty MVT payload
// (zero layers).
func buildEmptyTileBytes() []byte {
	return []byte{}
}
