// Package worldmap ports TS tools/pack/map/Worldmap.ts. Builds
// data/pack/mapview/worldmap.jag from per-map binary outputs and
// CSV/font/sprite assets.
package worldmap

// refColors is the 79-entry hardcoded floor-color palette from TS
// Worldmap.ts:533-613 (lines 534-612 inclusive carry one [edge, fill]
// pair each). Each row is [edgeColor, fillColor] as u32.
// Ordering matches FloType id ordering — if Content adds a new
// flo before this is in sync, the packer will panic on out-of-
// range access. Update both Content and this table together.
var refColors = [79][2]uint32{
	{0x00000038, 0x009c8f8e}, // cliff
	{0x00000016, 0x004a4242}, // cliff2
	{0x00000022, 0x004a4242}, // cliff3
	{0x0000002d, 0x00817574}, // cliff4
	{0x00000000, 0x003b1d0c}, // woodenfloor
	{0x00000000, 0x0050648d}, // water
	{0x00000000, 0x00206349}, // gungywater
	{0x0000001e, 0x004a4342}, // greyroof
	{0x01500053, 0x00c2c2ba}, // desertroof
	{0x0000001a, 0x00413b3a}, // road
	{0x0000000b, 0x00191616}, // darkstone
	{0x00000000, 0x00403935}, // pebblefloor
	{0x0000a822, 0x00783633}, // redfloor
	{0x0090ec0c, 0x00513a12}, // mudfloor
	{0x0090ec0c, 0x00120d03}, // mudfloor_bump
	{0x00715411, 0x006f4805}, // mudfloor2
	{0x00715411, 0x003c1d01}, // mudfloor2_bump
	{0x03815422, 0x00061789}, // bluefloor
	{0x00000000, 0x00e36116}, // lava
	{0x00000000, 0x004e4e50}, // marble
	{0x00915419, 0x00583a03}, // sandfloor
	{0x00a09419, 0x004d4320}, // l_brownfloor1
	{0x00a09419, 0x00574730}, // l_brownfloor1_bump
	{0x00000000, 0x0039332d}, // cliff_textured
	{0x00b09435, 0x009b9243}, // sand_cliff
	{0x00c06821, 0x005b5441}, // sand_rock
	{0x00000000, 0x00282211}, // oldbrick
	{0x00000000, 0x00333333}, // brick
	{0x01611c14, 0x003b5e0b}, // grass
	{0x0150004f, 0x00c8c0c0}, // ice_overlay
	{0x00a11012, 0x00734c05}, // upass_floor
	{0x00000000, 0x0037312a}, // stone_texture
	{0x0150004a, 0x00aaafb4}, // ice_overlay_blue
	{0x0000001a, 0x00474040}, // road_bridge
	{0x00000000, 0x003b1d0c}, // woodenfloor_bridge
	{0x0080f013, 0x0062420d}, // mud5_overlay
	{0x00000000, 0x00060505}, // black
	{0x03106027, 0x003e516e}, // lightblue
	{0x00000000, 0x0079a0d7}, // water_fountain
	{0x03808427, 0x004e4a82}, // bluefloor2
	{0x03107420, 0x00364c61}, // waterfallblue
	{0xff21542a, 0x00503000}, // invisible
	{0xff21542a, 0x00503000}, // invisible_occ
	{0x0000001a, 0x00474040}, // road_no_occlude
	{0x00000000, 0x003b1d0c}, // woodenfloor_no_occlude
	{0x00000000, 0x00282211}, // oldbrick_no_occlude
	{0x00000000, 0x00333333}, // brick_no_occlude
	{0x01611c14, 0x0036570a}, // grassland
	{0x01011413, 0x00393c07}, // muddygrass
	{0x00c11c15, 0x00403f07}, // vmuddygrass
	{0x0141181f, 0x00556c0e}, // lightgrass
	{0x0110ac21, 0x0065832a}, // sandygrass
	{0x00d10c0f, 0x00282805}, // swamp
	{0x0250e011, 0x0012513d}, // swamp2
	{0x00000027, 0x00605656}, // lightrock
	{0x00000019, 0x004c4444}, // darkrock
	{0x0000000f, 0x00171414}, // verydarkrock
	{0x0150004f, 0x00c2bbba}, // ice
	{0x01500049, 0x00b6b9bf}, // blueice
	{0x01500049, 0x0098a599}, // greenice
	{0x00c0742b, 0x00797343}, // desert1
	{0x00b0a436, 0x009b9243}, // desert2
	{0x0090ec0c, 0x001b1303}, // mud1
	{0x0090b415, 0x006b5d22}, // mud2
	{0x00a11012, 0x0039280b}, // mud3
	{0x00715411, 0x005c2403}, // mud4
	{0x0080f013, 0x00665716}, // mud5
	{0x00b09435, 0x00b48d4e}, // sand
	{0x0090b415, 0x0052471a}, // mud2_skew
	{0x00a11012, 0x006c4a0e}, // mud3_skew
	{0x00715411, 0x003c2701}, // mud4_skew
	{0x00000001, 0x00060505}, // black_rock
	{0x03106027, 0x00435e79}, // dullblue
	{0xffd06027, 0x008d524f}, // purple_pink
	{0x03106027, 0x0043779b}, // lightblue_underlay
	{0x00b0a82d, 0x00a9974a}, // desert_shadow
	{0x0080782f, 0x00886b4d}, // duel_arena
	{0x0080283c, 0x00b47a4e}, // duelarena
	{0x00b06826, 0x0071673f}, // hive
}
