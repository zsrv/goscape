package model

// Section layout (derived from TS Model.ts:49-141).
//
// The blob structure preceding the 18-byte trailer is:
//
//	[vertexFlags   ] vertexCount × 1 byte         @ vertexFlagsOffset  (=0)
//	[faceOrients   ] faceCount × 1 byte            @ faceOrientationsOffset
//	[facePriorities] faceCount × 1 byte if prio=255 @ facePrioritiesOffset
//	[faceLabels    ] faceCount × 1 byte if present  @ faceLabelsOffset
//	[faceInfos     ] faceCount × 1 byte if present  @ faceInfosOffset
//	[vertexLabels  ] vertexCount × 1 byte if present @ vertexLabelsOffset
//	[faceAlphas    ] faceCount × 1 byte if present  @ faceAlphasOffset
//	[faceVertices  ] dataLengthFaceOrientations bytes@ faceVerticesOffset
//	[faceColours   ] faceCount × 2 bytes            @ faceColoursOffset
//	[faceTextureAxis] texturedFaceCount × 6 bytes   @ faceTextureAxisOffset
//	[vertexX       ] dataLengthX bytes               @ vertexXOffset
//	[vertexY       ] dataLengthY bytes               @ vertexYOffset
//	[vertexZ       ] dataLengthZ bytes               @ vertexZOffset
//	--- 18-byte trailer ---
//	 +0  u2 vertexCount
//	 +2  u2 faceCount
//	 +4  u1 texturedFaceCount
//	 +5  u1 hasInfo
//	 +6  u1 priority
//	 +7  u1 hasAlpha
//	 +8  u1 hasFaceLabels
//	 +9  u1 hasVertexLabels
//	+10  u2 dataLengthX
//	+12  u2 dataLengthY
//	+14  u2 dataLengthZ
//	+16  u2 dataLengthFaceOrientations

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zsrv/goscape/pkg/io/filestream"
)

// ---- helpers ----------------------------------------------------------------

// u2be appends a big-endian uint16 to buf.
func u2be(buf []byte, v int) []byte {
	return append(buf, byte(v>>8), byte(v))
}

// u1 appends one byte.
func u1(buf []byte, v int) []byte {
	return append(buf, byte(v))
}

// u1b appends one byte (byte-typed variant, used with smartByte).
func u1b(buf []byte, v byte) []byte {
	return append(buf, v)
}

// smartByte encodes a single gsmart value that fits in 1 byte (range -64..63).
// TS Packet.gsmart single-byte form: encode as (v + 64).
func smartByte(v int) byte {
	return byte(v + 64)
}

// smartU2 encodes a gsmart value that requires 2 bytes.
// TS Packet.gsmart two-byte form: first byte >=128, encode as (v + 49152).
func smartU2(buf []byte, v int) []byte {
	enc := v + 49152
	return append(buf, byte(enc>>8), byte(enc))
}

// buildTrailer appends the 18-byte trailer.
func buildTrailer(buf []byte,
	vertexCount, faceCount, texturedFaceCount int,
	hasInfo, priority, hasAlpha, hasFaceLabels, hasVertexLabels int,
	dataLengthX, dataLengthY, dataLengthZ, dataLengthFaceOrientations int,
) []byte {
	buf = u2be(buf, vertexCount)
	buf = u2be(buf, faceCount)
	buf = u1(buf, texturedFaceCount)
	buf = u1(buf, hasInfo)
	buf = u1(buf, priority)
	buf = u1(buf, hasAlpha)
	buf = u1(buf, hasFaceLabels)
	buf = u1(buf, hasVertexLabels)
	buf = u2be(buf, dataLengthX)
	buf = u2be(buf, dataLengthY)
	buf = u2be(buf, dataLengthZ)
	buf = u2be(buf, dataLengthFaceOrientations)
	return buf
}

