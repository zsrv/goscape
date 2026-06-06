// Package model decodes RS2 model cache entries (archive 1).
//
// This is a faithful 1:1 Go port of the TypeScript class at:
// Server244-ref/engine/src/cache/graphics/Model.ts
//
// Section layout (derived from TS Model.ts:49-141):
//
//	[vertexFlags   ] vertexCount bytes           @ vertexFlagsOffset
//	[faceOrients   ] faceCount bytes             @ faceOrientationsOffset
//	[facePriorities] faceCount bytes if per-face @ facePrioritiesOffset (skipped when priority!=255)
//	[faceLabels    ] faceCount bytes if present  @ faceLabelsOffset
//	[faceInfos     ] faceCount bytes if present  @ faceInfosOffset
//	[vertexLabels  ] vertexCount bytes if present@ vertexLabelsOffset
//	[faceAlphas    ] faceCount bytes if present  @ faceAlphasOffset
//	[faceVertices  ] dataLengthFaceOrientations  @ faceVerticesOffset
//	[faceColours   ] faceCount*2 bytes           @ faceColoursOffset
//	[faceTextureAxis] texturedFaceCount*6 bytes  @ faceTextureAxisOffset
//	[vertexX       ] dataLengthX bytes           @ vertexXOffset
//	[vertexY       ] dataLengthY bytes           @ vertexYOffset
//	[vertexZ       ] dataLengthZ bytes           @ vertexZOffset
//	--- 18-byte trailer ---
//	u2 vertexCount | u2 faceCount | u1 texturedFaceCount
//	u1 hasInfo | u1 priority | u1 hasAlpha | u1 hasFaceLabels | u1 hasVertexLabels
//	u2 dataLengthX | u2 dataLengthY | u2 dataLengthZ | u2 dataLengthFaceOrientations
package model

// metadata holds the parsed trailer and pre-computed section offsets for one model.
// A nil data field means the original cache entry was empty/nil.
// TS Model.ts:3-21 (Metadata class).
type metadata struct {
	data                []byte
	vertexCount         int
	faceCount           int
	texturedFaceCount   int
	vertexFlagsOffset   int
	vertexXOffset       int
	vertexYOffset       int
	vertexZOffset       int
	vertexLabelsOffset  int
	faceVerticesOffset  int
	faceOrientationsOffset int
	faceColoursOffset   int
	faceInfosOffset     int
	facePrioritiesOffset int
	faceAlphasOffset    int
	faceLabelsOffset    int
	faceTextureAxisOffset int
}

// Model is a decoded RS2 model.  Fields mirror the TS Model instance fields.
// TS Model.ts:23-47.
type Model struct {
	VertexCount       int
	FaceCount         int
	TexturedFaceCount int

	VertexX []int32
	VertexY []int32
	VertexZ []int32

	FaceVertexA []int32
	FaceVertexB []int32
	FaceVertexC []int32

	TexturedVertexA []int32
	TexturedVertexB []int32
	TexturedVertexC []int32

	FaceInfo     []int32 // nil when hasInfo==0
	FacePriority []int32 // nil when priority!=255; model-wide priority in Priority field
	Priority     int     // used when FacePriority==nil; TS Model.ts:180
	FaceAlpha    []int32 // nil when hasAlpha==0
	FaceColour   []int32 // always populated when data present
	FaceColourA  []int32 // unused by unpack (future extension)
	FaceColourB  []int32
	FaceColourC  []int32

	VertexLabel []int32 // nil when hasVertexLabels==0
	FaceLabel   []int32 // nil when hasFaceLabels==0
}

// Store holds the metadata table and the decoded-model cache.
// Equivalent to the static fields on the TS Model class.
type Store struct {
	loaded int
	meta   []*metadata
	cache  map[int]*Model
}

// New returns a new empty Store.
func New() *Store {
	return &Store{
		cache: make(map[int]*Model),
	}
}

