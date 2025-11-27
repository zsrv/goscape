package asset

import (
	"io"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/zsrv/goscape/pkg/cache"
)

func (a *Asset) RootHandler(w http.ResponseWriter, r *http.Request) {
	// client concats the prefix with the expected crc from the initial /crc call (or the one it has cached? idk)
	// should make a way for the server to store all the crcs and check against them when they're requested
	// then reject a request if a crc we don't have is requested or something

	if strings.HasPrefix(r.URL.Path, "/crc") { // archive checksums
		// the number appended to the url is random
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		// would have to use bytes.Reader (implements ReadSeeker)
		//http.ServeContent(w, r, "", nil, cache.CrcBuffer)
		cache.MakeCRCs() // TEST - belongs in world
		io.Copy(w, cache.CrcBuffer)
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
		log.Println("rootHandler mid " + r.URL.Path)

		// TODO: packing process should spit out files with crc included in
		//  the name, but the server needs to be aware of the crc so it can
		//  send the proper length, so that's been pushed off till later...

		// strip _crc from filename, but keep extension
		filename := r.URL.Path[1:strings.LastIndex(r.URL.Path, "_")] + ".mid"

		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, path.Join("data/pack/client/songs", filename))
		return
	}

	// TODO: redirect / to rs2.cgi?

	a.log.Debug("unmatched path", "path", r.URL.Path)
	http.NotFound(w, r)
}
