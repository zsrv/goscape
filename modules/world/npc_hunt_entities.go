package world

import (
	"github.com/zsrv/goscape/pkg/objtype"
)

// huntNpcs iterates NPCs in the grid within huntRange and returns
// those passing type + category + distance filters. Matches TS
// Npc.huntNpcs at Engine-TS/.../Npc.ts:975-977, which delegates to
// HuntIterator's NPC branch at ScriptIterators.ts:98-120.
//
// Filter coverage (NAI-9):
//   - Range: Chebyshev distance <= n.huntRange
//   - Level match: NearbyNpcs is level-filtered internally
//   - CheckNpc: type-ID filter (-1 == allow all)
//   - CheckCategory: NpcType.Category filter (-1 == allow all)
//
// DEFERRED to audit pass:
//   - CheckVis (TS ScriptIterators.ts:113-118) — LoS/LoW gate.
//
// Does NOT exclude self (TS doesn't either — NPC can appear in its
// own zone's NPC list and pass all filters). Preserves TS quirk.
func (n *Npc) huntNpcs(s *Server, hunt *objtype.HuntType) []entity {
	if s.grid == nil || s.npcTypes == nil {
		return nil
	}
	zoneRadius := 1 + n.huntRange/8
	nids := s.grid.NearbyNpcs(n.x, n.z, n.level, zoneRadius)
	var hunted []entity
	for _, nid := range nids {
		if nid < 0 || nid >= len(s.npcs) {
			continue
		}
		other := s.npcs[nid]
		if other == nil {
			continue
		}
		if hunt.CheckNpc != -1 && other.typeId != hunt.CheckNpc {
			continue
		}
		if hunt.CheckCategory != -1 {
			if other.typeId < 0 || other.typeId >= len(s.npcTypes.Configs) {
				continue
			}
			ot := s.npcTypes.Configs[other.typeId]
			if ot == nil || ot.Category != hunt.CheckCategory {
				continue
			}
		}
		dx := other.x - n.x
		if dx < 0 {
			dx = -dx
		}
		dz := other.z - n.z
		if dz < 0 {
			dz = -dz
		}
		if dx > n.huntRange || dz > n.huntRange {
			continue
		}
		// CheckVis gate — TS ScriptIterators.ts:113-118.
		// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
		if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
				n.level, n.x, n.z, other.x, other.z, 1, 1, 1, 0) {
			continue
		}
		if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
			!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
				n.level, n.x, n.z, other.x, other.z, 1, 1, 1, 0) {
			continue
		}
		hunted = append(hunted, other)
	}
	return hunted
}

// huntObjs iterates dynamic objs in zone-radius and returns those
// passing the filter chain. Matches TS Npc.huntObjs at
// Engine-TS/.../Npc.ts:979-981 (HuntIterator OBJ branch at
// ScriptIterators.ts:121-144).
//
// goscape Zone.Objs contains only LifecycleDespawn (dynamic) objs
// by construction (pkg/zone/zone.go:221). Matches TS comment
// "scripting only cares about dynamic objs??" at
// ScriptIterators.ts:122.
//
// Filter coverage (NAI-9):
//   - Range: Chebyshev distance <= n.huntRange
//   - CheckObj: obj.Type filter (-1 == allow all)
//   - CheckCategory: ObjType.Category filter (-1 == allow all)
//
// DEFERRED to audit pass:
//   - CheckVis (TS ScriptIterators.ts:137-142) — LoS/LoW gate.
func (n *Npc) huntObjs(s *Server, hunt *objtype.HuntType) []entity {
	if s.zoneMap == nil || s.objTypes == nil {
		return nil
	}
	zoneRadius := 1 + n.huntRange/8
	var hunted []entity
	for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius) {
		for _, o := range zn.Objs {
			if o == nil {
				continue
			}
			if hunt.CheckObj != -1 && o.Type != hunt.CheckObj {
				continue
			}
			if hunt.CheckCategory != -1 {
				if o.Type < 0 || o.Type >= len(s.objTypes.Configs) {
					continue
				}
				ot := s.objTypes.Configs[o.Type]
				if ot == nil || ot.Category != hunt.CheckCategory {
					continue
				}
			}
			dx := o.X - n.x
			if dx < 0 {
				dx = -dx
			}
			dz := o.Z - n.z
			if dz < 0 {
				dz = -dz
			}
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// CheckVis gate — TS ScriptIterators.ts:137-142.
			// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
			if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
					n.level, n.x, n.z, o.X, o.Z, 1, 1, 1, 0) {
				continue
			}
			if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
					n.level, n.x, n.z, o.X, o.Z, 1, 1, 1, 0) {
				continue
			}
			hunted = append(hunted, o)
		}
	}
	return hunted
}

