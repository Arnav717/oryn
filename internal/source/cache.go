package source

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var cacheDir string

// SetCacheDir enables keeping slow lookups on disk between runs.
func SetCacheDir(dir string) { cacheDir = dir }

type cacheEntry[V any] struct {
	Value V         `json:"v"`
	At    time.Time `json:"at"`
}

// diskCache remembers lookups that rarely change. An expired entry is still
// returned, marked stale, so a failed fetch can fall back on it.
type diskCache[V any] struct {
	name string
	ttl  time.Duration

	mu     sync.Mutex
	loaded string // the dir it was loaded from
	items  map[string]cacheEntry[V]
}

func (c *diskCache[V]) path() string { return filepath.Join(cacheDir, c.name) }

// load reads the file once per directory. Holds mu.
func (c *diskCache[V]) load() {
	if c.items != nil && c.loaded == cacheDir {
		return
	}
	c.items, c.loaded = map[string]cacheEntry[V]{}, cacheDir
	if cacheDir == "" {
		return
	}
	if b, err := os.ReadFile(c.path()); err == nil && json.Unmarshal(b, &c.items) != nil {
		c.items = map[string]cacheEntry[V]{} // unreadable: start over
	}
}

func (c *diskCache[V]) get(key string, now time.Time) (v V, found, fresh bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	e, ok := c.items[key]
	return e.Value, ok, ok && now.Sub(e.At) < c.ttl
}

func (c *diskCache[V]) put(key string, v V, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	c.items[key] = cacheEntry[V]{v, now}
}

// replace keeps only this entry, for a cache where old keys are never read again.
func (c *diskCache[V]) replace(key string, v V, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.load()
	c.items = map[string]cacheEntry[V]{key: {v, now}}
}

func (c *diskCache[V]) save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cacheDir == "" || c.items == nil {
		return nil
	}
	b, err := json.Marshal(c.items)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(cacheDir, c.name+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), c.path())
}
