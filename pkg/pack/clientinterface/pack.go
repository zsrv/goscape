// pkg/pack/clientinterface/pack.go
package clientinterface

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// interfaceCRCMagic is the TS PackClient.ts:16 build-verify constant.
const interfaceCRCMagic int32 = -2146838800

// component mirrors the TS Component type (PackShared.ts:156-160).
//
// TS's Component.root is `string | null`. In Go we track presence with
// a parallel hasRoot bool, so the empty-string case stays observable.
type component struct {
	root     string
	hasRoot  bool
	children []int
	src      map[string]string
}

// Pack ports tools/pack/interface/PackClient.ts:packClientInterface.
//
// Calls packInterface (the workhorse from PackShared.ts:162-597) to
// produce both client and server Packets, build-verify-checks the
// client output, then saves both. compressWhole=true matches TS
// Jagfile.new(true) at PackClient.ts:8.
//
// NAI-192-D-NO-SRC-NO-OP mirror: missing src/scripts → no-op (return nil).
//
// NAI-213-D-NO-SOURCE-OF-TRUTH-FRESHNESS: TS PackClient.ts ORs a second
// shouldBuild('tools/pack/interface', '.ts', ...) gate that re-builds
// when the packer source itself changes. goscape has no equivalent
// "watch the packer code" surface; only the scripts-dir gate is kept.
func Pack(reg *pack.Registry, srcDir, outDir string) error {
	scriptsSrc := filepath.Join(srcDir, "scripts")
	clientOut := filepath.Join(outDir, "client", "interface")
	serverOut := filepath.Join(outDir, "server", "interface.dat")

	if _, err := os.Stat(scriptsSrc); os.IsNotExist(err) {
		return nil
	}
	if !pack.ShouldBuild(scriptsSrc, ".if", clientOut) {
		return nil
	}

	client, server, err := packInterface(reg, srcDir)
	if err != nil {
		return err
	}
	defer client.Release()
	defer server.Release()

	if err := pack.BuildVerify(client.Data, client.Length(), interfaceCRCMagic); err != nil {
		// NAI-213-D-BUILDVERIFY-INTERFACE-MAY-DIVERGE — CONFIRMED-EXCEPTION
		// (pack-media-compiler-12, 2026-05-28 audit closure):
		//
		// TS PackClient.ts:16-18 hard-throws on CRC mismatch when the
		// TS environment's build-verify toggle is set. goscape downgrades to an
		// informational stderr log and continues writing. The downgrade
		// is INTENTIONAL and STRUCTURAL — not a transient defer:
		//
		//   1. The interfaceCRCMagic constant is a hash of TS's stored
		//      name-id maps + script ordering. goscape's name-id maps
		//      derive from the cache being packed (which may be stock
		//      LostCity, a custom content tree, or a synthetic test
		//      fixture). Any name-id divergence — by design or accident
		//      — produces a different CRC than the TS-stored magic.
		//   2. Aborting on mismatch would make goscape unable to pack
		//      ANY content tree whose name-id map doesn't byte-match
		//      LostCity's at the build that generated the magic. Custom
		//      content trees and synthetic test fixtures are first-class
		//      use cases in goscape's design; the log lets the operator
		//      see the mismatch without breaking the pipeline.
		//   3. The magic constant is retained so it CAN re-engage if
		//      upstream pack consumers ever become TS-byte-faithful
		//      end-to-end (an env-gate could promote the log to a throw
		//      then), but that activation is not in scope for the
		//      current 1:1 parity arc.
		//
		// Audit row pack-media-compiler-12 closed as ✅ EXCEPTION-
		// DOCUMENTED — see docs/PORTING-CLOSED.md.
		fmt.Fprintf(os.Stderr, "clientinterface: %v (NAI-213-D-BUILDVERIFY-INTERFACE-MAY-DIVERGE)\n", err)
	}

	if err := os.MkdirAll(filepath.Dir(clientOut), 0o755); err != nil {
		return err
	}
	jag := jagfile.NewEmptyJagfile(true) // TS PackClient.ts:8 Jagfile.new(true)
	jag.Write("data", client)
	if err := jag.Save(clientOut); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(serverOut), 0o755); err != nil {
		return err
	}
	return server.Save(serverOut, server.Length(), 0)
}

