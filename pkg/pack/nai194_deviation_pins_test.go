package pack

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestNAI194_PackFileSingletonsDeferred_NoModuleLevelParamPack(t *testing.T) {
	decls := scanPackageDecls(t)
	for _, banned := range []string{
		"ParamPack", "EnumPack", "ObjPack", "LocPack", "InterfacePack",
		"StructPack", "CategoryPack", "SpotAnimPack", "NpcPack", "InvPack",
		"SynthPack", "SeqPack", "DbRowPack",
	} {
		if decls[banned] {
			t.Errorf("found top-level decl %q in pkg/pack — violates NAI-194-D-PACKFILE-SINGLETONS-DEFERRED", banned)
		}
	}
}

func TestNAI194_ValidateDeferred_NoBuildVerifyInParamSource(t *testing.T) {
	body, err := os.ReadFile("param.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"BuildVerify", "BUILD_VERIFY", "validateParam", "checkCRC", "checkcrc"} {
		if strings.Contains(string(body), banned) {
			t.Errorf("found %q in pkg/pack/param.go — violates NAI-194-D-VALIDATE-DEFERRED", banned)
		}
	}
}

// packParamConfigs's client output is TS-faithful "empty": p2(count)
// then count*p1(0). Pinned because the field is the TS contract for
// the client return even though TS PackShared.ts:323 discards it
// (`() => {}`) and goscape's packAndSaveParam likewise does not
// write it into the client jagfile.
func TestPackParamConfigs_ClientOutputAllZeroPerTSCallback(t *testing.T) {
	pf := newTestPF("param", map[int]string{0: "a", 1: "b", 2: "c", 3: "d"})
	cfgs := map[string][]ConfigLine{
		"a": {{Key: "type", Value: objtype.ScriptVarTypeInt}, {Key: "default", Value: "1"}},
		"b": {{Key: "type", Value: objtype.ScriptVarTypeString}, {Key: "default", Value: "x"}},
		"c": {{Key: "type", Value: objtype.ScriptVarTypeBoolean}, {Key: "default", Value: "yes"}, {Key: "autodisable", Value: false}},
	}
	_, client, err := packParamConfigs(cfgs, pf, &paramLookups{})
	if err != nil {
		t.Fatalf("packParamConfigs: %v", err)
	}
	want := []byte{0x00, 0x04, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(client.Dat.Data, want) {
		t.Fatalf("client.Dat: got % x, want % x", client.Dat.Data, want)
	}
}
