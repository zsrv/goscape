// Package filestream ports the 244 engine's FileStream (src/io/FileStream.ts
// at Engine-TS 9aadcec4): the dat/idx client cache store used by OnDemand,
// pack, and unpack tooling.
package filestream

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
)

const (
	sectorSize    = 520
	sectorData    = 512
	maxFileSize   = 2_000_000
	numIdx        = 5
	idxEntrySize  = 6
	sectorHdrSize = 8
)

// FileStream is the RS2 client cache store: main_file_cache.dat plus
// main_file_cache.idx0..idx4. Each idx entry is 6 bytes (size:3 BE,
// firstSector:3 BE); each dat sector is 520 bytes (8-byte header +
// up to 512 payload bytes).
//
// TS FileStream.ts:1-225 (Engine-TS 9aadcec4)
//
// FileStream is not safe for concurrent use; callers must serialize access.
type FileStream struct {
	dat *os.File
	idx [numIdx]*os.File

	// DiscardPacked mirrors TS discardPacked: when true, Read does not
	// populate the packed cache. Exported so callers can toggle it directly,
	// matching the TS field naming convention (camelCase → PascalCase).
	DiscardPacked bool

	packed [numIdx]map[int][]byte
}

// New opens (or creates) the cache files rooted at dir.
//
// TS FileStream.ts:14-32 (constructor)
func New(dir string, createNew, readOnly bool) *FileStream {
	// TS: if (!fs.existsSync(dir)) { fs.mkdirSync(dir, { recursive: true }); }
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic(err)
	}

	// TS: if (createNew || !fs.existsSync(`${dir}/main_file_cache.dat`)) { … }
	datPath := filepath.Join(dir, "main_file_cache.dat")
	if createNew {
		if err := os.WriteFile(datPath, nil, 0o666); err != nil {
			panic(err)
		}
		for i := 0; i <= 4; i++ {
			p := filepath.Join(dir, "main_file_cache.idx"+string(rune('0'+i)))
			if err := os.WriteFile(p, nil, 0o666); err != nil {
				panic(err)
			}
		}
	} else {
		// Create dat if it doesn't exist (mirrors TS "|| !exists" branch).
		if _, err := os.Stat(datPath); os.IsNotExist(err) {
			if err2 := os.WriteFile(datPath, nil, 0o666); err2 != nil {
				panic(err2)
			}
			for i := 0; i <= 4; i++ {
				p := filepath.Join(dir, "main_file_cache.idx"+string(rune('0'+i)))
				if err2 := os.WriteFile(p, nil, 0o666); err2 != nil {
					panic(err2)
				}
			}
		}
	}

	flag := os.O_RDWR
	if readOnly {
		flag = os.O_RDONLY
	}

	datFile, err := os.OpenFile(datPath, flag, 0o666)
	if err != nil {
		panic(err)
	}

	f := &FileStream{}
	f.dat = datFile

	for i := 0; i <= 4; i++ {
		p := filepath.Join(dir, "main_file_cache.idx"+string(rune('0'+i)))
		idxFile, err2 := os.OpenFile(p, flag, 0o666)
		if err2 != nil {
			panic(err2)
		}
		f.idx[i] = idxFile
		f.packed[i] = make(map[int][]byte)
	}

	return f
}

