// Package graphics ports tools/pack/graphics/pack.ts (338 LOC).
//
// Builds 21 named bytestreams from <srcDir>/models/*.{ob2,frame,base},
// gated by reg.Anim/Base/Model registries + order files in
// <srcDir>/pack/{anim,base,model}.order. Output: Jagfile at
// <outDir>/client/models.
package graphics

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Pack ports tools/pack/graphics/pack.ts:packClientGraphics.
//
// NAI-192-D-NO-SRC-NO-OP mirror: missing models src dir → no-op.
func Pack(reg *pack.Registry, srcDir, outDir string) error {
	modelsSrc := filepath.Join(srcDir, "models")
	clientOut := filepath.Join(outDir, "client", "models")

	if _, err := os.Stat(modelsSrc); os.IsNotExist(err) {
		return nil
	}

	if !pack.ShouldBuildFileAny(modelsSrc, clientOut) {
		return nil
	}

	modelPack, err := reg.EnsureModel()
	if err != nil {
		return err
	}
	animPack, err := reg.EnsureAnim()
	if err != nil {
		return err
	}
	basePack, err := reg.EnsureBase()
	if err != nil {
		return err
	}

	modelOrder := pack.LoadOrder(filepath.Join(srcDir, "pack", "model.order"))
	animOrder := pack.LoadOrder(filepath.Join(srcDir, "pack", "anim.order"))
	baseOrder := pack.LoadOrder(filepath.Join(srcDir, "pack", "base.order"))

	files := pack.ListFiles(modelsSrc)

	// Build per-stream packets.
	baseHead, baseType, baseLabel, err := packBaseStreams(basePack, baseOrder, files)
	if err != nil {
		return err
	}
	frameHead, frameTran1, frameTran2, frameDel, err := packAnimStreams(animPack, animOrder, files)
	if err != nil {
		return err
	}
	obHead, obFace1, obFace2, obFace3, obFace4, obFace5,
		obPoint1, obPoint2, obPoint3, obPoint4, obPoint5,
		obVertex1, obVertex2, obAxis, err := packModelStreams(modelPack, modelOrder, files)
	if err != nil {
		return err
	}

	jag := jagfile.NewEmptyJagfile(false)
	// TS line 293-313 ordering:
	jag.Write("base_label.dat", baseLabel)
	jag.Write("ob_point1.dat", obPoint1)
	jag.Write("ob_point2.dat", obPoint2)
	jag.Write("ob_point3.dat", obPoint3)
	jag.Write("ob_point4.dat", obPoint4)
	jag.Write("ob_point5.dat", obPoint5)
	jag.Write("ob_head.dat", obHead)
	jag.Write("base_head.dat", baseHead)
	jag.Write("frame_head.dat", frameHead)
	jag.Write("frame_tran1.dat", frameTran1)
	jag.Write("frame_tran2.dat", frameTran2)
	jag.Write("ob_vertex1.dat", obVertex1)
	jag.Write("ob_vertex2.dat", obVertex2)
	jag.Write("frame_del.dat", frameDel)
	jag.Write("base_type.dat", baseType)
	jag.Write("ob_face1.dat", obFace1)
	jag.Write("ob_face2.dat", obFace2)
	jag.Write("ob_face3.dat", obFace3)
	jag.Write("ob_face4.dat", obFace4)
	jag.Write("ob_face5.dat", obFace5)
	jag.Write("ob_axis.dat", obAxis)

	if err := os.MkdirAll(filepath.Dir(clientOut), 0o755); err != nil {
		return err
	}
	return jag.Save(clientOut)
}

// findFile is the goscape equivalent of TS `files.find(f => path.basename(f) === target)`.
func findFile(files []string, target string) string {
	for _, f := range files {
		if filepath.Base(f) == target {
			return f
		}
	}
	return ""
}

// loadPacket reads path into a fresh Packet (Pos=0).
func loadPacket(path string) (*packet.Packet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p := packet.Alloc(len(data) + 8)
	p.PData(data)
	p.Pos = 0
	return p, nil
}

// packBaseStreams ports TS lines 38-86.
func packBaseStreams(basePack *pack.PackFile, order []int, files []string) (head, typ, label *packet.Packet, err error) {
	head = packet.Alloc(5)
	typ = packet.Alloc(5)
	label = packet.Alloc(5)

	head.P2(uint16(len(order)))
	highest := 0
	for _, id := range order {
		if id > highest {
			highest = id
		}
	}
	head.P2(uint16(highest))

	for _, id := range order {
		name := basePack.GetByID(id)
		file := findFile(files, name+".base")
		if file == "" {
			fmt.Fprintf(os.Stderr, "missing base file %d %s\n", id, name)
			continue
		}
		data, err := loadPacket(file)
		if err != nil {
			return nil, nil, nil, err
		}

		data.Pos = data.Length() - 4
		typeLength := int(data.G2())
		labelLength := int(data.G2())

		head.P2(uint16(id))
		head.P1(uint8(typeLength))

		data.Pos = 0

		pType := make([]byte, typeLength)
		data.GData(pType, typeLength)
		typ.PData(pType)

		pLabel := make([]byte, labelLength)
		data.GData(pLabel, labelLength)
		label.PData(pLabel)
	}
	return head, typ, label, nil
}