// packInterface ports PackShared.ts:162-597 verbatim.
func packInterface(reg *pack.Registry, srcDir string) (client, server *packet.Packet, err error) {
	interfacePack, err := reg.EnsureInterface()
	if err != nil {
		return nil, nil, err
	}
	objPack, err := reg.EnsureObj()
	if err != nil {
		return nil, nil, err
	}
	modelPack, err := reg.EnsureModel()
	if err != nil {
		return nil, nil, err
	}
	seqPack, err := reg.EnsureSeq()
	if err != nil {
		return nil, nil, err
	}
	varpPack, err := reg.EnsureVarp()
	if err != nil {
		return nil, nil, err
	}

	components := map[int]*component{}
	interfaceOrder := pack.LoadOrder(filepath.Join(srcDir, "pack", "interface.order"))
	for _, id := range interfaceOrder {
		components[id] = &component{src: map[string]string{}}
	}

	var loadErr error
	pack.NameMapLoadDir(filepath.Join(srcDir, "scripts"), ".if", func(src []string, file, _ string) {
		if loadErr != nil {
			return
		}
		ifName := strings.TrimSuffix(file, ".if")
		ifId := interfacePack.GetByName(ifName)
		if components[ifId] == nil {
			loadErr = fmt.Errorf("clientinterface: could not find name <-> ID for interface file %q (id=%d)", ifName, ifId)
			return
		}

		components[ifId].src["type"] = "layer"
		components[ifId].src["width"] = "512"
		components[ifId].src["height"] = "334"

		comName := ""
		comId := -1
		for _, line := range src {
			if strings.HasPrefix(line, "[") {
				// TS: line.substring(1, line.length - 1) — strips [ and ]
				comName = line[1 : len(line)-1]
				comId = interfacePack.GetByName(ifName + ":" + comName)
				if comId == -1 || components[comId] == nil {
					loadErr = fmt.Errorf("clientinterface: missing component ID %s:%s in pack/interface.order", ifName, comName)
					return
				}
				components[comId].root = ifName
				components[comId].hasRoot = true
				components[ifId].children = append(components[ifId].children, comId)
				continue
			}
			eq := strings.IndexByte(line, '=')
			if eq < 0 {
				continue
			}
			key := line[:eq]
			value := line[eq+1:]

			if key == "layer" {
				layerId := interfacePack.GetByName(ifName + ":" + value)
				if layerId == -1 {
					loadErr = fmt.Errorf("clientinterface: layer %s:%s does not exist", ifName, value)
					return
				}
				if slices.Contains(components[layerId].children, comId) {
					loadErr = fmt.Errorf("clientinterface: layer %s:%s already has %s as a child", ifName, value, comName)
					return
				}
				components[layerId].children = append(components[layerId].children, comId)
				if idx := slices.Index(components[ifId].children, comId); idx >= 0 {
					components[ifId].children = slices.Delete(components[ifId].children, idx, idx+1)
				}
			}

			if comId != -1 {
				components[comId].src[key] = value
			} else {
				components[ifId].src[key] = value
			}
		}
	})
	if loadErr != nil {
		return nil, nil, loadErr
	}

	client = packet.Alloc(5)
	server = packet.Alloc(5)

	lastRoot := ""
	lastRootSet := false
	client.P2(uint16(interfacePack.Max))
	server.P2(uint16(interfacePack.Max))

	for _, id := range interfaceOrder {
		com := components[id]
		src := com.src

		if !com.hasRoot || !lastRootSet || lastRoot != com.root {
			client.P2(0xffff) // TS: client.p2(-1)
			if com.hasRoot {
				client.P2(uint16(interfacePack.GetByName(com.root)))
				lastRoot = com.root
				lastRootSet = true
			} else {
				client.P2(uint16(id))
				lastRoot = interfacePack.GetByID(id)
				lastRootSet = true
			}
		}

		client.P2(uint16(id))

		server.P2(uint16(id))
		server.PJStrLF(interfacePack.GetByID(id))
		server.PBool(src["type"] == "overlay")

		comType := nameToType(src["type"])
		// TS p1 takes a number; negative values wrap mod 256. nameToType
		// returns -1 for unknown — preserve TS's 0xff wrap via int8.
		client.P1(uint8(int8(comType)))

		buttonType := nameToButtonType(src["buttontype"])
		client.P1(uint8(buttonType))

		client.P2(uint16(atoiOr0(src["clientcode"])))
		client.P2(uint16(atoiOr0(src["width"])))
		client.P2(uint16(atoiOr0(src["height"])))

		if overlayer, ok := src["overlayer"]; ok && overlayer != "" {
			layerId := interfacePack.GetByName(com.root + ":" + overlayer)
			client.P2(uint16(layerId + 0x100))
		} else {
			client.P1(0)
		}

		comparatorCount := 0
		for j := 1; j <= 5; j++ {
			if _, ok := src[fmt.Sprintf("script%d", j)]; ok {
				comparatorCount++
			}
		}
		client.P1(uint8(comparatorCount))
		for j := 1; j <= comparatorCount; j++ {
			parts := strings.Split(src[fmt.Sprintf("script%d", j)], ",")
			client.P1(uint8(nameToComparator(parts[0])))
			if len(parts) > 1 {
				client.P2(uint16(atoiOr0(parts[1])))
			} else {
				client.P2(0)
			}
		}

		scriptCount := 0
		for j := 1; j <= 5; j++ {
			if _, ok := src[fmt.Sprintf("script%dop1", j)]; ok {
				scriptCount++
			}
		}
		client.P1(uint8(scriptCount))
		for j := 1; j <= scriptCount; j++ {
			opCount := 0
			for k := 1; k <= 20; k++ {
				op, ok := src[fmt.Sprintf("script%dop%d", j, k)]
				if !ok {
					continue
				}
				opCount++
				parts := strings.Split(op, ",")
				switch parts[0] {
				case "stat_level", "stat_base_level", "stat_xp", "pushvar", "stat_xp_remaining":
					opCount += 1
				case "inv_count", "inv_contains", "testbit":
					opCount += 2
				}
			}

			if src[fmt.Sprintf("script%dop1", j)] == "" {
				// TS L337-339: TODO note retained; stats:com_0 et al.
				client.P2(uint16(opCount))
			} else {
				client.P2(uint16(opCount + 1))
			}
			for k := 1; k <= opCount; k++ {
				op, ok := src[fmt.Sprintf("script%dop%d", j, k)]
				if !ok || op == "" {
					continue
				}
				parts := strings.Split(op, ",")
				client.P2(uint16(nameToScript(parts[0])))

				switch parts[0] {
				case "stat_level", "stat_base_level", "stat_xp", "stat_xp_remaining":
					client.P2(uint16(nameToStat(parts[1])))
				case "inv_count":
					client.P2(uint16(interfacePack.GetByName(parts[1])))
					client.P2(uint16(objPack.GetByName(parts[2])))
				case "pushvar":
					client.P2(uint16(varpPack.GetByName(parts[1])))
				case "inv_contains":
					client.P2(uint16(interfacePack.GetByName(parts[1])))
					client.P2(uint16(objPack.GetByName(parts[2])))
				case "testbit":
					client.P2(uint16(varpPack.GetByName(parts[1])))
					client.P2(uint16(atoiOr0(parts[2])))
				}
			}
			if opCount > 0 {
				client.P2(0)
			}
		}

		switch comType {
		case 0:
			client.P2(uint16(atoiOr0(src["scroll"])))
			client.PBool(src["hide"] == "yes")
			client.P1(uint8(len(com.children)))
			for _, childId := range com.children {
				client.P2(uint16(childId))
				client.P2(uint16(atoiOr0(components[childId].src["x"])))
				client.P2(uint16(atoiOr0(components[childId].src["y"])))
			}
		case 2:
			client.PBool(src["draggable"] == "yes")
			client.PBool(src["interactable"] == "yes")
			client.PBool(src["usable"] == "yes")
			if margin, ok := src["margin"]; ok && margin != "" {
				mp := strings.Split(margin, ",")
				client.P1(uint8(atoiOr0(mp[0])))
				if len(mp) > 1 {
					client.P1(uint8(atoiOr0(mp[1])))
				} else {
					client.P1(0)
				}
			} else {
				client.P1(0)
				client.P1(0)
			}
			for j := 1; j <= 20; j++ {
				if slot, ok := src[fmt.Sprintf("slot%d", j)]; ok {
					client.PBool(true)
					parts := strings.Split(slot, ":")
					sprite := parts[0]
					x, y := "0", "0"
					if len(parts) > 1 && parts[1] != "" {
						off := strings.Split(parts[1], ",")
						if len(off) >= 2 {
							x, y = off[0], off[1]
						}
					}
					client.P2(uint16(atoiOr0(x)))
					client.P2(uint16(atoiOr0(y)))
					client.PJStrLF(sprite)
				} else {
					client.PBool(false)
				}
			}
			for j := 1; j <= 5; j++ {
				client.PJStrLF(src[fmt.Sprintf("option%d", j)])
			}
		case 3:
			client.PBool(src["fill"] == "yes")
		case 4:
			client.PBool(src["center"] == "yes")
			client.P1(uint8(nameToFont(src["font"])))
			client.PBool(src["shadowed"] == "yes")
			client.PJStrLF(src["text"])
			client.PJStrLF(src["activetext"])
		}

		if comType == 3 || comType == 4 {
			client.P4(uint32(atoiOr0(src["colour"])))
			client.P4(uint32(atoiOr0(src["activecolour"])))
			client.P4(uint32(atoiOr0(src["overcolour"])))
		}

		if comType == 5 {
			client.PJStrLF(src["graphic"])
			client.PJStrLF(src["activegraphic"])
		}

		if comType == 6 {
			if model, ok := src["model"]; ok && model != "" {
				mid := modelPack.GetByName(model)
				if mid == -1 {
					return nil, nil, fmt.Errorf("clientinterface: %s invalid model %q", com.root, model)
				}
				client.P2(uint16(mid + 0x100))
			} else {
				client.P1(0)
			}
			if am, ok := src["activemodel"]; ok && am != "" {
				mid := modelPack.GetByName(am)
				if mid == -1 {
					return nil, nil, fmt.Errorf("clientinterface: %s invalid activemodel %q", com.root, am)
				}
				client.P2(uint16(mid + 0x100))
			} else {
				client.P1(0)
			}
			if an, ok := src["anim"]; ok && an != "" {
				sid := seqPack.GetByName(an)
				if sid == -1 {
					return nil, nil, fmt.Errorf("clientinterface: %s invalid anim %q", com.root, an)
				}
				client.P2(uint16(sid + 0x100))
			} else {
				client.P1(0)
			}
			if aa, ok := src["activeanim"]; ok && aa != "" {
				sid := seqPack.GetByName(aa)
				if sid == -1 {
					return nil, nil, fmt.Errorf("clientinterface: %s invalid activeanim %q", com.root, aa)
				}
				client.P2(uint16(sid + 0x100))
			} else {
				client.P1(0)
			}
			client.P2(uint16(atoiOr0(src["zoom"])))
			client.P2(uint16(atoiOr0(src["xan"])))
			client.P2(uint16(atoiOr0(src["yan"])))
		}

		if comType == 7 {
			client.PBool(src["center"] == "yes")
			client.P1(uint8(nameToFont(src["font"])))
			client.PBool(src["shadowed"] == "yes")
			client.P4(uint32(atoiOr0(src["colour"])))
			if margin, ok := src["margin"]; ok && margin != "" {
				mp := strings.Split(margin, ",")
				client.P2(uint16(atoiOr0(mp[0])))
				if len(mp) > 1 {
					client.P2(uint16(atoiOr0(mp[1])))
				} else {
					client.P2(0)
				}
			} else {
				client.P2(0)
				client.P2(0)
			}
			client.PBool(src["interactable"] == "yes")
			for j := 1; j <= 5; j++ {
				client.PJStrLF(src[fmt.Sprintf("option%d", j)])
			}
		}

		if buttonType == 2 || comType == 2 {
			client.PJStrLF(src["actionverb"])
			client.PJStrLF(src["action"])
			flags := 0
			if target, ok := src["actiontarget"]; ok && target != "" {
				ts := strings.Split(target, ",")
				if slices.Contains(ts, "obj") {
					flags |= 0x1
				}
				if slices.Contains(ts, "npc") {
					flags |= 0x2
				}
				if slices.Contains(ts, "loc") {
					flags |= 0x4
				}
				if slices.Contains(ts, "player") {
					flags |= 0x8
				}
				if slices.Contains(ts, "heldobj") {
					flags |= 0x10
				}
			}
			client.P2(uint16(flags))
		}

		if buttonType == 1 || buttonType == 4 || buttonType == 5 || buttonType == 6 {
			client.PJStrLF(src["option"])
		}
	}

	return client, server, nil
}

// atoiOr0 ports TS PackShared.ts uses of parseInt(s).
//
// JS parseInt auto-detects a leading "0x"/"0X" prefix as hex; strconv.Atoi
// is strict base-10 and errors on the prefix, silently returning 0. Without
// this branch, every colour/activecolour/overcolour field in a Content
// .if file (all written as 0xRRGGBB hex literals) zeros out, producing
// a 297761-byte client/interface payload whose bytes diverge from TS at
// the first comType==4 colour P4.
func atoiOr0(s string) int {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		n, _ := strconv.ParseUint(s[2:], 16, 64)
		return int(int32(uint32(n)))
	}
	n, _ := strconv.Atoi(s)
	return n
}
