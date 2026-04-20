package sysmon

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/kaicmurilo/tokalytics/pkg/utils"
)

// DiskPath é um diretório ou arquivo contado no total em disco da ferramenta.
type DiskPath struct {
	Path  string `json:"path"`
	Bytes uint64 `json:"bytes"`
}

const diskCacheTTL = 45 * time.Second

var (
	diskCacheMu   sync.Mutex
	diskCacheAt   time.Time
	diskCacheData map[string][]DiskPath
)

func shortenHome(p, home string) string {
	if home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func shortenHomeAny(p string, homes []string) string {
	for _, home := range homes {
		if home != "" && strings.HasPrefix(p, home) {
			return "~" + strings.TrimPrefix(p, home)
		}
	}
	return p
}

func pathDiskUsage(p string) uint64 {
	fi, err := os.Stat(p)
	if err != nil {
		return 0
	}
	if !fi.IsDir() {
		return uint64(fi.Size())
	}
	var n uint64
	_ = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		n += uint64(info.Size())
		return nil
	})
	return n
}

func appendToolPaths(home string, toolID string, out *[]string) {
	switch toolID {
	case "claude":
		*out = append(*out, filepath.Join(home, ".claude"))
		if f := filepath.Join(home, ".claude.json"); fileExists(f) {
			*out = append(*out, f)
		}
	case "cursor":
		*out = append(*out, filepath.Join(home, ".cursor"))
		switch runtime.GOOS {
		case "darwin":
			*out = append(*out, filepath.Join(home, "Library", "Application Support", "Cursor"))
		case "windows":
			if ap := os.Getenv("APPDATA"); ap != "" {
				*out = append(*out, filepath.Join(ap, "Cursor"))
			}
			if loc := os.Getenv("LOCALAPPDATA"); loc != "" {
				*out = append(*out, filepath.Join(loc, "Cursor"))
			}
		default:
			if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
				*out = append(*out, filepath.Join(xdg, "Cursor"))
			} else {
				*out = append(*out, filepath.Join(home, ".config", "Cursor"))
			}
		}
	case "gemini":
		*out = append(*out, filepath.Join(home, ".gemini"))
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// diskUsageByTool retorna, por id de ferramenta, lista de caminhos com bytes (cache TTL).
func diskUsageByTool() map[string][]DiskPath {
	diskCacheMu.Lock()
	defer diskCacheMu.Unlock()
	if time.Since(diskCacheAt) < diskCacheTTL && diskCacheData != nil {
		return diskCacheData
	}

	homes := utils.DataHomeRoots()
	if len(homes) == 0 {
		diskCacheData = map[string][]DiskPath{}
		diskCacheAt = time.Now()
		return diskCacheData
	}

	next := make(map[string][]DiskPath)
	for _, id := range []string{"cursor", "claude", "gemini"} {
		var raw []string
		for _, home := range homes {
			appendToolPaths(home, id, &raw)
		}
		seen := map[string]struct{}{}
		var parts []DiskPath
		var sum uint64
		for _, p := range raw {
			ap, err := filepath.Abs(p)
			if err != nil {
				ap = p
			}
			if _, ok := seen[ap]; ok {
				continue
			}
			seen[ap] = struct{}{}
			b := pathDiskUsage(ap)
			if b == 0 && !fileExists(ap) {
				continue
			}
			sum += b
			parts = append(parts, DiskPath{
				Path:  shortenHomeAny(ap, homes),
				Bytes: b,
			})
		}
		if sum > 0 || len(parts) > 0 {
			next[id] = parts
		}
	}

	diskCacheData = next
	diskCacheAt = time.Now()
	return diskCacheData
}
