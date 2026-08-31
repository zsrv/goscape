package ondemand

import (
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/cache"
)

// publicMimeTypes mirrors web.ts:26-32 MIME_TYPES. The TS fallback Content-Type
// for unknown extensions is "text/plain" (web.ts:117), not the stdlib
// application/octet-stream sniff — so we mirror the map explicitly rather than
// rely on mime.TypeByExtension / http.ServeFile's sniffing.
var publicMimeTypes = map[string]string{
	".js":   "application/javascript",
	".mjs":  "application/javascript",
	".css":  "text/css",
	".html": "text/html",
	".wasm": "application/wasm",
	".sf2":  "application/octet-stream",
}

// archiveRoutes maps URL prefix → archive-0 file index.
// Verified against Engine-TS 9aadcec4 src/web.ts:65-80:
//
//	/title       → cache.read(0, 1)
//	/config      → cache.read(0, 2)
//	/interface   → cache.read(0, 3)
//	/media       → cache.read(0, 4)
//	/versionlist → cache.read(0, 5)  (NEW at 244; replaces 225's /models loose-file route)
//	/textures    → cache.read(0, 6)
//	/wordenc     → cache.read(0, 7)
//	/sounds      → cache.read(0, 8)
//
// Prefix order matters: more-specific prefixes must appear before less-specific
// ones (e.g. /versionlist before /v, if any). The table is checked in order.
var archiveRoutes = []struct {
	prefix string
	file   int
}{
	{"/title", 1},
	{"/config", 2},
	{"/interface", 3},
	{"/media", 4},
	{"/versionlist", 5},
	{"/textures", 6},
	{"/wordenc", 7},
	{"/sounds", 8},
}

// archiveCRCMatches reports whether the trailing portion of an archive route
// parses as the CRC that archive currently has.
//
// TS parses with tryParseInt(crc, -1) and compares against CrcTable[file], so
// an absent or unparseable suffix yields -1 and can never match a real CRC.
// The CRC is a signed int32 on the wire (CrcTable holds getcrc output), so the
// comparison is done in int32 space; parsing into uint32 and comparing would
// reject every negative CRC.
func archiveCRCMatches(suffix string, file int) bool {
	snap := cache.CRC()
	if snap == nil || file < 0 || file >= len(snap.Table) {
		return false
	}
	got, err := strconv.ParseInt(suffix, 10, 64)
	if err != nil {
		return false
	}
	return int32(got) == int32(snap.Table[file])
}

// isValidMapName matches the m{x}_{z} / l{x}_{z} cache-key convention used
// by goscape-client (see client.go:9479 / 9496). Anything else is rejected
// so the path joined under data/pack/client/maps cannot escape that dir
// via "..", absolute paths, or unexpected segments.
func isValidMapName(s string) bool {
	if len(s) < 4 || (s[0] != 'm' && s[0] != 'l') {
		return false
	}
	underscore := -1
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c == '_' {
			if underscore != -1 {
				return false
			}
			underscore = i
			continue
		}
		if c < '0' || c > '9' {
			return false
		}
	}
	// require at least one digit on each side of the single underscore
	return underscore > 1 && underscore < len(s)-1
}