// Unpack parses the 18-byte trailer from data and stores metadata for id.
// nil/empty data produces a zeroed metadata entry (mirrors TS Model.ts:50-56).
// If metadata for id is already present the call is a no-op (TS Model.ts:58-60).
//
// TS Model.ts:49-141
func (s *Store) Unpack(id int, data []byte) {
	// Grow meta slice if needed.
	for len(s.meta) <= id {
		s.meta = append(s.meta, nil)
	}

	// nil/empty data → zeroed entry. TS Model.ts:50-56.
	if len(data) == 0 {
		info := &metadata{} // vertexCount/faceCount/texturedFaceCount all 0; offsets all 0/-1
		info.facePrioritiesOffset = 0 // TS default is 0, not -1
		s.meta[id] = info
		return
	}

	// Already loaded: skip. TS Model.ts:58-60.
	if s.meta[id] != nil {
		return
	}

	// Guard against truncated data. TS Model.ts:63 sets buf.pos = data.length - 18;
	// a Uint8Array with data.length < 18 would yield a negative Packet.pos and read
	// garbage metadata rather than throwing (Uint8Array.subarray clamps negative
	// starts to 0). No upstream caller feeds truncated entries, so Go treats this
	// as empty/nil and stores a zeroed entry instead of panicking.
	if len(data) < 18 {
		info := &metadata{}
		info.facePrioritiesOffset = 0 // TS default
		s.meta[id] = info
		return
	}

	// Parse 18-byte trailer at data.length - 18. TS Model.ts:63-79.
	pos := len(data) - 18

	vertexCount := int(uint16(data[pos])<<8 | uint16(data[pos+1]))
	faceCount := int(uint16(data[pos+2])<<8 | uint16(data[pos+3]))
	texturedFaceCount := int(data[pos+4])

	hasInfo := int(data[pos+5])
	priority := int(data[pos+6])
	hasAlpha := int(data[pos+7])
	hasFaceLabels := int(data[pos+8])
	hasVertexLabels := int(data[pos+9])

	dataLengthX := int(uint16(data[pos+10])<<8 | uint16(data[pos+11]))
	dataLengthY := int(uint16(data[pos+12])<<8 | uint16(data[pos+13]))
	dataLengthZ := int(uint16(data[pos+14])<<8 | uint16(data[pos+15]))
	dataLengthFaceOrientations := int(uint16(data[pos+16])<<8 | uint16(data[pos+17]))

	info := &metadata{
		data:              data,
		vertexCount:       vertexCount,
		faceCount:         faceCount,
		texturedFaceCount: texturedFaceCount,
	}

	// Compute absolute section offsets from pos=0. TS Model.ts:81-140.
	off := 0

	info.vertexFlagsOffset = off // TS Model.ts:82-83
	off += vertexCount

	info.faceOrientationsOffset = off // TS Model.ts:85-86
	off += faceCount

	// facePrioritiesOffset. TS Model.ts:88-94.
	info.facePrioritiesOffset = off
	if priority == 255 {
		off += faceCount
	} else {
		info.facePrioritiesOffset = -priority - 1
	}

	// faceLabelsOffset. TS Model.ts:96-101.
	info.faceLabelsOffset = off
	if hasFaceLabels == 1 {
		off += faceCount
	} else {
		info.faceLabelsOffset = -1
	}

	// faceInfosOffset. TS Model.ts:103-108.
	info.faceInfosOffset = off
	if hasInfo == 1 {
		off += faceCount
	} else {
		info.faceInfosOffset = -1
	}

	// vertexLabelsOffset. TS Model.ts:110-115.
	info.vertexLabelsOffset = off
	if hasVertexLabels == 1 {
		off += vertexCount
	} else {
		info.vertexLabelsOffset = -1
	}

	// faceAlphasOffset. TS Model.ts:117-122.
	info.faceAlphasOffset = off
	if hasAlpha == 1 {
		off += faceCount
	} else {
		info.faceAlphasOffset = -1
	}

	// faceVerticesOffset. TS Model.ts:124-125.
	info.faceVerticesOffset = off
	off += dataLengthFaceOrientations

	// faceColoursOffset. TS Model.ts:127-128.
	info.faceColoursOffset = off
	off += faceCount * 2

	// faceTextureAxisOffset. TS Model.ts:130-131.
	info.faceTextureAxisOffset = off
	off += texturedFaceCount * 6

	// vertexX/Y/ZOffset. TS Model.ts:133-140.
	info.vertexXOffset = off
	off += dataLengthX

	info.vertexYOffset = off
	off += dataLengthY

	info.vertexZOffset = off
	off += dataLengthZ // end of data body; off not used further but mirrors TS Model.ts:139-140
	_ = off

	s.meta[id] = info
}

