// Package pack ports the source-side packing-pipeline foundation from
// LostCityRS/Engine-TS/tools/pack/*.ts. It provides format-agnostic
// primitives — filesystem caches, source-file parsers, the PackFile
// name↔id registry struct, file-freshness helpers, and a generic
// [header]-block crawler — that downstream per-config packer slices
// (NAI-192+) build on. No production callsites in this slice.
package pack

import (
	"os"
	"strings"
	"sync"
)

// NAI-191-D-CONCURRENCY: TS tools/pack/FsCache.ts uses unguarded
// module-level Maps (single-threaded). Goscape guards the equivalent
// caches with a single RWMutex so the eventual ::rebuild worker
// goroutine can call freely from a non-tick context.
var (
	fsCacheMu   sync.RWMutex
	dirCache    = map[string][]string{}
	existsCache = map[string]bool{}
	statsCache  = map[string]os.FileInfo{}
)

// ClearFsCache resets the memoized dir/exists/stat lookups.
func ClearFsCache() {
	fsCacheMu.Lock()
	defer fsCacheMu.Unlock()
	dirCache = map[string][]string{}
	existsCache = map[string]bool{}
	statsCache = map[string]os.FileInfo{}
}

// FileExists reports whether path exists on disk. Result is memoized
// until ClearFsCache.
func FileExists(path string) bool {
	fsCacheMu.RLock()
	if v, ok := existsCache[path]; ok {
		fsCacheMu.RUnlock()
		return v
	}
	fsCacheMu.RUnlock()

	_, err := os.Stat(path)
	exists := err == nil

	fsCacheMu.Lock()
	existsCache[path] = exists
	fsCacheMu.Unlock()
	return exists
}

// FileStat returns os.FileInfo for path, memoized.
func FileStat(path string) (os.FileInfo, error) {
	fsCacheMu.RLock()
	if v, ok := statsCache[path]; ok {
		fsCacheMu.RUnlock()
		return v, nil
	}
	fsCacheMu.RUnlock()

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	fsCacheMu.Lock()
	statsCache[path] = info
	fsCacheMu.Unlock()
	return info, nil
}

// ListDir returns all entries (recursive) under path. Subdirectory
// entries are suffixed "/" to match TS tools/pack/FsCache.ts. Returns
// nil for missing paths. Cached entries are the bare directory contents
// (e.g. "a.txt", "sub/"); the returned paths prepend the input path
// (e.g. "<path>/a.txt").
func ListDir(path string) []string {
	path = strings.TrimSuffix(path, "/")

	fsCacheMu.RLock()
	cached, ok := dirCache[path]
	fsCacheMu.RUnlock()

	var files []string
	if ok {
		files = cached
	} else {
		if _, err := os.Stat(path); err != nil {
			return nil
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}
		files = make([]string, 0, len(entries))
		for _, e := range entries {
			if e.IsDir() {
				files = append(files, e.Name()+"/")
			} else {
				files = append(files, e.Name())
			}
		}
		fsCacheMu.Lock()
		dirCache[path] = files
		fsCacheMu.Unlock()
	}

	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, path+"/"+f)
		if strings.HasSuffix(f, "/") {
			out = append(out, ListDir(path+"/"+f)...)
		}
	}
	return out
}

// ListFiles is TS parity for listFiles(path, out=[]). Equivalent to
// ListDir; subdirectory-suffixed entries are included in the output.
func ListFiles(path string) []string {
	return ListDir(path)
}
