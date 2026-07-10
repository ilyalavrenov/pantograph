package render

import (
	"fmt"
	"runtime"
	"sync"

	"golang.org/x/sync/singleflight"
)

var compileSem = make(chan struct{}, runtime.GOMAXPROCS(0)) //nolint:gochecknoglobals // process-wide CPU bound for the d2 compile path

func withCompileSlot(fn func()) {
	compileSem <- struct{}{}
	defer func() { <-compileSem }()

	fn()
}

type compileCache struct {
	sf singleflight.Group
	mu sync.RWMutex
	m  map[string][]byte
}

func newCompileCache() *compileCache {
	return &compileCache{m: make(map[string][]byte)}
}

func (c *compileCache) compile(d2src string) ([]byte, error) {
	c.mu.RLock()
	svg, ok := c.m[d2src]
	c.mu.RUnlock()

	if ok {
		return svg, nil
	}

	v, err, _ := c.sf.Do(d2src, func() (any, error) {
		out, cErr := compileSVG(d2src)
		if cErr != nil {
			return nil, cErr
		}

		c.mu.Lock()
		c.m[d2src] = out
		c.mu.Unlock()

		return out, nil
	})
	if err != nil {
		return nil, fmt.Errorf("compile (cached): %w", err)
	}

	svg, ok = v.([]byte)
	if !ok {
		return nil, fmt.Errorf("compile cache: singleflight returned %T, want []byte", v)
	}

	return svg, nil
}