// FromID decodes and returns the Model for id, or an empty Model when
// metadata is missing or data is nil.  Results are cached after first decode.
//
// TS Model.ts:143-327
func (s *Store) FromID(id int) *Model {
	// Missing metadata → empty model. TS Model.ts:144-146.
	if id >= len(s.meta) || s.meta[id] == nil {
		return &Model{}
	}

	// TS Model.loaded++. TS Model.ts:148 increments unconditionally per fromId call,
	// including cache hits. With the Go-side decode cache this no longer equals
	// unique decodes, but it faithfully counts calls (matching TS call-count semantics).
	s.loaded++

	info := s.meta[id]

	// No data (nil-data entry) → empty model. TS Model.ts:151-153.
	if info.data == nil {
		return &Model{}
	}

	// Return cached decode. (TS re-decodes each call; we cache for efficiency
	// since the result is immutable.)
	if m, ok := s.cache[id]; ok {
		return m
	}

	model := s.decode(info)
	s.cache[id] = model
	return model
}

// decode performs the full model decode from info.data.
// TS Model.ts:155-326.
func (s *Store) decode(info *metadata) *Model {
	data := info.data

	model := &Model{
		VertexCount:       info.vertexCount,
		FaceCount:         info.faceCount,
		TexturedFaceCount: info.texturedFaceCount,
	}

	// Allocate vertex arrays. TS Model.ts:159-167.
	model.VertexX = make([]int32, model.VertexCount)
	model.VertexY = make([]int32, model.VertexCount)
	model.VertexZ = make([]int32, model.VertexCount)
	model.FaceVertexA = make([]int32, model.FaceCount)
	model.FaceVertexB = make([]int32, model.FaceCount)
	model.FaceVertexC = make([]int32, model.FaceCount)
	model.TexturedVertexA = make([]int32, model.TexturedFaceCount)
	model.TexturedVertexB = make([]int32, model.TexturedFaceCount)
	model.TexturedVertexC = make([]int32, model.TexturedFaceCount)

	// Conditional allocations. TS Model.ts:169-191.
	if info.vertexLabelsOffset >= 0 {
		model.VertexLabel = make([]int32, model.VertexCount)
	}
	if info.faceInfosOffset >= 0 {
		model.FaceInfo = make([]int32, model.FaceCount)
	}
	if info.facePrioritiesOffset >= 0 {
		model.FacePriority = make([]int32, model.FaceCount)
	} else {
		model.Priority = -info.facePrioritiesOffset - 1 // TS Model.ts:180
	}
	if info.faceAlphasOffset >= 0 {
		model.FaceAlpha = make([]int32, model.FaceCount)
	}
	if info.faceLabelsOffset >= 0 {
		model.FaceLabel = make([]int32, model.FaceCount)
	}
	model.FaceColour = make([]int32, model.FaceCount)

	// ---- Vertex decode ---- TS Model.ts:193-238
	// Five cursors (point1..point5) with independent positions into data.
	p1 := info.vertexFlagsOffset   // vertex flags cursor
	p2 := info.vertexXOffset       // vertex X delta cursor
	p3 := info.vertexYOffset       // vertex Y delta cursor
	p4 := info.vertexZOffset       // vertex Z delta cursor
	p5 := info.vertexLabelsOffset  // vertex labels cursor (may be -1 / ignored)

	var dx, dy, dz int32
	for v := 0; v < model.VertexCount; v++ {
		flags := int(data[p1])
		p1++

		var a, b, c int32
		if (flags & 0x1) != 0 {
			a, p2 = gsmart(data, p2) // TS point2.gsmart()
		}
		if (flags & 0x2) != 0 {
			b, p3 = gsmart(data, p3) // TS point3.gsmart()
		}
		if (flags & 0x4) != 0 {
			c, p4 = gsmart(data, p4) // TS point4.gsmart()
		}

		model.VertexX[v] = dx + a
		model.VertexY[v] = dy + b
		model.VertexZ[v] = dz + c
		dx = model.VertexX[v]
		dy = model.VertexY[v]
		dz = model.VertexZ[v]

		if model.VertexLabel != nil {
			model.VertexLabel[v] = int32(data[p5])
			p5++
		}
	}

	// ---- Face attribute decode ---- TS Model.ts:241-273
	f1 := info.faceColoursOffset   // faceColour cursor  (face1)
	f2 := info.faceInfosOffset     // faceInfo cursor    (face2); may be -1
	f3 := info.facePrioritiesOffset // facePriority cursor (face3); may be -1
	f4 := info.faceAlphasOffset    // faceAlpha cursor   (face4); may be -1
	f5 := info.faceLabelsOffset    // faceLabel cursor   (face5); may be -1

	for f := 0; f < model.FaceCount; f++ {
		model.FaceColour[f] = int32(uint16(data[f1])<<8 | uint16(data[f1+1]))
		f1 += 2

		if model.FaceInfo != nil {
			model.FaceInfo[f] = int32(data[f2])
			f2++
		}
		if model.FacePriority != nil {
			model.FacePriority[f] = int32(data[f3])
			f3++
		}
		if model.FaceAlpha != nil {
			model.FaceAlpha[f] = int32(data[f4])
			f4++
		}
		if model.FaceLabel != nil {
			model.FaceLabel[f] = int32(data[f5])
			f5++
		}
	}

	// ---- Face vertex index decode ---- TS Model.ts:276-314
	v1 := info.faceVerticesOffset    // face vertex deltas (vertex1)
	v2 := info.faceOrientationsOffset // face orientations (vertex2)

	var fa, fb, fc, last int32

	for f := 0; f < model.FaceCount; f++ {
		orientation := int(data[v2])
		v2++

		var delta int32
		switch orientation {
		case 1: // TS Model.ts:289-293
			var d1, d2, d3 int32
			d1, v1 = gsmart(data, v1)
			fa = d1 + last
			d2, v1 = gsmart(data, v1)
			fb = d2 + fa
			d3, v1 = gsmart(data, v1)
			fc = d3 + fb
			last = fc
		case 2: // TS Model.ts:294-298: a=a, b=c, c=new
			delta, v1 = gsmart(data, v1)
			fb = fc
			fc = delta + last
			last = fc
		case 3: // TS Model.ts:299-303: a=c, b=b, c=new
			delta, v1 = gsmart(data, v1)
			fa = fc
			fc = delta + last
			last = fc
		case 4: // TS Model.ts:304-310: swap a<->b, c=new
			delta, v1 = gsmart(data, v1)
			fa, fb = fb, fa
			fc = delta + last
			last = fc
		}

		model.FaceVertexA[f] = fa
		model.FaceVertexB[f] = fb
		model.FaceVertexC[f] = fc
	}

	// ---- Texture axis decode ---- TS Model.ts:317-323
	ax := info.faceTextureAxisOffset
	for f := 0; f < model.TexturedFaceCount; f++ {
		model.TexturedVertexA[f] = int32(uint16(data[ax])<<8 | uint16(data[ax+1]))
		ax += 2
		model.TexturedVertexB[f] = int32(uint16(data[ax])<<8 | uint16(data[ax+1]))
		ax += 2
		model.TexturedVertexC[f] = int32(uint16(data[ax])<<8 | uint16(data[ax+1]))
		ax += 2
	}

	return model
}

