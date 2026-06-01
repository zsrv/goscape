package world

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/cache"
	"github.com/zsrv/goscape/pkg/io/packet"
	gameserver "github.com/zsrv/goscape/pkg/io/protocol/game/server"
)

// rebuildGetMapsChunkSize is the max payload bytes per DATA_LAND/DATA_LOC
// packet. 1000 - opcode(1) - length prefix(2) - mapX(1) - mapZ(1) - off(2)
// - totalLen(2) = 991.
const rebuildGetMapsChunkSize = 991

const (
	rebuildGetMapsLastBuildTicks = 10 // request is stale after 10 ticks
	rebuildGetMapsMapsLimit      = 18 // 9 mapsquares x 2 file types
)

// sendDataLand writes one chunk of land data for (mapX, mapZ).
// Wire: p1(mapX) p1(mapZ) p2(off) p2(totalLen) pdata(chunk).
func sendDataLand(p *Player, mapX, mapZ, off, total int, chunk []byte) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	buf.P2(uint16(off))
	buf.P2(uint16(total))
	buf.PData(chunk)
	p.writeOut(gameserver.OpDataLand, buf.Bytes())
}

// sendDataLoc writes one chunk of loc data. Same wire format as sendDataLand.
func sendDataLoc(p *Player, mapX, mapZ, off, total int, chunk []byte) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	buf.P2(uint16(off))
	buf.P2(uint16(total))
	buf.PData(chunk)
	p.writeOut(gameserver.OpDataLoc, buf.Bytes())
}

// sendDataLandDone signals end-of-stream for one mapsquare's land file.
// Wire: p1(mapX) p1(mapZ).
func sendDataLandDone(p *Player, mapX, mapZ int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	p.writeOut(gameserver.OpDataLandDone, buf.Bytes())
}

// sendDataLocDone signals end-of-stream for one mapsquare's loc file.
func sendDataLocDone(p *Player, mapX, mapZ int) {
	buf := packet.NewPacket(nil)
	buf.P1(uint8(mapX))
	buf.P1(uint8(mapZ))
	p.writeOut(gameserver.OpDataLocDone, buf.Bytes())
}

// streamLand chunks the client-pack land file for (mapX, mapZ) into
// DATA_LAND packets followed by exactly one DATA_LAND_DONE. Silent
// no-op if the mapsquare isn't in cache.Preload().Data.
//
// Reads from cache.Preload().Data (data/pack/client/maps/) — NOT
// gamemap.LandBytes (data/pack/server/maps/). Per TS
// RebuildGetMapsHandler.ts:44 (PRELOADED.get('m${x}_${z}')). The two
// directories hold byte-different files for the same filename: server
// maps are uncompressed for collision parsing; client maps are
// pre-compressed/encrypted for over-the-wire delivery and match the
// CRCs advertised in RebuildNormal.
func streamLand(p *Player, mapX, mapZ int) {
	data := cache.Preload().Data[fmt.Sprintf("m%d_%d", mapX, mapZ)]
	if data == nil {
		return
	}
	total := len(data)
	for off := 0; off < total; off += rebuildGetMapsChunkSize {
		end := off + rebuildGetMapsChunkSize
		if end > total {
			end = total
		}
		sendDataLand(p, mapX, mapZ, off, total, data[off:end])
	}
	sendDataLandDone(p, mapX, mapZ)
}

// streamLoc is the symmetric helper for DATA_LOC. Same client-pack
// source, same per-chunk + done-packet structure. Per TS
// RebuildGetMapsHandler.ts:54.
func streamLoc(p *Player, mapX, mapZ int) {
	data := cache.Preload().Data[fmt.Sprintf("l%d_%d", mapX, mapZ)]
	if data == nil {
		return
	}
	total := len(data)
	for off := 0; off < total; off += rebuildGetMapsChunkSize {
		end := off + rebuildGetMapsChunkSize
		if end > total {
			end = total
		}
		sendDataLoc(p, mapX, mapZ, off, total, data[off:end])
	}
	sendDataLocDone(p, mapX, mapZ)
}

// handleRebuildGetMaps services the client's request (opcode 150) for a
// batch of m/l files. Each 3-byte entry is a packed (type, mapsquare)
// tuple: bit 16 = type (0=land, 1=loc); bits 0..15 = (mapX<<8)|mapZ.
//
// Validation matches TS RebuildGetMapsHandler:
//   - Reject (silently) if buildArea.LastBuild + 10 < currentTick (stale).
//   - Reject (silently) if entries > MAPS_LIMIT (18).
//   - Skip per-entry if mapsquare not in buildArea.Mapsquares.
//   - Skip per-entry if cache.Preload().Data has no bytes for that file.
//
// No error-response opcodes are sent - clients retry on their own.
func handleRebuildGetMaps(p *Player, payload []byte) error {
	if p.client == nil || p.client.server == nil {
		return nil
	}
	s := p.client.server

	if p.buildArea.lastBuild+rebuildGetMapsLastBuildTicks < s.currentTick {
		return nil
	}

	nEntries := len(payload) / 3
	if nEntries > rebuildGetMapsMapsLimit {
		return nil
	}

	r := packet.NewPacket(payload)
	for i := 0; i < nEntries; i++ {
		packed := int(r.G3())
		mapsquare := uint16(packed & 0xFFFF)
		if !p.buildArea.mapsquares[mapsquare] {
			continue
		}
		typ := (packed >> 16) & 0x1
		mapX := int(mapsquare>>8) & 0xFF
		mapZ := int(mapsquare) & 0xFF
		switch typ {
		case 0:
			streamLand(p, mapX, mapZ)
		case 1:
			streamLoc(p, mapX, mapZ)
		}
	}

	// Mirrors TS RebuildGetMapsHandler.ts:66 — refresh activeZones to
	// the player's current 7×7 zone window now that maps are loaded.
	p.buildArea.rebuildZones()
	return nil
}
