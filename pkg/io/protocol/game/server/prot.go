package server

// Op describes a server→client game packet opcode.
type Op struct {
	Opcode      byte
	PayloadSize int // 0=fixed-zero, 2=fixed-2, 4=fixed-4, -1=1-byte-len, -2=2-byte-len
}

// Modal interface opcodes and logout — sub-spec 1 only.
// Remaining ~40 server opcodes added in sub-specs 2–4.
var (
	OpIfClose        = Op{Opcode: 129, PayloadSize: 0}
	OpIfOpenMain     = Op{Opcode: 168, PayloadSize: 2}
	OpIfOpenChat     = Op{Opcode: 14, PayloadSize: 2}
	OpIfOpenSide     = Op{Opcode: 195, PayloadSize: 2}
	OpIfOpenMainSide = Op{Opcode: 28, PayloadSize: 4}
	OpLogout         = Op{Opcode: 142, PayloadSize: 0}

	// S5f: per-component setters (fire-and-forget wire ops used by IF_SET* opcodes).
	OpIfSetText       = Op{Opcode: 201, PayloadSize: -2}
	OpIfSetModel      = Op{Opcode: 87, PayloadSize: 4}
	OpIfSetNpcHead    = Op{Opcode: 204, PayloadSize: 4}
	OpIfSetPlayerHead = Op{Opcode: 197, PayloadSize: 2}
	OpIfSetAnim       = Op{Opcode: 146, PayloadSize: 4}
	OpIfSetHide       = Op{Opcode: 26, PayloadSize: 3}
	OpIfSetObject     = Op{Opcode: 46, PayloadSize: 6}
	OpIfSetColour     = Op{Opcode: 2, PayloadSize: 4}
	OpIfSetPosition   = Op{Opcode: 209, PayloadSize: 6}
	OpIfSetRecol      = Op{Opcode: 103, PayloadSize: 6}
	OpIfSetTab        = Op{Opcode: 167, PayloadSize: 3}
	OpIfSetTabActive  = Op{Opcode: 84, PayloadSize: 1}

	// S5g: dialog suspension. Server sends only the opcode byte to
	// prompt the client to open an "enter a number" count dialog.
	OpPCountDialog = Op{Opcode: 243, PayloadSize: 0}

	// Camera control. TS ServerGameProt.CAM_RESET = 239, payload 0.
	// Sent by the CAM_RESET script opcode to reset the client's camera.
	OpCamReset = Op{Opcode: 239, PayloadSize: 0}

	OpRebuildNormal    = Op{Opcode: 237, PayloadSize: -2}
	OpUpdateInvFull    = Op{Opcode: 98, PayloadSize: -2}
	OpUpdateInvPartial = Op{Opcode: 213, PayloadSize: -2}
	OpPlayerInfo       = Op{Opcode: 184, PayloadSize: -2}
	OpNpcInfo          = Op{Opcode: 1, PayloadSize: -2}

	OpUpdateStat            = Op{Opcode: 44, PayloadSize: 6}
	OpUpdateRunEnergy       = Op{Opcode: 68, PayloadSize: 1}
	OpUpdateInvStopTransmit = Op{Opcode: 15, PayloadSize: 2}

	// Per-player VARP sync. VARP_SMALL fits values in [-128, 127];
	// VARP_LARGE carries full int32 range.
	OpVarpSmall = Op{Opcode: 150, PayloadSize: 3}
	OpVarpLarge = Op{Opcode: 175, PayloadSize: 6}

	OpUpdateZonePartialFollows  = Op{Opcode: 7, PayloadSize: 2}
	OpUpdateZoneFullFollows     = Op{Opcode: 135, PayloadSize: 2}
	OpUpdateZonePartialEnclosed = Op{Opcode: 162, PayloadSize: -2}

	// Zone-nested opcodes, reused as top-level packets for per-player
	// UpdateZonePartialFollows delivery. Sizes match the Java client's
	// SERVERPROT_SIZES at the matching indices.
	OpLocAddChange = Op{Opcode: 59, PayloadSize: 4}
	OpLocAnim      = Op{Opcode: 42, PayloadSize: 4}
	OpLocDel       = Op{Opcode: 76, PayloadSize: 2}
	OpLocMerge     = Op{Opcode: 23, PayloadSize: 14}
	OpMapAnim      = Op{Opcode: 191, PayloadSize: 6}
	OpMapProjAnim  = Op{Opcode: 69, PayloadSize: 15}
	OpObjAdd       = Op{Opcode: 223, PayloadSize: 5}
	OpObjCount     = Op{Opcode: 151, PayloadSize: 7}
	OpObjDel       = Op{Opcode: 49, PayloadSize: 3}
	OpObjReveal    = Op{Opcode: 50, PayloadSize: 7}

	// Map-data streaming (sub-spec 5b). 991-byte chunk size per DATA_LAND/LOC.
	OpDataLand     = Op{Opcode: 132, PayloadSize: -2}
	OpDataLoc      = Op{Opcode: 220, PayloadSize: -2}
	OpDataLandDone = Op{Opcode: 80, PayloadSize: 2}
	OpDataLocDone  = Op{Opcode: 20, PayloadSize: 2}

	// Interaction (sub-spec 6a).
	OpUnsetMapFlag = Op{Opcode: 19, PayloadSize: 0}

	// RuneScript S2 — chat output emitted by the MES opcode.
	OpMessageGame = Op{Opcode: 4, PayloadSize: -1}
)