// buildSyntheticModel builds a minimal synthetic 2-vertex, 2-face model blob.
//
// Parameters chosen so ALL optional sections are absent:
//   - priority = 7 (not 255) → no per-face priority section; global priority = 7
//   - hasInfo = 0, hasAlpha = 0, hasFaceLabels = 0, hasVertexLabels = 0
//   - texturedFaceCount = 0
//
// Vertex deltas (gsmart 1-byte, value=0 → encoded byte = 64):
//
//	vertex 0: flags=0x7 (all axes), dx=0, dy=0, dz=0
//	vertex 1: flags=0x7, dx=1, dy=2, dz=3
//
// Face orientations: face0=orientation1 (new triangle), face1=orientation2 (b=c, new c)
//
// faceColour: face0=0x1234, face1=0x5678
//
// With hasInfo=0, faceInfo section is absent → FaceInfo will be nil.
func buildSyntheticModel() []byte {
	var buf []byte

	// --- vertexFlags section (2 vertices × 1 byte) ---
	buf = u1(buf, 0x7) // vertex 0: flags = X|Y|Z
	buf = u1(buf, 0x7) // vertex 1: flags = X|Y|Z

	// --- faceOrientations section (2 faces × 1 byte) ---
	buf = u1(buf, 1) // face0: orientation=1 (new triangle, reads 3 deltas)
	buf = u1(buf, 2) // face1: orientation=2 (b=c, reads 1 delta)

	// no facePriorities (priority != 255)
	// no faceLabels (hasFaceLabels=0)
	// no faceInfos (hasInfo=0)
	// no vertexLabels (hasVertexLabels=0)
	// no faceAlphas (hasAlpha=0)

	// --- faceVertices section ---
	// face0 orientation=1: 3 gsmart values: a=0, b=0, c=1 (last=1)
	//   delta1 = 0-last_before = 0-0 = 0 → a=0+0=0; gsmart(0)=64
	//   delta2 = b-a = 0-0 = 0 → b=0+0=0; gsmart(0)=64
	//   delta3 = c-b = 1-0 = 1 → c=1+0=1? No: delta = c - last; last=0 before, so delta3=1; c = delta3+last=1+0? wait
	// TS: a = d1+last; b = d2+a; c = d3+b; last=c
	//   d1=0 → a=0+0=0; d2=0 → b=0+0=0; d3=1 → c=1+0=1; last=1
	// face1 orientation=2: b=c=1, delta → c=delta+last(=1); delta=0 → c=0+1=1
	// So: faceVertexA[0]=0, faceVertexB[0]=0, faceVertexC[0]=1
	//     faceVertexA[1]=0, faceVertexB[1]=1, faceVertexC[1]=1
	faceVertsStart := len(buf)
	buf = u1b(buf, smartByte(0)) // d1=0
	buf = u1b(buf, smartByte(0)) // d2=0
	buf = u1b(buf, smartByte(1)) // d3=1
	buf = u1b(buf, smartByte(0)) // face1 delta=0
	faceVertsLen := len(buf) - faceVertsStart

	// --- faceColours section (2 faces × 2 bytes) ---
	buf = u2be(buf, 0x1234) // face0 colour
	buf = u2be(buf, 0x5678) // face1 colour

	// no textureAxis (texturedFaceCount=0)

	// --- vertexX section: 2 vertices × gsmart bytes ---
	// vertex 0: dx=0; vertex 1: dx=1 (cumulative, so delta=1)
	vertXStart := len(buf)
	buf = u1b(buf, smartByte(0)) // vertex0 X delta=0
	buf = u1b(buf, smartByte(1)) // vertex1 X delta=1
	vertXLen := len(buf) - vertXStart

	// --- vertexY section ---
	vertYStart := len(buf)
	buf = u1b(buf, smartByte(0)) // vertex0 Y delta=0
	buf = u1b(buf, smartByte(2)) // vertex1 Y delta=2
	vertYLen := len(buf) - vertYStart

	// --- vertexZ section ---
	vertZStart := len(buf)
	buf = u1b(buf, smartByte(0)) // vertex0 Z delta=0
	buf = u1b(buf, smartByte(3)) // vertex1 Z delta=3
	vertZLen := len(buf) - vertZStart

	// --- 18-byte trailer ---
	buf = buildTrailer(buf,
		2, 2, 0, // vertexCount, faceCount, texturedFaceCount
		0, // hasInfo=0
		7, // priority=7 (not 255 → global priority)
		0, // hasAlpha=0
		0, // hasFaceLabels=0
		0, // hasVertexLabels=0
		vertXLen, vertYLen, vertZLen, faceVertsLen,
	)

	return buf
}

