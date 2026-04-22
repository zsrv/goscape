package objtype

import (
	"fmt"

	"github.com/zsrv/goscape/pkg/io/packet"
)

// HuntModeType values mirror TS HuntModeType.
// See Engine-TS/src/engine/entity/hunt/HuntModeType.ts.
const (
	HuntModeOff     = 0
	HuntModePlayer  = 1
	HuntModeNpc     = 2
	HuntModeObj     = 3
	HuntModeScenery = 4
)

// HuntVis values mirror TS HuntVis.
const (
	HuntVisOff         = 0
	HuntVisLineOfSight = 1
	HuntVisLineOfWalk  = 2
)

// HuntNobodyNear values mirror TS HuntNobodyNear.
const (
	HuntNobodyNearKeepHunting = 0
	HuntNobodyNearPauseHunt   = 1
)

// HuntCheckNotTooStrong values mirror TS HuntCheckNotTooStrong.
const (
	HuntCheckNotTooStrongOff               = 0
	HuntCheckNotTooStrongOutsideWilderness = 1
)

// HuntCheckVar is one entry in HuntType.CheckVars: a variable-ID plus a
// comparison operator and constant. Populated by decode codes 18/19/20.
type HuntCheckVar struct {
	VarID     int
	Condition string
	Val       int
}

// HuntType is a single `hunt.dat` record.
type HuntType struct {
	ConfigType
	Type               int
	CheckVis           int
	CheckNotTooStrong  int
	CheckNotBusy       bool
	FindKeepHunting    bool
	FindNewMode        int
	NobodyNear         int
	CheckNotCombat     int
	CheckNotCombatSelf int
	CheckAfk           bool
	Rate               int
	CheckCategory      int
	CheckNpc           int
	CheckObj           int
	CheckLoc           int
	CheckInv           int
	CheckObjParam      int
	CheckInvCondition  string
	CheckInvVal        int
	CheckVars          []HuntCheckVar
}

// NewHuntType returns a HuntType populated with TS defaults.
func NewHuntType(id int) *HuntType {
	return &HuntType{
		ConfigType: ConfigType{
			ID: id,
		},
		Type:               HuntModeOff,
		CheckVis:           HuntVisOff,
		CheckNotTooStrong:  HuntCheckNotTooStrongOff,
		FindNewMode:        NPCModeNull,
		NobodyNear:         HuntNobodyNearPauseHunt,
		CheckNotCombat:     -1,
		CheckNotCombatSelf: -1,
		CheckAfk:           true,
		Rate:               1,
		CheckCategory:      -1,
		CheckNpc:           -1,
		CheckObj:           -1,
		CheckLoc:           -1,
		CheckInv:           -1,
		CheckObjParam:      -1,
		CheckInvVal:        -1,
	}
}

// HuntTypeConfigs is the parsed registry.
type HuntTypeConfigs struct {
	ConfigNames map[string]int
	Configs     []*HuntType
}

// Decode dispatches on the hunt-config opcode, matching TS HuntType.decode
// at Engine-TS/src/cache/config/HuntType.ts:99-147.
func (t *HuntType) Decode(code uint8, dat *packet.Packet) error {
	switch code {
	case 1:
		t.Type = int(dat.G1())
	case 2:
		t.CheckVis = int(dat.G1())
	case 3:
		t.CheckNotTooStrong = int(dat.G1())
	case 4:
		t.CheckNotBusy = true
	case 5:
		t.FindKeepHunting = true
	case 6:
		t.FindNewMode = int(dat.G1())
	case 7:
		t.NobodyNear = int(dat.G1())
	case 8:
		t.CheckNotCombat = int(dat.G2())
	case 9:
		t.CheckNotCombatSelf = int(dat.G2())
	case 10:
		t.CheckAfk = false
	case 11:
		t.Rate = int(dat.G2())
	case 12:
		t.CheckCategory = int(dat.G2())
	case 13:
		t.CheckNpc = int(dat.G2())
	case 14:
		t.CheckObj = int(dat.G2())
	case 15:
		t.CheckLoc = int(dat.G2())
	case 16:
		t.CheckInv = int(dat.G2())
		t.CheckObj = int(dat.G2())
		t.CheckInvCondition = dat.GJStrLF()
		t.CheckInvVal = int(int32(dat.G4()))
	case 17:
		t.CheckInv = int(dat.G2())
		t.CheckObjParam = int(dat.G2())
		t.CheckInvCondition = dat.GJStrLF()
		t.CheckInvVal = int(int32(dat.G4()))
	case 18, 19, 20:
		t.CheckVars = append(t.CheckVars, HuntCheckVar{
			VarID:     int(dat.G2()),
			Condition: dat.GJStrLF(),
			Val:       int(int32(dat.G4())),
		})
	case 250:
		t.DebugName = dat.GJStrLF()
	default:
		return fmt.Errorf("unrecognized hunt config code %d", code)
	}
	return nil
}