// gsmart reads a signed smart value at pos in data and returns (value, newPos).
// Matches TS Packet.gsmart(): if first byte >= 128, read 2 bytes and subtract
// 49152; otherwise read 1 byte and subtract 64.
// TS Packet.ts gsmart / pkg/io/packet Packet.GSmart.
func gsmart(data []byte, pos int) (int32, int) {
	if data[pos] >= 128 {
		v := int32(uint16(data[pos])<<8|uint16(data[pos+1])) - 49152
		return v, pos + 2
	}
	v := int32(data[pos]) - 64
	return v, pos + 1
}

// modelHasTexture returns true iff model has a textured face matching textureId.
// A face is textured when (faceInfo[i] & 0x3) > 1 and faceColour[i] == textureId.
// nil faceInfo or faceColour, or zero texturedFaceCount, ⇒ false.
// TS Model.ts:330-343 (modelHasTexture).
func (s *Store) modelHasTexture(modelID int, textureID int) bool {
	model := s.FromID(modelID)
	// TS Model.ts:332 checks `!model` because TS fromId can return null;
	// Go FromID always returns a non-nil *Model, so the nil clause is dropped.
	if model.FaceColour == nil || model.TexturedFaceCount == 0 {
		return false
	}
	for i := 0; i < model.FaceCount; i++ {
		if model.FaceInfo != nil && (model.FaceInfo[i]&0x3) > 1 && model.FaceColour[i] == int32(textureID) {
			return true
		}
	}
	return false
}

// ModelsHaveTexture returns true iff any model in modelIDs has a face with
// the given textureID.
// TS Model.ts:346-354 (modelsHaveTexture).
func (s *Store) ModelsHaveTexture(modelIDs []int, textureID int) bool {
	for _, id := range modelIDs {
		if s.modelHasTexture(id, textureID) {
			return true
		}
	}
	return false
}