// buildTexturedFaceModel builds a model with 1 face that is textured
// (faceInfo bit 0x3 = infoVal) and faceColour = textureID.
// hasInfo=1, priority=7, 1 vertex, 1 face, 1 texturedFace (so texturedFaceCount>0
// satisfies the TS Model.ts:332 guard `!model.texturedFaceCount → false`).
//
// Section order (TS Model.ts:81-140):
//
//	vertexFlags(1) | faceOrientations(1) | faceInfos(1) | faceVertices(3) |
//	faceColours(2) | faceTextureAxis(6) | vertexX(1) | vertexY(1) | vertexZ(1)
func buildTexturedFaceModel(textureID int, infoVal int) []byte {
	var buf []byte

	// vertexFlags section: 1 vertex, flags=0x7
	buf = u1(buf, 0x7)

	// faceOrientations section: 1 face, orientation=1
	buf = u1(buf, 1)

	// no facePriorities (priority=7 != 255)
	// no faceLabels (hasFaceLabels=0)

	// faceInfos section (hasInfo=1): 1 face = infoVal
	buf = u1(buf, infoVal)

	// no vertexLabels (hasVertexLabels=0)
	// no faceAlphas (hasAlpha=0)

	// faceVertices section: orientation=1 reads 3 gsmart; a=0,b=0,c=0
	faceVertsStart := len(buf)
	buf = u1b(buf, smartByte(0))
	buf = u1b(buf, smartByte(0))
	buf = u1b(buf, smartByte(0))
	faceVertsLen := len(buf) - faceVertsStart

	// faceColours section: face0 = textureID
	buf = u2be(buf, textureID)

	// faceTextureAxis section: 1 textured face × 6 bytes (3×u2)
	// values: texturedVertexA=0, texturedVertexB=0, texturedVertexC=0
	buf = u2be(buf, 0)
	buf = u2be(buf, 0)
	buf = u2be(buf, 0)

	// vertexX/Y/Z sections: 1 vertex each (delta=0)
	vertXStart := len(buf)
	buf = u1b(buf, smartByte(0))
	vertXLen := len(buf) - vertXStart

	vertYStart := len(buf)
	buf = u1b(buf, smartByte(0))
	vertYLen := len(buf) - vertYStart

	vertZStart := len(buf)
	buf = u1b(buf, smartByte(0))
	vertZLen := len(buf) - vertZStart

	buf = buildTrailer(buf,
		1, 1, 1, // vertexCount=1, faceCount=1, texturedFaceCount=1
		1, // hasInfo=1
		7, // priority=7
		0, // hasAlpha=0
		0, // hasFaceLabels=0
		0, // hasVertexLabels=0
		vertXLen, vertYLen, vertZLen, faceVertsLen,
	)
	return buf
}

// ---- tests ------------------------------------------------------------------

// TestTrailerParse verifies every Metadata field from the synthetic 2-face blob.
// TS Model.ts:49-141.
func TestTrailerParse(t *testing.T) {
	blob := buildSyntheticModel()
	s := New()
	s.Unpack(0, blob)

	require.Less(t, 0, len(s.meta), "meta slice must be populated")
	info := s.meta[0]
	require.NotNil(t, info)

	assert.Equal(t, 2, info.vertexCount, "vertexCount")
	assert.Equal(t, 2, info.faceCount, "faceCount")
	assert.Equal(t, 0, info.texturedFaceCount, "texturedFaceCount")
	assert.Equal(t, 0, info.vertexFlagsOffset, "vertexFlagsOffset starts at 0")
	assert.Equal(t, 2, info.faceOrientationsOffset, "faceOrientations after 2 vertexFlags bytes")
	// priority=7 (not 255) → facePrioritiesOffset = -7-1 = -8
	assert.Equal(t, -8, info.facePrioritiesOffset, "facePrioritiesOffset (global priority=7 → -priority-1)")
	// hasInfo=0 → faceInfosOffset = -1
	assert.Equal(t, -1, info.faceInfosOffset, "faceInfosOffset (hasInfo=0)")
	// hasAlpha=0 → faceAlphasOffset = -1
	assert.Equal(t, -1, info.faceAlphasOffset, "faceAlphasOffset (hasAlpha=0)")
	// hasFaceLabels=0 → faceLabelsOffset = -1
	assert.Equal(t, -1, info.faceLabelsOffset, "faceLabelsOffset (hasFaceLabels=0)")
	// hasVertexLabels=0 → vertexLabelsOffset = -1
	assert.Equal(t, -1, info.vertexLabelsOffset, "vertexLabelsOffset (hasVertexLabels=0)")
	// faceVerticesOffset = 2(vertexFlags) + 2(faceOrientations) = 4
	assert.Equal(t, 4, info.faceVerticesOffset, "faceVerticesOffset")
	// faceColoursOffset = 4 + faceVertsLen(4) = 8
	assert.Equal(t, 8, info.faceColoursOffset, "faceColoursOffset")
	// faceTextureAxisOffset = 8 + faceCount*2 = 8+4 = 12
	assert.Equal(t, 12, info.faceTextureAxisOffset, "faceTextureAxisOffset")
	// vertexXOffset = 12 + 0 (no textured faces) = 12
	assert.Equal(t, 12, info.vertexXOffset, "vertexXOffset")
	// vertexYOffset = 12 + vertXLen(2) = 14
	assert.Equal(t, 14, info.vertexYOffset, "vertexYOffset")
	// vertexZOffset = 14 + vertYLen(2) = 16
	assert.Equal(t, 16, info.vertexZOffset, "vertexZOffset")
}

