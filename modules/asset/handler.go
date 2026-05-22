package asset

import (
	"net/http"
	"path"
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

func (a *Asset) RootHandler(w http.ResponseWriter, r *http.Request) {
	// client concats the prefix with the expected crc from the initial /crc call (or the one it has cached? idk)
	// should make a way for the server to store all the crcs and check against them when they're requested
	// then reject a request if a crc we don't have is requested or something

	if strings.HasPrefix(r.URL.Path, "/crc") { // archive checksums
		// the number appended to the url is random
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		w.Write(cache.CRC().Bytes)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/title") { // title screen
		// TODO: check [http.Dir.Open] for path sanitization ideas
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client", "title"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/config") { // config
		// TODO: check [http.Dir.Open] for path sanitization ideas
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client", "config"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/interface") { // interface
		// TODO: check [http.Dir.Open] for path sanitization ideas
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client", "interface"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/media") { // 2d graphics
		// TODO: check [http.Dir.Open] for path sanitization ideas
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client", "media"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/models") { // 3d graphics
		// TODO: check [http.Dir.Open] for path sanitization ideas
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client", "models"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/textures") { // textures
		// TODO: check [http.Dir.Open] for path sanitization ideas
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client", "textures"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/wordenc") { // chat system
		// TODO: check [http.Dir.Open] for path sanitization ideas
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client", "wordenc"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/sounds") { // sound effects
		// TODO: check [http.Dir.Open] for path sanitization ideas
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client", "sounds"))
		return
	}
	if strings.HasSuffix(r.URL.Path, ".mid") {
		a.log.Debug("rootHandler mid", "path", r.URL.Path)

		// TODO: packing process should spit out files with crc included in
		//  the name, but the server needs to be aware of the crc so it can
		//  send the proper length, so that's been pushed off till later...

		// strip _crc from filename, but keep extension
		filename := r.URL.Path[1:strings.LastIndex(r.URL.Path, "_")] + ".mid"

		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client/songs", filename))
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

// clientIP returns the request's source IP per the configured
// SourceIPExtractor (defaults to cf-connecting-ip / x-forwarded-for to match
// the TS upstream's web.ts getIp()), falling back to r.RemoteAddr. Returns
// the empty string when no extractor is configured.
func (a *Asset) clientIP(r *http.Request) string {
	if a.sourceIPs == nil {
		return ""
	}
	return a.sourceIPs.Get(r)
}

// servePublic serves r from the configured public dir. Returns true if the
// request was handled (success or 4xx surfaced by the stdlib), false if no
// public root is configured or the path did not resolve to a regular file —
// in which case the caller should fall through to 404.
func (a *Asset) servePublic(w http.ResponseWriter, r *http.Request) bool {
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