func (a *OnDemand) RootHandler(w http.ResponseWriter, r *http.Request) {
	// /crc — archive checksums (unchanged from 225).
	if strings.HasPrefix(r.URL.Path, "/crc") {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write(cache.CRC().Bytes)
		return
	}

	// Archive-0 routes: /title, /config, /interface, /media, /versionlist,
	// /textures, /wordenc, /sounds — each reads from FileStream cache.read(0, N).
	// Verified against Engine-TS 9aadcec4 src/web.ts:65-80.
	// Missing-file posture: 404 (established goscape-ondemand posture; TS would
	// 500 via Bun non-null assertion on cache.read(0, N)! — decision row).
	for _, ar := range archiveRoutes {
		if strings.HasPrefix(r.URL.Path, ar.prefix) {
			// CRC gate — TS web.ts:164-250 @1d25566c. Engine-TS 8139461a
			// moved these routes from unconditional prefix matches to
			// `/<name>:crc` handlers that 404 unless the trailing value
			// equals CrcTable[n]. The client always appends the CRC it holds,
			// so a stale request now fails fast instead of being served an
			// archive the client will reject anyway.
			//
			// /crc itself stays ungated (handled above): it is how the client
			// learns the CRCs in the first place.
			if !archiveCRCMatches(r.URL.Path[len(ar.prefix):], ar.file) {
				http.NotFound(w, r)
				return
			}
			a.serveArchive(w, r, 0, ar.file)
			return
		}
	}

	// rev-274: the /ondemand.zip and /build static routes (added at 244 from
	// web.ts:81-84) were DROPPED upstream — TS web.ts (dee467c8) no longer
	// serves either, and PackAll.ts no longer emits the backing
	// data/pack/ondemand.zip or data/pack/server/build artifacts. Routes
	// removed; the rev244-b6-* exceptions are retired (see docs/PORTING.md, T26).

	// /maps/ — goscape-specific HTTP cache fallback for per-zone map/loc files.
	// No analog in 244 web.ts; kept as-is for the goscape-client CacheHTTPFallback
	// arrangement. Revisit at B6 when the client arrangement changes.
	// The name is constrained to ^[ml]\d+_\d+$ to guarantee the joined path
	// resolves under data/pack/client/maps.
	if name, ok := strings.CutPrefix(r.URL.Path, "/maps/"); ok {
		if !isValidMapName(name) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, filepath.Join("data", "pack", "client", "maps", name))
		return
	}

	// /rs2.cgi Java applet bootstrap — mirrors web.ts:88-113. Matched before
	// the public/ fallback to preserve TS dispatch order.
	if r.URL.Path == "/rs2.cgi" {
		a.Rs2CgiHandler(w, r)
		return
	}

	// TODO: redirect / to rs2.cgi?

	// public/ static-file fallback — mirrors web.ts:114-119.
	// http.Dir handles path-traversal safety (rejects ".." escape and absolute
	// paths via cleaning + filepath.Join scoped to the root).
	if a.servePublic(w, r) {
		return
	}

	a.log.Debug("unmatched path", "path", r.URL.Path, "sourceIPs", a.clientIP(r))
	http.NotFound(w, r)
}

// serveArchive serves archive/file from the module's FileStream under cacheMu.
// Returns 404 when the cache is not configured or the file is not present.
func (a *OnDemand) serveArchive(w http.ResponseWriter, r *http.Request, archive, file int) {
	a.cacheMu.Lock()
	var data []byte
	if a.cache != nil {
		data = a.cache.Read(archive, file, false)
	}
	a.cacheMu.Unlock()

	if data == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// clientIP returns the request's source IP per the configured
// SourceIPExtractor (defaults to cf-connecting-ip / x-forwarded-for to match
// the TS upstream's web.ts getIp()), falling back to r.RemoteAddr. Returns
// the empty string when no extractor is configured.
func (a *OnDemand) clientIP(r *http.Request) string {
	if a.sourceIPs == nil {
		return ""
	}
	return a.sourceIPs.Get(r)
}

// servePublic serves r from the configured public dir. Returns true if the
// request was handled (success or 4xx surfaced by the stdlib), false if no
// public root is configured or the path did not resolve to a regular file —
// in which case the caller should fall through to 404.
func (a *OnDemand) servePublic(w http.ResponseWriter, r *http.Request) bool {
	if a.cfg.PublicDir == "" {
		return false
	}

	root := http.Dir(a.cfg.PublicDir)
	f, err := root.Open(r.URL.Path)
	if err != nil {
		return false
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil || stat.IsDir() {
		return false
	}

	ct, ok := publicMimeTypes[strings.ToLower(path.Ext(r.URL.Path))]
	if !ok {
		ct = "text/plain"
	}
	w.Header().Set("Content-Type", ct)

	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
	return true
}