// TestTruncatedData verifies that a data slice shorter than 18 bytes produces a zeroed
// metadata entry without panicking.
//
// TS Model.ts:63 sets buf.pos = data.length - 18; for data.length < 18 the Uint8Array
// Packet would receive a negative pos, clamp it to 0 via subarray, and silently read
// garbage metadata. No upstream caller feeds truncated entries; Go stores a zeroed entry
// instead of panicking (cf. the nil-data path).
func TestTruncatedData(t *testing.T) {
	truncated := make([]byte, 17) // one byte short of the 18-byte trailer
	s := New()
	s.Unpack(0, truncated) // must not panic

	require.Len(t, s.meta, 1)
	info := s.meta[0]
	require.NotNil(t, info, "truncated data must produce zeroed entry, not nil")
	assert.Equal(t, 0, info.vertexCount, "vertexCount must be zero")
	assert.Equal(t, 0, info.faceCount, "faceCount must be zero")
	assert.Equal(t, 0, info.texturedFaceCount, "texturedFaceCount must be zero")
	assert.Nil(t, info.data, "data field must be nil for zeroed entry")
}

// TestNilData verifies nil/empty data produces a zeroed metadata entry with no panic.
// TS Model.ts:50-56.
func TestNilData(t *testing.T) {
	s := New()
	s.Unpack(5, nil)

	require.Len(t, s.meta, 6)
	info := s.meta[5]
	require.NotNil(t, info, "nil data must produce zeroed entry, not nil")
	assert.Equal(t, 0, info.vertexCount)
	assert.Equal(t, 0, info.faceCount)
	assert.Equal(t, 0, info.texturedFaceCount)
	assert.Nil(t, info.data)
}

// TestFromIDMissingMeta verifies FromID returns an empty Model when no metadata exists.
// TS Model.ts:144-146.
func TestFromIDMissingMeta(t *testing.T) {
	s := New()
	m := s.FromID(99)
	require.NotNil(t, m, "FromID must not return nil")
	assert.Equal(t, 0, m.FaceCount)
	assert.Nil(t, m.FaceColour)
}

// TestFromIDNilData verifies FromID returns an empty Model when metadata has nil data.
// TS Model.ts:151-153.
func TestFromIDNilData(t *testing.T) {
	s := New()
	s.Unpack(0, nil) // zeroed entry; data=nil
	m := s.FromID(0)
	require.NotNil(t, m)
	assert.Equal(t, 0, m.FaceCount)
	assert.Nil(t, m.FaceColour)
}

