// Package worldmap ports TS tools/pack/map/Worldmap.ts. Builds
// data/pack/mapview/worldmap.jag from per-map binary outputs and
// CSV/font/sprite assets.
package worldmap

// refColors is the 103-entry hardcoded floor-color palette from TS
// Worldmap.ts:520-622 @ dee467c8. Each row carries one [edge, fill]
// pair. rev-274 (dee467c8) recomputed every fill color and appended
// two entries (slayer_tower, morytania_dark_green) — transcribed
// wholesale from the TS refColors table, not hand-edited deltas.
// Each row is [edgeColor, fillColor] as u32.
// Ordering matches FloType id ordering — if Content adds a new
// flo before this is in sync, the packer will panic on out-of-
// range access. Update both Content and this table together.
var refColors = [103][2]uint32{
	{0x00000038, 0x00847776}, // cliff
	{0x00000016, 0x00504746}, // cliff2
	{0x00000022, 0x00564d4d}, // cliff3
	{0x0000002d, 0x00766a69}, // cliff4
	{0x00000000, 0x003a1c0c}, // woodenfloor
	{0x00000000, 0x004f648d}, // water
	{0x00000000, 0x001f6248}, // gungywater
	{0x0000001e, 0x00504746}, // greyroof
	{0x01500053, 0x00bbb9b2}, // desertroof
	{0x0000001a, 0x00463f3f}, // road
	{0x0000000b, 0x00100f0f}, // darkstone
	{0x00000000, 0x003f3934}, // pebblefloor
	{0x0000a822, 0x0095342f}, // redfloor
	{0x0090ec0c, 0x00503911}, // mudfloor
	{0x0090ec0c, 0x003b250c}, // mudfloor_bump
	{0x00715411, 0x00674204}, // mudfloor2
	{0x00715411, 0x004f3a03}, // mudfloor2_bump
	{0x03815422, 0x0012068c}, // bluefloor
	{0x00000000, 0x00e15f15}, // lava
	{0x00000000, 0x004d4d4f}, // marble
	{0x00915419, 0x00887006}, // sandfloor
	{0x00a09419, 0x00544a24}, // l_brownfloor1
	{0x00a09419, 0x00605528}, // l_brownfloor1_bump
	{0x00000000, 0x0038322d}, // cliff_textured
	{0x00b09435, 0x00c09757}, // sand_cliff
	{0x00c06821, 0x00786d42}, // sand_rock
	{0x00000000, 0x00282011}, // oldbrick
	{0x00000000, 0x00595650}, // brick
	{0x01611c14, 0x0036760f}, // grass
	{0x0150004f, 0x00aea5a4}, // ice_overlay
	{0x00a11012, 0x003b3507}, // upass_floor
	{0x00000000, 0x00363029}, // stone_texture
	{0x0150004a, 0x00b6babe}, // ice_overlay_blue
	{0x0000001a, 0x003e3836}, // road_bridge
	{0x00000000, 0x003a1c0c}, // woodenfloor_bridge
	{0x0080f013, 0x005b4d14}, // mud5_overlay
	{0x00000000, 0x00060404}, // black
	{0x03106027, 0x0058697c}, // lightblue
	{0x00000000, 0x00799ed7}, // water_fountain
	{0x03808427, 0x00404995}, // bluefloor2
	{0x03107420, 0x00324a5b}, // waterfallblue
	{0xff21542a, 0x00503000}, // invisible
	{0xff21542a, 0x00503000}, // invisible_occ
	{0x0000001a, 0x0046403f}, // road_no_occlude
	{0x00000000, 0x003a1c0c}, // woodenfloor_no_occlude
	{0x00000000, 0x00282011}, // oldbrick_no_occlude
	{0x00000000, 0x00595650}, // brick_no_occlude
	{0x01611c14, 0x00154301}, // grassland
	{0x01011413, 0x002c3306}, // muddygrass
	{0x00c11c15, 0x0064630c}, // vmuddygrass
	{0x0141181f, 0x00569006}, // lightgrass
	{0x0110ac21, 0x00747a26}, // sandygrass
	{0x00d10c0f, 0x003f3407}, // swamp
	{0x0250e011, 0x0014412b}, // swamp2
	{0x00000027, 0x00544b4a}, // lightrock
	{0x00000019, 0x00423b3b}, // darkrock
	{0x0000000f, 0x001b1717}, // verydarkrock
	{0x0150004f, 0x00aea4a4}, // ice
	{0x01500049, 0x009699a2}, // blueice
	{0x01500049, 0x0097a498}, // greenice
	{0x00c0742b, 0x00847349}, // desert1
	{0x00b0a436, 0x00cdbe41}, // desert2
	{0x0090ec0c, 0x004a3817}, // mud1
	{0x0090b415, 0x003c351a}, // mud2
	{0x00a11012, 0x0045310f}, // mud3
	{0x00715411, 0x005b3303}, // mud4
	{0x0080f013, 0x00382906}, // mud5
	{0x00b09435, 0x00a39845}, // sand
	{0x0090b415, 0x005b431c}, // mud2_skew
	{0x00a11012, 0x004a3003}, // mud3_skew
	{0x00715411, 0x004f2d03}, // mud4_skew
	{0x00000001, 0x00060404}, // black_rock
	{0x03106027, 0x004b6387}, // dullblue
	{0xffd06027, 0x00745453}, // purple_pink
	{0x03106027, 0x00416075}, // lightblue_underlay
	{0x00b0a82d, 0x00a47a33}, // desert_shadow
	{0x0080782f, 0x00816648}, // duel_arena
	{0x0080283c, 0x00bfa054}, // duelarena
	{0x00b06826, 0x00817a38}, // hive
	{0x0080a41c, 0x00775032}, // agility
	{0x00909012, 0x00483c28}, // brownmud
	{0x0090301a, 0x003a3529}, // mountain_overlay
	{0x00903014, 0x0028261c}, // mountain_dark_overlay
	{0x00000000, 0x007c693b}, // elfbrick
	{0x01203c0f, 0x002c321c}, // elf_wastelands
	{0x00015407, 0x000a0100}, // dark_red
	{0x0390601d, 0x00322d50}, // grey_blue
	{0x00a04417, 0x00605435}, // viking_town_overlay
	{0x0090b814, 0x00362811}, // viking_mud_overlay
	{0x00903c0d, 0x000f0e0b}, // viking_cave_overlay
	{0x00909012, 0x002f271a}, // legendssword_cave
	{0x0090301a, 0x00494133}, // mountain
	{0x01906416, 0x00344e2b}, // darkgrass
	{0x00903014, 0x002b251e}, // mountain_dark
	{0x03108c23, 0x00295163}, // grey_blue_underlay
	{0x00a0c40f, 0x004e4818}, // autumnal
	{0x00a04419, 0x00443e30}, // viking_town
	{0x00a0440d, 0x001d1b15}, // viking_town_dark
	{0x01c0a018, 0x00224f22}, // jungle_green
	{0x0150dc13, 0x00466b17}, // jungle_dark_green
	{0x00a0c011, 0x0055431b}, // mm_town_overlay
	{0x01203c0f, 0x00303224}, // slayer_tower
	{0x02605c07, 0x00030704}, // morytania_dark_green
}
