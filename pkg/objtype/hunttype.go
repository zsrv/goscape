package objtype

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