// TestFullDecode verifies faceInfo (nil here), faceColour, vertex positions,
// and face vertex indices decoded from the synthetic 2-face blob.
// TS Model.ts:155-326.
func TestFullDecode(t *testing.T) {
	blob := buildSyntheticModel()
	s := New()
	s.Unpack(0, blob)
	m := s.FromID(0)
	require.NotNil(t, m)

	assert.Equal(t, 2, m.VertexCount)
	assert.Equal(t, 2, m.FaceCount)
	assert.Equal(t, 0, m.TexturedFaceCount)

	// hasInfo=0 → faceInfo must be nil
	assert.Nil(t, m.FaceInfo, "faceInfo must be nil when hasInfo=0")

	// faceColour pinned
	require.Len(t, m.FaceColour, 2)
	assert.Equal(t, int32(0x1234), m.FaceColour[0], "faceColour[0]")
	assert.Equal(t, int32(0x5678), m.FaceColour[1], "faceColour[1]")

	// global priority = 7 (priority byte in trailer = 7)
	assert.Nil(t, m.FacePriority, "facePriority must be nil when priority!=255")
	assert.Equal(t, 7, m.Priority, "global priority")

	// vertex positions (cumulative): vertex0=(0,0,0), vertex1=(1,2,3)
	require.Len(t, m.VertexX, 2)
	assert.Equal(t, int32(0), m.VertexX[0])
	assert.Equal(t, int32(1), m.VertexX[1])
	assert.Equal(t, int32(0), m.VertexY[0])
	assert.Equal(t, int32(2), m.VertexY[1])
	assert.Equal(t, int32(0), m.VertexZ[0])
	assert.Equal(t, int32(3), m.VertexZ[1])

	// face vertex indices
	// face0 orientation=1: d1=0,d2=0,d3=1 → a=0, b=0, c=1, last=1
	assert.Equal(t, int32(0), m.FaceVertexA[0])
	assert.Equal(t, int32(0), m.FaceVertexB[0])
	assert.Equal(t, int32(1), m.FaceVertexC[0])
	// face1 orientation=2: b=c=1, delta=0 → c=0+1=1, last=1
	assert.Equal(t, int32(0), m.FaceVertexA[1])
	assert.Equal(t, int32(1), m.FaceVertexB[1])
	assert.Equal(t, int32(1), m.FaceVertexC[1])
}

// TestGsmartTwoByte verifies the 2-byte gsmart path for vertex deltas outside -64..63.
//
// smartU2 encodes (v + 49152) as two bytes with the high byte ≥ 128.  The gsmart
// decoder subtracts 49152 to recover v.  This branch was previously covered only by
// the real-cache smoke test; this synthetic test pins the decoded vertex value for
// deterministic CI coverage.
//
// Vertex layout: 1 vertex, flags=0x7 (all axes), deltas X=100, Y=-100, Z=0.
// Expected positions after decode: VertexX[0]=100, VertexY[0]=-100, VertexZ[0]=0.
func TestGsmartTwoByte(t *testing.T) {
	var buf []byte

	// --- vertexFlags section (1 vertex) ---
	buf = u1(buf, 0x7) // flags: X|Y|Z present

	// --- faceOrientations section (1 face, orientation=1) ---
	buf = u1(buf, 1)

	// no optional sections (priority=7, hasInfo=0, hasAlpha=0, hasFaceLabels=0, hasVertexLabels=0)

	// --- faceVertices section: orientation=1 reads 3 deltas: d1=0, d2=0, d3=0 ---
	// a=0, b=0, c=0
	faceVertsStart := len(buf)
	buf = u1b(buf, smartByte(0))
	buf = u1b(buf, smartByte(0))
	buf = u1b(buf, smartByte(0))
	faceVertsLen := len(buf) - faceVertsStart

	// --- faceColours section: 1 face × 2 bytes ---
	buf = u2be(buf, 0x0001)

	// --- vertexX section: 1 vertex, delta=100 (requires 2-byte gsmart) ---
	vertXStart := len(buf)
	buf = smartU2(buf, 100)
	vertXLen := len(buf) - vertXStart

	// --- vertexY section: 1 vertex, delta=-100 (requires 2-byte gsmart) ---
	vertYStart := len(buf)
	buf = smartU2(buf, -100)
	vertYLen := len(buf) - vertYStart

	// --- vertexZ section: 1 vertex, delta=0 (1-byte gsmart) ---
	vertZStart := len(buf)
	buf = u1b(buf, smartByte(0))
	vertZLen := len(buf) - vertZStart

	// --- 18-byte trailer ---
	buf = buildTrailer(buf,
		1, 1, 0, // vertexCount=1, faceCount=1, texturedFaceCount=0
		0, // hasInfo=0
		7, // priority=7 (not 255)
		0, // hasAlpha=0
		0, // hasFaceLabels=0
		0, // hasVertexLabels=0
		vertXLen, vertYLen, vertZLen, faceVertsLen,
	)

	s := New()
	s.Unpack(0, buf)
	m := s.FromID(0)
	require.NotNil(t, m)

	require.Len(t, m.VertexX, 1)
	assert.Equal(t, int32(100), m.VertexX[0], "2-byte gsmart positive delta X=100")
	assert.Equal(t, int32(-100), m.VertexY[0], "2-byte gsmart negative delta Y=-100")
	assert.Equal(t, int32(0), m.VertexZ[0], "1-byte gsmart delta Z=0")
}

