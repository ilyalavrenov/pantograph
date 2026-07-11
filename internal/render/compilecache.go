package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
)

var compileSem = make(chan struct{}, runtime.GOMAXPROCS(0)) //nolint:gochecknoglobals // process-wide CPU bound for the d2 compile path

func withCompileSlot(fn func()) {
	compileSem <- struct{}{}
	defer func() { <-compileSem }()

	fn()
}

type compileCache struct {
	mu      sync.RWMutex
	m       map[string][]byte
	dir     string // "" disables disk caching
	initDir sync.Once
}

func newCompileCache() *compileCache {
	return &compileCache{m: make(map[string][]byte)}
}

func newDiskCompileCache(dir string) *compileCache {
	return &compileCache{m: make(map[string][]byte), dir: dir}
}

const cacheEnvVar = "PANTOGRAPH_CACHE_DIR"

func cacheRoot() string {
	if dir := os.Getenv(cacheEnvVar); dir != "" {
		return dir
	}

	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}

	return filepath.Join(base, "pantograph")
}

//nolint:gochecknoglobals // process-wide memo of the build identity
var (
	buildIDOnce sync.Once
	buildIDVal  string
)

func buildID() string {
	buildIDOnce.Do(func() {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}

		parts := []string{bi.Main.Version}
		for _, s := range bi.Settings {
			if s.Key == "vcs.revision" || s.Key == "vcs.modified" {
				parts = append(parts, s.Value)
			}
		}

		for _, d := range bi.Deps {
			if d.Path == "oss.terrastruct.com/d2" {
				parts = append(parts, d.Version)

				break
			}
		}

		buildIDVal = strings.Join(parts, "/")
	})

	return buildIDVal
}

func cacheKey(d2src string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s", buildID(), d2src)

	return hex.EncodeToString(h.Sum(nil))
}

func (c *compileCache) compile(d2src string) ([]byte, error) {
	key := cacheKey(d2src)

	c.mu.RLock()
	svg, ok := c.m[key]
	c.mu.RUnlock()

	if ok {
		return svg, nil
	}

	if svg, ok := c.readDisk(key); ok {
		c.store(key, svg)

		return svg, nil
	}

	out, err := compileSVG(d2src)
	if err != nil {
		return nil, fmt.Errorf("compile (cached): %w", err)
	}

	c.store(key, out)
	c.writeDisk(key, out)

	return out, nil
}

func (c *compileCache) store(key string, svg []byte) {
	c.mu.Lock()
	c.m[key] = svg
	c.mu.Unlock()
}

func (c *compileCache) diskPath(key string) string {
	return filepath.Join(c.dir, key+".svg")
}

func (c *compileCache) readDisk(key string) ([]byte, bool) {
	if c.dir == "" {
		return nil, false
	}

	b, err := os.ReadFile(c.diskPath(key))

	return b, err == nil
}

func (c *compileCache) writeDisk(key string, svg []byte) {
	if c.dir == "" {
		return
	}

	c.initDir.Do(func() {
		_ = os.MkdirAll(c.dir, 0o755) //nolint:errcheck,mnd // best-effort
	})

	// Write-then-rename so a killed process can't leave a truncated file that a
	// later run serves as a valid hit.
	tmp, err := os.CreateTemp(c.dir, key+".*.tmp")
	if err != nil {
		return
	}

	if _, err := tmp.Write(svg); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())

		return
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())

		return
	}

	_ = os.Rename(tmp.Name(), c.diskPath(key)) //nolint:errcheck // best-effort cache of generated SVG
}