// packAnimStreams ports TS lines 90-148.
func packAnimStreams(animPack *pack.PackFile, order []int, files []string) (head, tran1, tran2, del *packet.Packet, err error) {
	head = packet.Alloc(5)
	tran1 = packet.Alloc(5)
	tran2 = packet.Alloc(5)
	del = packet.Alloc(5)

	head.P2(uint16(len(order)))
	highest := 0
	for _, id := range order {
		if id > highest {
			highest = id
		}
	}
	head.P2(uint16(highest))

	for _, id := range order {
		name := animPack.GetByID(id)
		file := findFile(files, name+".frame")
		if file == "" {
			fmt.Fprintf(os.Stderr, "missing frame file %d %s\n", id, name)
			continue
		}
		data, err := loadPacket(file)
		if err != nil {
			return nil, nil, nil, nil, err
		}

		data.Pos = data.Length() - 8
		headLen := int(data.G2())
		tran1Len := int(data.G2())
		tran2Len := int(data.G2())
		delLen := int(data.G2())

		data.Pos = 0

		pHead := make([]byte, headLen)
		data.GData(pHead, headLen)

		pTran1 := make([]byte, tran1Len)
		data.GData(pTran1, tran1Len)

		pTran2 := make([]byte, tran2Len)
		data.GData(pTran2, tran2Len)

		pDel := make([]byte, delLen)
		data.GData(pDel, delLen)

		head.PData(pHead)
		tran1.PData(pTran1)
		tran2.PData(pTran2)
		del.PData(pDel)
	}
	return head, tran1, tran2, del, nil
}

// packModelStreams ports TS lines 152-287.
func packModelStreams(modelPack *pack.PackFile, order []int, files []string) (
	head, face1, face2, face3, face4, face5,
	point1, point2, point3, point4, point5,
	vertex1, vertex2, axis *packet.Packet, err error,
) {
	head = packet.Alloc(5)
	face1 = packet.Alloc(5)
	face2 = packet.Alloc(5)
	face3 = packet.Alloc(5)
	face4 = packet.Alloc(5)
	face5 = packet.Alloc(5)
	point1 = packet.Alloc(5)
	point2 = packet.Alloc(5)
	point3 = packet.Alloc(5)
	point4 = packet.Alloc(5)
	point5 = packet.Alloc(5)
	vertex1 = packet.Alloc(5)
	vertex2 = packet.Alloc(5)
	axis = packet.Alloc(5)

	head.P2(uint16(len(order)))

	for _, id := range order {
		name := modelPack.GetByID(id)
		file := findFile(files, name+".ob2")
		if file == "" {
			fmt.Fprintf(os.Stderr, "missing ob2 file %d %s\n", id, name)
			continue
		}
		data, err := loadPacket(file)
		if err != nil {
			return nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, err
		}

		data.Pos = data.Length() - 18
		vertexCount := int(data.G2())
		faceCount := int(data.G2())
		texturedFaceCount := int(data.G1())
		hasInfo := int(data.G1())
		hasPriorities := int(data.G1())
		hasAlpha := int(data.G1())
		hasFaceLabels := int(data.G1())
		hasVertexLabels := int(data.G1())
		vertexXLength := int(data.G2())
		vertexYLength := int(data.G2())
		vertexZLength := int(data.G2())
		faceVertexLength := int(data.G2())

		head.P2(uint16(id))
		head.P2(uint16(vertexCount))
		head.P2(uint16(faceCount))
		head.P1(uint8(texturedFaceCount))
		head.P1(uint8(hasInfo))
		head.P1(uint8(hasPriorities))
		head.P1(uint8(hasAlpha))
		head.P1(uint8(hasFaceLabels))
		head.P1(uint8(hasVertexLabels))

		data.Pos = 0

		readSegment := func(out *packet.Packet, n int) {
			buf := make([]byte, n)
			data.GData(buf, n)
			out.PData(buf)
		}

		readSegment(point1, vertexCount)
		readSegment(vertex2, faceCount)
		if hasPriorities == 255 {
			readSegment(face3, faceCount)
		}
		if hasFaceLabels == 1 {
			readSegment(face5, faceCount)
		}
		if hasInfo == 1 {
			readSegment(face2, faceCount)
		}
		if hasVertexLabels == 1 {
			readSegment(point5, vertexCount)
		}
		if hasAlpha == 1 {
			readSegment(face4, faceCount)
		}
		readSegment(vertex1, faceVertexLength)
		readSegment(face1, faceCount*2)
		readSegment(axis, texturedFaceCount*6)
		readSegment(point2, vertexXLength)
		readSegment(point3, vertexYLength)
		readSegment(point4, vertexZLength)
	}
	return head, face1, face2, face3, face4, face5,
		point1, point2, point3, point4, point5,
		vertex1, vertex2, axis, nil
}