// TestModelsHaveTexture exercises the modelsHaveTexture/modelHasTexture matrix.
// TS Model.ts:330-354.
func TestModelsHaveTexture(t *testing.T) {
	const textureID = 0x00AA

	// model 0: textured face (infoVal=2 → bits 0x3=2 > 1) with matching colour
	// model 1: textured face with non-matching colour
	// model 2: untextured face (infoVal=1 → bits 0x3=1, not >1) with matching colour
	// model 3: no faceInfo (hasInfo=0) with matching colour → no faceInfo nil → false
	s := New()
	s.Unpack(0, buildTexturedFaceModel(textureID, 2)) // textured, matching
	s.Unpack(1, buildTexturedFaceModel(0x00BB, 2))    // textured, non-matching
	s.Unpack(2, buildTexturedFaceModel(textureID, 1)) // infoVal bits=1 (not >1)
	s.Unpack(3, buildSyntheticModel())                // hasInfo=0, faceInfo=nil

	// case 1: textured face matching id → true
	assert.True(t, s.ModelsHaveTexture([]int{0}, textureID), "textured face matching id must return true")

	// case 2: textured face different id → false
	assert.False(t, s.ModelsHaveTexture([]int{1}, textureID), "textured face with different colour must return false")

	// case 3: (faceInfo & 0x3) == 1 (not > 1) → false
	assert.False(t, s.ModelsHaveTexture([]int{2}, textureID), "(faceInfo&0x3)=1 must not count as textured")

	// case 4: faceInfo nil → false (hasInfo=0 model)
	assert.False(t, s.ModelsHaveTexture([]int{3}, textureID), "nil faceInfo must return false")

	// case 5: multiple models, only second matches → true
	assert.True(t, s.ModelsHaveTexture([]int{1, 0}, textureID), "second model matching must return true")

	// case 6: no models match → false
	assert.False(t, s.ModelsHaveTexture([]int{1, 2, 3}, textureID), "no match must return false")
}

// TestAlreadyLoaded verifies that calling Unpack twice for same id is a no-op.
// TS Model.ts:58-60.
func TestAlreadyLoaded(t *testing.T) {
	blob := buildSyntheticModel()
	s := New()
	s.Unpack(0, blob)
	origInfo := s.meta[0]
	s.Unpack(0, blob) // second call must be no-op
	assert.Same(t, origInfo, s.meta[0], "second Unpack must not replace metadata")
}

// TestRealCacheSmoke is an env-gated smoke test that unpacks every model from
// the real Rev-244 cache. Skipped when GOSCAPE_REF244_DIR is unset.
// TS Model.ts:49-327.
func TestRealCacheSmoke(t *testing.T) {
	dir := os.Getenv("GOSCAPE_REF244_DIR")
	if dir == "" {
		t.Skip("GOSCAPE_REF244_DIR not set")
	}

	packDir := dir + "/data/pack"
	fs := filestream.New(packDir, false, true)
	require.NotNil(t, fs, "filestream must open")

	count := fs.Count(1)
	assert.Greater(t, count, 3000, "expected >3000 models in archive 1")

	s := New()
	decoded := 0
	for id := range count {
		data := fs.Read(1, id, true)
		s.Unpack(id, data)
		if len(data) > 0 {
			decoded++
		}
	}
	// Guard against silent degradation: a filestream decompress regression
	// returning nil for every versioned entry used to pass this test (zeroed
	// metadata is panic-free). The 244 reference cache holds ~3.4k models.
	assert.Greater(t, decoded, 3000, "expected >3000 readable models (filestream decompress regression?)")

	// FromID(0) must decode without panic — and actually decode something.
	m := s.FromID(0)
	require.NotNil(t, m)
	assert.Greater(t, m.FaceCount, 0, "model 0 must decode real geometry")
}
