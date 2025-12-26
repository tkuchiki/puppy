package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type fileCache struct{ dir string }

func newFileCache(dir string) (*fileCache, error) {
	if dir == "" {
		dir = "./ddapm_cache"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &fileCache{dir: dir}, nil
}

func (c *fileCache) key(op, site string, body any) string {
	b, _ := json.Marshal(body)
	sum := sha256.Sum256(b)
	return filepath.Join(c.dir, fmt.Sprintf("%s_%s_%s.json", op, site, hex.EncodeToString(sum[:])))
}

func (c *fileCache) Get(op, site string, body any, ttl time.Duration, out any) (bool, error) {
	p := c.key(op, site, body)
	fi, err := os.Stat(p)
	if err != nil {
		return false, nil
	}
	if time.Since(fi.ModTime()) > ttl {
		return false, nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(b, out)
}

func (c *fileCache) Set(op, site string, body any, v any) {
	p := c.key(op, site, body)
	b, _ := json.Marshal(v)
	tmp := p + ".tmp"
	_ = os.WriteFile(tmp, b, 0o644)
	_ = os.Rename(tmp, p)
}