// Close closes all underlying files. This is a Go addition for deterministic
// cleanup (TS relies on GC/process exit; Go requires explicit Close).
func (f *FileStream) Close() error {
	var lastErr error
	if f.dat != nil {
		if err := f.dat.Close(); err != nil {
			lastErr = err
		}
	}
	for i := 0; i < numIdx; i++ {
		if f.idx[i] != nil {
			if err := f.idx[i].Close(); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// datLen returns the current size of the dat file in bytes. TS RandomAccessFile
// exposes a live .length property; Go re-stats each time.
func (f *FileStream) datLen() int64 {
	fi, err := f.dat.Stat()
	if err != nil {
		return 0
	}
	return fi.Size()
}

// idxLen returns the current size of idx[index] in bytes.
func (f *FileStream) idxLen(index int) int64 {
	fi, err := f.idx[index].Stat()
	if err != nil {
		return 0
	}
	return fi.Size()
}

// Count returns the number of files in archive index (idx length / 6).
//
// TS FileStream.ts:35-41 (count)
//
// TS quirk: the guard is `index > this.idx.length` (should be `>=`), but
// `!this.idx[index]` on the next line catches the one case that slips through
// (index == 5 when idx has length 5). Since Go's idx is a fixed [5]*os.File
// and all slots are always populated, we mirror the OBSERVABLE behaviour:
// any index outside [0, 4] returns 0.
func (f *FileStream) Count(index int) int {
	if index < 0 || index >= numIdx || f.idx[index] == nil {
		return 0
	}
	return int(f.idxLen(index) / idxEntrySize)
}

// Read reads file from archive, optionally gunzipping the result.
//
// TS FileStream.ts:44-123 (read)
func (f *FileStream) Read(archive, file int, decompress bool) []byte {
	// TS: if (!this.dat) { return null; }
	if f.dat == nil {
		return nil
	}

	// TS: if (archive < 0 || archive >= this.idx.length || !this.idx[archive]) { return null; }
	if archive < 0 || archive >= numIdx || f.idx[archive] == nil {
		return nil
	}

	// TS: if (file < 0 || file >= this.count(archive)) { return null; }
	if file < 0 || file >= f.Count(archive) {
		return nil
	}

	// TS: if (this.packed[archive][file]) { return this.packed[archive][file]; }
	if cached, ok := f.packed[archive][file]; ok {
		return cached
	}

	// Read the 6-byte idx entry.
	idxBuf := make([]byte, idxEntrySize)
	if _, err := f.idx[archive].ReadAt(idxBuf, int64(file*idxEntrySize)); err != nil {
		return nil
	}
	// TS: idxHeader.g3(), g3()
	size := int(idxBuf[0])<<16 | int(idxBuf[1])<<8 | int(idxBuf[2])
	sector := int(idxBuf[3])<<16 | int(idxBuf[4])<<8 | int(idxBuf[5])

	// TS: if (size > 2000000) { return null; }
	if size > maxFileSize {
		return nil
	}

	// TS: if (sector <= 0 || sector > this.dat.length / 520) { return null; }
	dLen := f.datLen()
	if sector <= 0 || int64(sector) > dLen/sectorSize {
		return nil
	}

	// TS: const data: Packet = new Packet(new Uint8Array(size));
	data := make([]byte, size)
	pos := 0 // data.pos

	// TS: for (let part: number = 0; data.pos < size; part++) {
	for part := 0; pos < size; part++ {
		// TS: if (sector === 0) { break; }
		if sector == 0 {
			break
		}

		// TS: this.dat.pos = sector * 520;
		// TS reads available + 8 bytes total: header (8) + payload (available).
		available := size - pos
		if available > sectorData {
			available = sectorData
		}

		hdrAndData := make([]byte, sectorHdrSize+available)
		if _, err := f.dat.ReadAt(hdrAndData, int64(sector)*sectorSize); err != nil && err != io.EOF {
			return nil
		}

		// TS: header.g2(), g2(), g3(), g1()
		sectorFile := int(hdrAndData[0])<<8 | int(hdrAndData[1])
		sectorPart := int(hdrAndData[2])<<8 | int(hdrAndData[3])
		nextSector := int(hdrAndData[4])<<16 | int(hdrAndData[5])<<8 | int(hdrAndData[6])
		sectorIndex := int(hdrAndData[7])

		// TS: if (file !== sectorFile || part !== sectorPart || archive !== sectorIndex - 1) { return null; }
		if file != sectorFile || part != sectorPart || archive != sectorIndex-1 {
			return nil
		}

		// TS: if (nextSector < 0 || nextSector > this.dat.length / 520) { return null; }
		dLen = f.datLen()
		if nextSector < 0 || int64(nextSector) > dLen/sectorSize {
			return nil
		}

		// TS: data.pdata(header.data, header.pos, header.data.length)
		// header.pos is 8 after reading the 4 header fields; header.data.length
		// is sectorHdrSize+available; so this copies bytes [8, sectorHdrSize+available)
		// = exactly `available` payload bytes into data at the current pos.
		copy(data[pos:], hdrAndData[sectorHdrSize:])
		pos += available

		sector = nextSector
	}

	// TS: if (!decompress) { ... return data.data; }
	if !decompress {
		if !f.DiscardPacked {
			f.packed[archive][file] = data
		}
		return data
	}

	// TS: if (archive === 0) { return data.data; }
	if archive == 0 {
		return data
	}

	// TS: return new Uint8Array(zlib.gunzipSync(data.data));
	// TS throws here (gunzipSync) on bad data; nil is goscape's panic-free analog.
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	out, err := io.ReadAll(r)
	if err != nil {
		// RS2-244 model files are stored as truncated gzip streams (no CRC/ISIZE
		// footer), which causes io.ReadAll to return io.ErrUnexpectedEOF after
		// successfully decompressing all the data. TS's zlib.gunzipSync is lenient
		// and returns the decompressed bytes in this case; mirror that behaviour by
		// accepting unexpected EOF as a non-fatal condition when we have data.
		if err != io.ErrUnexpectedEOF || len(out) == 0 {
			return nil
		}
		return out
	}
	_ = r.Close() // ignore trailer errors for truncated streams
	return out
}

// Write writes data to archive/file, optionally appending a 2-byte big-endian
// version trailer. Returns false on failure.
//
// TS FileStream.ts:131-196 (write)
func (f *FileStream) Write(archive, file int, data []byte, version int) bool {
	// TS: if (!this.dat) { return false; }
	if f.dat == nil {
		return false
	}

	// TS: if (archive < 0 || archive > this.idx.length || !this.idx[archive]) { return false; }
	// TS FileStream.ts:135 uses 'archive > this.idx.length' — a latent off-by-one masked by JS
	// undefined at idx[5] (!undefined → false). The observable contract is "reject archive 5
	// without crashing": Go needs >= numIdx because idx[5] would panic, not nil-check.
	if archive < 0 || archive >= numIdx || f.idx[archive] == nil {
		return false
	}

	// TS: if (version !== 0) { append 2 bytes }
	// TS FileStream.ts:140-147
	if version != 0 {
		withVersion := make([]byte, len(data)+2)
		copy(withVersion, data)
		withVersion[len(data)] = byte(version >> 8)
		withVersion[len(data)+1] = byte(version)
		data = withVersion
	}

	// TS: let sector = Math.trunc((this.dat.length + 519) / 520); if (sector === 0) { sector = 1; }
	dLen := f.datLen()
	sector := int((dLen + 519) / sectorSize)
	if sector == 0 {
		sector = 1
	}

	// Write idx entry: p3(len) p3(firstSector)
	// TS: idx.pos = file * 6; ... idxHeader.p3(data.length); idxHeader.p3(sector);
	idxEntry := [idxEntrySize]byte{
		byte(len(data) >> 16), byte(len(data) >> 8), byte(len(data)),
		byte(sector >> 16), byte(sector >> 8), byte(sector),
	}
	if _, err := f.idx[archive].WriteAt(idxEntry[:], int64(file*idxEntrySize)); err != nil {
		return false
	}

	written := 0
	for part := 0; written < len(data); part++ {
		// TS: let nextSector = Math.trunc((this.dat.length + 519) / 520);
		// this.dat.length is read BEFORE writing this sector (live file size).
		dLen = f.datLen()
		nextSector := int((dLen + 519) / sectorSize)

		// TS: if (nextSector === 0) { nextSector++; }
		if nextSector == 0 {
			nextSector++
		}

		// TS: if (nextSector === sector) { nextSector++; }
		if nextSector == sector {
			nextSector++
		}

		// TS: if (data.length - written <= 512) { nextSector = 0; }
		if len(data)-written <= sectorData {
			nextSector = 0
		}

		// TS: this.dat.pos = sector * 520; write 8-byte header
		hdr := [sectorHdrSize]byte{
			byte(file >> 8), byte(file),
			byte(part >> 8), byte(part),
			byte(nextSector >> 16), byte(nextSector >> 8), byte(nextSector),
			byte(archive + 1),
		}
		if _, err := f.dat.WriteAt(hdr[:], int64(sector)*sectorSize); err != nil {
			return false
		}

		// TS: write up to 512 payload bytes immediately after the header
		available := len(data) - written
		if available > sectorData {
			available = sectorData
		}
		if _, err := f.dat.WriteAt(data[written:written+available], int64(sector)*sectorSize+sectorHdrSize); err != nil {
			return false
		}

		written += available
		sector = nextSector
	}

	return true
}

// Has reports whether archive/file exists and has a valid idx entry.
//
// TS FileStream.ts:198-225 (has)
func (f *FileStream) Has(archive, file int) bool {
	// TS: if (!this.dat) { return false; }
	if f.dat == nil {
		return false
	}

	// TS: if (archive < 0 || archive >= this.idx.length || !this.idx[archive]) { return false; }
	if archive < 0 || archive >= numIdx || f.idx[archive] == nil {
		return false
	}

	// TS: if (file < 0 || file >= this.count(archive)) { return false; }
	if file < 0 || file >= f.Count(archive) {
		return false
	}

	// TS: if (this.packed[archive][file]) { return true; }
	if _, ok := f.packed[archive][file]; ok {
		return true
	}

	// Read the 6-byte idx entry.
	idxBuf := make([]byte, idxEntrySize)
	if _, err := f.idx[archive].ReadAt(idxBuf, int64(file*idxEntrySize)); err != nil {
		return false
	}
	size := int(idxBuf[0])<<16 | int(idxBuf[1])<<8 | int(idxBuf[2])
	sector := int(idxBuf[3])<<16 | int(idxBuf[4])<<8 | int(idxBuf[5])

	// TS: if (size > 2000000) { return false; }
	if size > maxFileSize {
		return false
	}

	// TS: if (sector <= 0 || sector > this.dat.length / 520) { return false; }
	dLen := f.datLen()
	if sector <= 0 || int64(sector) > dLen/sectorSize {
		return false
	}

	return true
}