// huntLocs iterates locs in zone-radius and returns those passing
// the filter chain. Matches TS Npc.huntLocs at
// Engine-TS/.../Npc.ts:983-985 (HuntIterator SCENERY branch at
// ScriptIterators.ts:145-167).
//
// Zone.Locs contains both static (Forever) and dynamic (Despawn)
// locs — matches TS getAllLocsSafe(true). The "static" label follows
// pkg/entity/lifecycle.go:8, where LifecycleForever is documented as
// "statics — never despawn" (pkg/zone's AddStaticLoc docstring uses
// "Respawn" for the same concept — a pre-existing terminology drift
// that NAI-9 doesn't attempt to resolve).
//
// Multi-tile locs use SW corner for distance (l.X/l.Z ARE the SW
// corner by goscape entity.Entity convention, matching TS which
// passes {x: loc.x, z: loc.z} to distanceToSW).
//
// Filter coverage (NAI-9):
//   - Range: Chebyshev distance <= n.huntRange
//   - CheckLoc: loc.Type() filter (-1 == allow all)
//   - CheckCategory: LocType.Category filter (-1 == allow all)
//
// DEFERRED to audit pass:
//   - CheckVis (TS ScriptIterators.ts:160-165) — LoS/LoW gate.
func (n *Npc) huntLocs(s *Server, hunt *objtype.HuntType) []entity {
	if s.zoneMap == nil || s.locTypes == nil {
		return nil
	}
	zoneRadius := 1 + n.huntRange/8
	var hunted []entity
	for _, zn := range s.zoneMap.NearbyZones(n.level, n.x, n.z, zoneRadius) {
		for _, l := range zn.Locs {
			if l == nil {
				continue
			}
			if hunt.CheckLoc != -1 && l.Type() != hunt.CheckLoc {
				continue
			}
			if hunt.CheckCategory != -1 {
				if l.Type() < 0 || l.Type() >= len(s.locTypes.Configs) {
					continue
				}
				lt := s.locTypes.Configs[l.Type()]
				if lt == nil || lt.Category != hunt.CheckCategory {
					continue
				}
			}
			dx := l.X - n.x
			if dx < 0 {
				dx = -dx
			}
			dz := l.Z - n.z
			if dz < 0 {
				dz = -dz
			}
			if dx > n.huntRange || dz > n.huntRange {
				continue
			}
			// CheckVis gate — TS ScriptIterators.ts:160-165.
			// gamemap==nil short-circuits to gate-pass; see NAI-12 spec § error handling.
			// FIDELITY: TS passes {loc.x, loc.z} (1×1), not multi-tile width/length;
			// goscape preserves that quirk.
			if hunt.CheckVis == objtype.HuntVisLineOfSight && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfSight(
					n.level, n.x, n.z, l.X, l.Z, 1, 1, 1, 0) {
				continue
			}
			if hunt.CheckVis == objtype.HuntVisLineOfWalk && s.gamemap != nil &&
				!s.gamemap.Pathfinder.LineValidator.HasLineOfWalk(
					n.level, n.x, n.z, l.X, l.Z, 1, 1, 1, 0) {
				continue
			}
			hunted = append(hunted, l)
		}
	}
	return hunted
}
