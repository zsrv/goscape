// pkg/pack/compiler/trigger/server_trigger_type.go
//
// Ports TS src/runescript/trigger/ServerTriggerType.ts: 156 *TriggerType
// singletons declared in TS class-static order. ServerTriggerTypeAll
// mirrors TS ServerTriggerType.ALL (push-on-construction order).
//
// TS `name` is uppercase; goscape `Identifier` is lowercase — translated
// eagerly here so no runtime ToLower is required.
package trigger

import (
	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
	typ "github.com/zsrv/goscape/pkg/pack/compiler/type"
)

var (
	// PROC (id=0): all-pointers set (TS uses Object.values(PointerType)).
	ServerTriggerProc = &TriggerType{
		ID:              0,
		Identifier:      "proc",
		SubjectMode:     ModeName,
		AllowParameters: true,
		AllowReturns:    true,
		Pointers:        pointer.NewPointerSet(pointer.All...),
	}

	// LABEL (id=1): all-pointers set.
	ServerTriggerLabel = &TriggerType{
		ID:              1,
		Identifier:      "label",
		SubjectMode:     ModeName,
		AllowParameters: true,
		Pointers:        pointer.NewPointerSet(pointer.All...),
	}

	ServerTriggerDebugProc = &TriggerType{
		ID:              2,
		Identifier:      "debugproc",
		SubjectMode:     ModeName,
		AllowParameters: true,
		Pointers:        pointer.NewPointerSet(pointer.ActivePlayer),
	}

	ServerTriggerApNpc1 = &TriggerType{
		ID:          3,
		Identifier:  "apnpc1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerApNpc2 = &TriggerType{
		ID:          4,
		Identifier:  "apnpc2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerApNpc3 = &TriggerType{
		ID:          5,
		Identifier:  "apnpc3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerApNpc4 = &TriggerType{
		ID:          6,
		Identifier:  "apnpc4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerApNpc5 = &TriggerType{
		ID:          7,
		Identifier:  "apnpc5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerApNpcU = &TriggerType{
		ID:          8,
		Identifier:  "apnpcu",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastUseItem, pointer.LastUseSlot, pointer.ActiveNpc),
	}

	ServerTriggerApNpcT = &TriggerType{
		ID:          9,
		Identifier:  "apnpct",
		SubjectMode: NewModeType(typ.ScriptVarComponent, false, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerOpNpc1 = &TriggerType{
		ID:          10,
		Identifier:  "opnpc1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerOpNpc2 = &TriggerType{
		ID:          11,
		Identifier:  "opnpc2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerOpNpc3 = &TriggerType{
		ID:          12,
		Identifier:  "opnpc3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerOpNpc4 = &TriggerType{
		ID:          13,
		Identifier:  "opnpc4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerOpNpc5 = &TriggerType{
		ID:          14,
		Identifier:  "opnpc5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerOpNpcU = &TriggerType{
		ID:          15,
		Identifier:  "opnpcu",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastUseItem, pointer.LastUseSlot, pointer.ActiveNpc),
	}

	ServerTriggerOpNpcT = &TriggerType{
		ID:          16,
		Identifier:  "opnpct",
		SubjectMode: NewModeType(typ.ScriptVarComponent, false, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveNpc),
	}

	ServerTriggerAiApNpc1 = &TriggerType{
		ID:          17,
		Identifier:  "ai_apnpc1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerAiApNpc2 = &TriggerType{
		ID:          18,
		Identifier:  "ai_apnpc2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerAiApNpc3 = &TriggerType{
		ID:          19,
		Identifier:  "ai_apnpc3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerAiApNpc4 = &TriggerType{
		ID:          20,
		Identifier:  "ai_apnpc4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerAiApNpc5 = &TriggerType{
		ID:          21,
		Identifier:  "ai_apnpc5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerAiOpNpc1 = &TriggerType{
		ID:          24,
		Identifier:  "ai_opnpc1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerAiOpNpc2 = &TriggerType{
		ID:          25,
		Identifier:  "ai_opnpc2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerAiOpNpc3 = &TriggerType{
		ID:          26,
		Identifier:  "ai_opnpc3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerAiOpNpc4 = &TriggerType{
		ID:          27,
		Identifier:  "ai_opnpc4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerAiOpNpc5 = &TriggerType{
		ID:          28,
		Identifier:  "ai_opnpc5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveNpc2),
	}

	ServerTriggerApObj1 = &TriggerType{
		ID:          31,
		Identifier:  "apobj1",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerApObj2 = &TriggerType{
		ID:          32,
		Identifier:  "apobj2",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerApObj3 = &TriggerType{
		ID:          33,
		Identifier:  "apobj3",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerApObj4 = &TriggerType{
		ID:          34,
		Identifier:  "apobj4",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerApObj5 = &TriggerType{
		ID:          35,
		Identifier:  "apobj5",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerApObjU = &TriggerType{
		ID:          36,
		Identifier:  "apobju",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastUseItem, pointer.LastUseSlot, pointer.ActiveObj),
	}

	ServerTriggerApObjT = &TriggerType{
		ID:          37,
		Identifier:  "apobjt",
		SubjectMode: NewModeType(typ.ScriptVarComponent, false, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerOpObj1 = &TriggerType{
		ID:          38,
		Identifier:  "opobj1",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerOpObj2 = &TriggerType{
		ID:          39,
		Identifier:  "opobj2",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerOpObj3 = &TriggerType{
		ID:          40,
		Identifier:  "opobj3",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerOpObj4 = &TriggerType{
		ID:          41,
		Identifier:  "opobj4",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerOpObj5 = &TriggerType{
		ID:          42,
		Identifier:  "opobj5",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerOpObjU = &TriggerType{
		ID:          43,
		Identifier:  "opobju",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastUseItem, pointer.LastUseSlot, pointer.ActiveObj),
	}

	ServerTriggerOpObjT = &TriggerType{
		ID:          44,
		Identifier:  "opobjt",
		SubjectMode: NewModeType(typ.ScriptVarComponent, false, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveObj),
	}

	ServerTriggerAiApObj1 = &TriggerType{
		ID:          45,
		Identifier:  "ai_apobj1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerAiApObj2 = &TriggerType{
		ID:          46,
		Identifier:  "ai_apobj2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerAiApObj3 = &TriggerType{
		ID:          47,
		Identifier:  "ai_apobj3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerAiApObj4 = &TriggerType{
		ID:          48,
		Identifier:  "ai_apobj4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerAiApObj5 = &TriggerType{
		ID:          49,
		Identifier:  "ai_apobj5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerAiOpObj1 = &TriggerType{
		ID:          52,
		Identifier:  "ai_opobj1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerAiOpObj2 = &TriggerType{
		ID:          53,
		Identifier:  "ai_opobj2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerAiOpObj3 = &TriggerType{
		ID:          54,
		Identifier:  "ai_opobj3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerAiOpObj4 = &TriggerType{
		ID:          55,
		Identifier:  "ai_opobj4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerAiOpObj5 = &TriggerType{
		ID:          56,
		Identifier:  "ai_opobj5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveObj),
	}

	ServerTriggerApLoc1 = &TriggerType{
		ID:          59,
		Identifier:  "aploc1",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerApLoc2 = &TriggerType{
		ID:          60,
		Identifier:  "aploc2",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerApLoc3 = &TriggerType{
		ID:          61,
		Identifier:  "aploc3",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerApLoc4 = &TriggerType{
		ID:          62,
		Identifier:  "aploc4",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerApLoc5 = &TriggerType{
		ID:          63,
		Identifier:  "aploc5",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerApLocU = &TriggerType{
		ID:          64,
		Identifier:  "aplocu",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastUseItem, pointer.LastUseSlot, pointer.ActiveLoc),
	}

	ServerTriggerApLocT = &TriggerType{
		ID:          65,
		Identifier:  "aploct",
		SubjectMode: NewModeType(typ.ScriptVarComponent, false, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerOpLoc1 = &TriggerType{
		ID:          66,
		Identifier:  "oploc1",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerOpLoc2 = &TriggerType{
		ID:          67,
		Identifier:  "oploc2",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerOpLoc3 = &TriggerType{
		ID:          68,
		Identifier:  "oploc3",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerOpLoc4 = &TriggerType{
		ID:          69,
		Identifier:  "oploc4",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerOpLoc5 = &TriggerType{
		ID:          70,
		Identifier:  "oploc5",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerOpLocU = &TriggerType{
		ID:          71,
		Identifier:  "oplocu",
		SubjectMode: NewModeType(typ.ScriptVarLoc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastUseItem, pointer.LastUseSlot, pointer.ActiveLoc),
	}

	ServerTriggerOpLocT = &TriggerType{
		ID:          72,
		Identifier:  "oploct",
		SubjectMode: NewModeType(typ.ScriptVarComponent, false, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActiveLoc),
	}

	ServerTriggerAiApLoc1 = &TriggerType{
		ID:          73,
		Identifier:  "ai_aploc1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerAiApLoc2 = &TriggerType{
		ID:          74,
		Identifier:  "ai_aploc2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerAiApLoc3 = &TriggerType{
		ID:          75,
		Identifier:  "ai_aploc3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerAiApLoc4 = &TriggerType{
		ID:          76,
		Identifier:  "ai_aploc4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerAiApLoc5 = &TriggerType{
		ID:          77,
		Identifier:  "ai_aploc5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerAiOpLoc1 = &TriggerType{
		ID:          80,
		Identifier:  "ai_oploc1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerAiOpLoc2 = &TriggerType{
		ID:          81,
		Identifier:  "ai_oploc2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerAiOpLoc3 = &TriggerType{
		ID:          82,
		Identifier:  "ai_oploc3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerAiOpLoc4 = &TriggerType{
		ID:          83,
		Identifier:  "ai_oploc4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerAiOpLoc5 = &TriggerType{
		ID:          84,
		Identifier:  "ai_oploc5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActiveLoc),
	}

	ServerTriggerApPlayer1 = &TriggerType{
		ID:          87,
		Identifier:  "applayer1",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerApPlayer2 = &TriggerType{
		ID:          88,
		Identifier:  "applayer2",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerApPlayer3 = &TriggerType{
		ID:          89,
		Identifier:  "applayer3",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerApPlayer4 = &TriggerType{
		ID:          90,
		Identifier:  "applayer4",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerApPlayer5 = &TriggerType{
		ID:          91,
		Identifier:  "applayer5",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerApPlayerU = &TriggerType{
		ID:          92,
		Identifier:  "applayeru",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastUseItem, pointer.LastUseSlot, pointer.ActivePlayer2),
	}

	ServerTriggerApPlayerT = &TriggerType{
		ID:          93,
		Identifier:  "applayert",
		SubjectMode: NewModeType(typ.ScriptVarComponent, false, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerOpPlayer1 = &TriggerType{
		ID:          94,
		Identifier:  "opplayer1",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerOpPlayer2 = &TriggerType{
		ID:          95,
		Identifier:  "opplayer2",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerOpPlayer3 = &TriggerType{
		ID:          96,
		Identifier:  "opplayer3",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerOpPlayer4 = &TriggerType{
		ID:          97,
		Identifier:  "opplayer4",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerOpPlayer5 = &TriggerType{
		ID:          98,
		Identifier:  "opplayer5",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerOpPlayerU = &TriggerType{
		ID:          99,
		Identifier:  "opplayeru",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastUseItem, pointer.LastUseSlot, pointer.ActivePlayer2),
	}

	ServerTriggerOpPlayerT = &TriggerType{
		ID:          100,
		Identifier:  "opplayert",
		SubjectMode: NewModeType(typ.ScriptVarComponent, false, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.ActivePlayer2),
	}

	ServerTriggerAiApPlayer1 = &TriggerType{
		ID:          101,
		Identifier:  "ai_applayer1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	ServerTriggerAiApPlayer2 = &TriggerType{
		ID:          102,
		Identifier:  "ai_applayer2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	ServerTriggerAiApPlayer3 = &TriggerType{
		ID:          103,
		Identifier:  "ai_applayer3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	ServerTriggerAiApPlayer4 = &TriggerType{
		ID:          104,
		Identifier:  "ai_applayer4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	ServerTriggerAiApPlayer5 = &TriggerType{
		ID:          105,
		Identifier:  "ai_applayer5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	ServerTriggerAiOpPlayer1 = &TriggerType{
		ID:          108,
		Identifier:  "ai_opplayer1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	ServerTriggerAiOpPlayer2 = &TriggerType{
		ID:          109,
		Identifier:  "ai_opplayer2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	ServerTriggerAiOpPlayer3 = &TriggerType{
		ID:          110,
		Identifier:  "ai_opplayer3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	ServerTriggerAiOpPlayer4 = &TriggerType{
		ID:          111,
		Identifier:  "ai_opplayer4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	ServerTriggerAiOpPlayer5 = &TriggerType{
		ID:          112,
		Identifier:  "ai_opplayer5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.ActivePlayer),
	}

	// QUEUE (id=116): TS omits subjectMode so it defaults to SubjectMode.Name.
	ServerTriggerQueue = &TriggerType{
		ID:              116,
		Identifier:      "queue",
		SubjectMode:     ModeName,
		AllowParameters: true,
		Pointers:        pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerAiQueue1 = &TriggerType{
		ID:          117,
		Identifier:  "ai_queue1",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue2 = &TriggerType{
		ID:          118,
		Identifier:  "ai_queue2",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue3 = &TriggerType{
		ID:          119,
		Identifier:  "ai_queue3",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue4 = &TriggerType{
		ID:          120,
		Identifier:  "ai_queue4",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue5 = &TriggerType{
		ID:          121,
		Identifier:  "ai_queue5",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue6 = &TriggerType{
		ID:          122,
		Identifier:  "ai_queue6",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue7 = &TriggerType{
		ID:          123,
		Identifier:  "ai_queue7",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue8 = &TriggerType{
		ID:          124,
		Identifier:  "ai_queue8",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue9 = &TriggerType{
		ID:          125,
		Identifier:  "ai_queue9",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue10 = &TriggerType{
		ID:          126,
		Identifier:  "ai_queue10",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue11 = &TriggerType{
		ID:          127,
		Identifier:  "ai_queue11",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue12 = &TriggerType{
		ID:          128,
		Identifier:  "ai_queue12",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue13 = &TriggerType{
		ID:          129,
		Identifier:  "ai_queue13",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue14 = &TriggerType{
		ID:          130,
		Identifier:  "ai_queue14",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue15 = &TriggerType{
		ID:          131,
		Identifier:  "ai_queue15",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue16 = &TriggerType{
		ID:          132,
		Identifier:  "ai_queue16",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue17 = &TriggerType{
		ID:          133,
		Identifier:  "ai_queue17",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue18 = &TriggerType{
		ID:          134,
		Identifier:  "ai_queue18",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue19 = &TriggerType{
		ID:          135,
		Identifier:  "ai_queue19",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	ServerTriggerAiQueue20 = &TriggerType{
		ID:          136,
		Identifier:  "ai_queue20",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc, pointer.LastInt),
	}

	// SOFTTIMER (id=137): TS omits subjectMode → default Name.
	ServerTriggerSoftTimer = &TriggerType{
		ID:              137,
		Identifier:      "softtimer",
		SubjectMode:     ModeName,
		AllowParameters: true,
		Pointers:        pointer.NewPointerSet(pointer.ActivePlayer),
	}

	// TIMER (id=138): TS omits subjectMode → default Name.
	ServerTriggerTimer = &TriggerType{
		ID:              138,
		Identifier:      "timer",
		SubjectMode:     ModeName,
		AllowParameters: true,
		Pointers:        pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerAiTimer = &TriggerType{
		ID:          139,
		Identifier:  "ai_timer",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc),
	}

	ServerTriggerOpHeld1 = &TriggerType{
		ID:          140,
		Identifier:  "opheld1",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerOpHeld2 = &TriggerType{
		ID:          141,
		Identifier:  "opheld2",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerOpHeld3 = &TriggerType{
		ID:          142,
		Identifier:  "opheld3",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerOpHeld4 = &TriggerType{
		ID:          143,
		Identifier:  "opheld4",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerOpHeld5 = &TriggerType{
		ID:          144,
		Identifier:  "opheld5",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerOpHeldU = &TriggerType{
		ID:          145,
		Identifier:  "opheldu",
		SubjectMode: NewModeType(typ.ScriptVarNamedObj, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastItem, pointer.LastSlot, pointer.LastUseItem, pointer.LastUseSlot),
	}

	ServerTriggerOpHeldT = &TriggerType{
		ID:          146,
		Identifier:  "opheldt",
		SubjectMode: NewModeType(typ.ScriptVarComponent, false, false),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerIfButton = &TriggerType{
		ID:          147,
		Identifier:  "if_button",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastCom),
	}

	ServerTriggerIfClose = &TriggerType{
		ID:          148,
		Identifier:  "if_close",
		SubjectMode: NewModeType(typ.ScriptVarInterface, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer),
	}

	ServerTriggerInvButton1 = &TriggerType{
		ID:          149,
		Identifier:  "inv_button1",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerIfButton1 = &TriggerType{
		ID:          149,
		Identifier:  "if_button1",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerInvButton2 = &TriggerType{
		ID:          150,
		Identifier:  "inv_button2",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerIfButton2 = &TriggerType{
		ID:          150,
		Identifier:  "if_button2",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerInvButton3 = &TriggerType{
		ID:          151,
		Identifier:  "inv_button3",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerIfButton3 = &TriggerType{
		ID:          151,
		Identifier:  "if_button3",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerInvButton4 = &TriggerType{
		ID:          152,
		Identifier:  "inv_button4",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerIfButton4 = &TriggerType{
		ID:          152,
		Identifier:  "if_button4",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerInvButton5 = &TriggerType{
		ID:          153,
		Identifier:  "inv_button5",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerIfButton5 = &TriggerType{
		ID:          153,
		Identifier:  "if_button5",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastItem, pointer.LastSlot),
	}

	ServerTriggerInvButtonD = &TriggerType{
		ID:          154,
		Identifier:  "inv_buttond",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastSlot, pointer.LastTargetSlot),
	}

	ServerTriggerIfButtonD = &TriggerType{
		ID:          154,
		Identifier:  "if_buttond",
		SubjectMode: NewModeType(typ.ScriptVarComponent, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.LastSlot, pointer.LastTargetSlot),
	}

	ServerTriggerWalkTrigger = &TriggerType{
		ID:          155,
		Identifier:  "walktrigger",
		SubjectMode: ModeName,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerLogin = &TriggerType{
		ID:          157,
		Identifier:  "login",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerLogout = &TriggerType{
		ID:           158,
		Identifier:   "logout",
		SubjectMode:  ModeNone,
		AllowReturns: true,
		Returns:      typ.PrimitiveBoolean,
		Pointers:     pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerTutorial = &TriggerType{
		ID:          159,
		Identifier:  "tutorial",
		SubjectMode: ModeNone,
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerAdvanceStat = &TriggerType{
		ID:          160,
		Identifier:  "advancestat",
		SubjectMode: NewModeType(typ.ScriptVarStat, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerMapZone = &TriggerType{
		ID:          161,
		Identifier:  "mapzone",
		SubjectMode: NewModeType(typ.PrimitiveMapzone, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerMapZoneExit = &TriggerType{
		ID:          162,
		Identifier:  "mapzoneexit",
		SubjectMode: NewModeType(typ.PrimitiveMapzone, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerZone = &TriggerType{
		ID:          163,
		Identifier:  "zone",
		SubjectMode: NewModeType(typ.PrimitiveCoord, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerZoneExit = &TriggerType{
		ID:          164,
		Identifier:  "zoneexit",
		SubjectMode: NewModeType(typ.PrimitiveCoord, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerChangeStat = &TriggerType{
		ID:          165,
		Identifier:  "changestat",
		SubjectMode: NewModeType(typ.ScriptVarStat, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActivePlayer, pointer.PActivePlayer),
	}

	ServerTriggerAiSpawn = &TriggerType{
		ID:          166,
		Identifier:  "ai_spawn",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc),
	}

	ServerTriggerAiDespawn = &TriggerType{
		ID:          167,
		Identifier:  "ai_despawn",
		SubjectMode: NewModeType(typ.ScriptVarNpc, true, true),
		Pointers:    pointer.NewPointerSet(pointer.ActiveNpc),
	}
)

// ServerTriggerTypeAll mirrors TS ServerTriggerType.ALL: every singleton in
// constructor-call order (which equals static-field-declaration order). 156
// entries. Indices are NOT IDs (TS IDs have gaps; INV_BUTTON*/IF_BUTTON*
// share IDs but get separate slots).
var ServerTriggerTypeAll = []*TriggerType{
	ServerTriggerProc, ServerTriggerLabel, ServerTriggerDebugProc,
	ServerTriggerApNpc1, ServerTriggerApNpc2, ServerTriggerApNpc3, ServerTriggerApNpc4, ServerTriggerApNpc5, ServerTriggerApNpcU, ServerTriggerApNpcT,
	ServerTriggerOpNpc1, ServerTriggerOpNpc2, ServerTriggerOpNpc3, ServerTriggerOpNpc4, ServerTriggerOpNpc5, ServerTriggerOpNpcU, ServerTriggerOpNpcT,
	ServerTriggerAiApNpc1, ServerTriggerAiApNpc2, ServerTriggerAiApNpc3, ServerTriggerAiApNpc4, ServerTriggerAiApNpc5,
	ServerTriggerAiOpNpc1, ServerTriggerAiOpNpc2, ServerTriggerAiOpNpc3, ServerTriggerAiOpNpc4, ServerTriggerAiOpNpc5,
	ServerTriggerApObj1, ServerTriggerApObj2, ServerTriggerApObj3, ServerTriggerApObj4, ServerTriggerApObj5, ServerTriggerApObjU, ServerTriggerApObjT,
	ServerTriggerOpObj1, ServerTriggerOpObj2, ServerTriggerOpObj3, ServerTriggerOpObj4, ServerTriggerOpObj5, ServerTriggerOpObjU, ServerTriggerOpObjT,
	ServerTriggerAiApObj1, ServerTriggerAiApObj2, ServerTriggerAiApObj3, ServerTriggerAiApObj4, ServerTriggerAiApObj5,
	ServerTriggerAiOpObj1, ServerTriggerAiOpObj2, ServerTriggerAiOpObj3, ServerTriggerAiOpObj4, ServerTriggerAiOpObj5,
	ServerTriggerApLoc1, ServerTriggerApLoc2, ServerTriggerApLoc3, ServerTriggerApLoc4, ServerTriggerApLoc5, ServerTriggerApLocU, ServerTriggerApLocT,
	ServerTriggerOpLoc1, ServerTriggerOpLoc2, ServerTriggerOpLoc3, ServerTriggerOpLoc4, ServerTriggerOpLoc5, ServerTriggerOpLocU, ServerTriggerOpLocT,
	ServerTriggerAiApLoc1, ServerTriggerAiApLoc2, ServerTriggerAiApLoc3, ServerTriggerAiApLoc4, ServerTriggerAiApLoc5,
	ServerTriggerAiOpLoc1, ServerTriggerAiOpLoc2, ServerTriggerAiOpLoc3, ServerTriggerAiOpLoc4, ServerTriggerAiOpLoc5,
	ServerTriggerApPlayer1, ServerTriggerApPlayer2, ServerTriggerApPlayer3, ServerTriggerApPlayer4, ServerTriggerApPlayer5, ServerTriggerApPlayerU, ServerTriggerApPlayerT,
	ServerTriggerOpPlayer1, ServerTriggerOpPlayer2, ServerTriggerOpPlayer3, ServerTriggerOpPlayer4, ServerTriggerOpPlayer5, ServerTriggerOpPlayerU, ServerTriggerOpPlayerT,
	ServerTriggerAiApPlayer1, ServerTriggerAiApPlayer2, ServerTriggerAiApPlayer3, ServerTriggerAiApPlayer4, ServerTriggerAiApPlayer5,
	ServerTriggerAiOpPlayer1, ServerTriggerAiOpPlayer2, ServerTriggerAiOpPlayer3, ServerTriggerAiOpPlayer4, ServerTriggerAiOpPlayer5,
	ServerTriggerQueue,
	ServerTriggerAiQueue1, ServerTriggerAiQueue2, ServerTriggerAiQueue3, ServerTriggerAiQueue4, ServerTriggerAiQueue5,
	ServerTriggerAiQueue6, ServerTriggerAiQueue7, ServerTriggerAiQueue8, ServerTriggerAiQueue9, ServerTriggerAiQueue10,
	ServerTriggerAiQueue11, ServerTriggerAiQueue12, ServerTriggerAiQueue13, ServerTriggerAiQueue14, ServerTriggerAiQueue15,
	ServerTriggerAiQueue16, ServerTriggerAiQueue17, ServerTriggerAiQueue18, ServerTriggerAiQueue19, ServerTriggerAiQueue20,
	ServerTriggerSoftTimer, ServerTriggerTimer, ServerTriggerAiTimer,
	ServerTriggerOpHeld1, ServerTriggerOpHeld2, ServerTriggerOpHeld3, ServerTriggerOpHeld4, ServerTriggerOpHeld5, ServerTriggerOpHeldU, ServerTriggerOpHeldT,
	ServerTriggerIfButton, ServerTriggerIfClose,
	ServerTriggerInvButton1, ServerTriggerIfButton1,
	ServerTriggerInvButton2, ServerTriggerIfButton2,
	ServerTriggerInvButton3, ServerTriggerIfButton3,
	ServerTriggerInvButton4, ServerTriggerIfButton4,
	ServerTriggerInvButton5, ServerTriggerIfButton5,
	ServerTriggerInvButtonD, ServerTriggerIfButtonD,
	ServerTriggerWalkTrigger, ServerTriggerLogin, ServerTriggerLogout, ServerTriggerTutorial,
	ServerTriggerAdvanceStat, ServerTriggerMapZone, ServerTriggerMapZoneExit,
	ServerTriggerZone, ServerTriggerZoneExit, ServerTriggerChangeStat,
	ServerTriggerAiSpawn, ServerTriggerAiDespawn,
}
