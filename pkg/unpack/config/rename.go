package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zsrv/goscape/pkg/colorconv"
	"github.com/zsrv/goscape/pkg/pack"
)

// renameModelIdk renames a model file to the idk/ subdirectory.
// Collision suffix format: name_2, name_3, ...
//
// TS source: IdkConfig.ts:12-39.
func renameModelIdk(modelID int, name string, modelPack *pack.PackFile, srcDir string, errorf func(string, ...any)) string {
	existingFiles := pack.ListFilesExt(filepath.Join(srcDir, "models"), ".ob2")

	current := modelPack.GetByID(modelID)
	if !strings.HasPrefix(current, "model_") {
		return current
	}

	// Build collision-free name with idk_ prefix if needed.
	attempt := name
	if !strings.HasPrefix(name, "idk_") {
		attempt = "idk_" + name
	}
	i := 2
	for modelPack.GetByName(attempt) != -1 {
		base := name
		if !strings.HasPrefix(name, "idk_") {
			base = "idk_" + name
		}
		attempt = fmt.Sprintf("%s_%d", base, i)
		i++
	}
	if attempt != name {
		name = attempt
	}

	// Move the file on disk (or report missing).
	// TS IdkConfig.ts:27-32.
	filePath := findFileInList(existingFiles, current+".ob2")
	if filePath != "" {
		dest := filepath.Join(srcDir, "models", "idk", name+".ob2")
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)
		_ = os.Rename(filePath, dest)
	} else if errorf != nil {
		errorf("Model not found on filesystem idk %s", current)
	}

	modelPack.Register(modelID, name)
	return name
}

// renameModelNpc renames a model file to the npc/ subdirectory.
// Collision suffix format: namei2, namei3, ... (no underscore before i).
//
// TS source: NpcConfig.ts:12-39.
func renameModelNpc(modelID int, name string, modelPack *pack.PackFile, srcDir string, errorf func(string, ...any)) string {
	existingFiles := pack.ListFilesExt(filepath.Join(srcDir, "models"), ".ob2")

	current := modelPack.GetByID(modelID)
	if !strings.HasPrefix(current, "model_") {
		return current
	}

	// Build collision-free name with npc_ prefix if needed.
	attempt := name
	if !strings.HasPrefix(name, "npc_") {
		attempt = "npc_" + name
	}
	i := 2
	for modelPack.GetByName(attempt) != -1 {
		base := name
		if !strings.HasPrefix(name, "npc_") {
			base = "npc_" + name
		}
		attempt = fmt.Sprintf("%si%d", base, i)
		i++
	}
	if attempt != name {
		name = attempt
	}

	// Move the file on disk (or report missing).
	// TS NpcConfig.ts:27-32.
	filePath := findFileInList(existingFiles, current+".ob2")
	if filePath != "" {
		dest := filepath.Join(srcDir, "models", "npc", name+".ob2")
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)
		_ = os.Rename(filePath, dest)
	} else if errorf != nil {
		errorf("Model not found on filesystem npc %s", current)
	}

	modelPack.Register(modelID, name)
	return name
}

// renameModelObj renames a model file to the obj/ subdirectory.
// Collision suffix format: namei2, namei3, ... (no underscore before i).
//
// TS source: ObjConfig.ts:12-39.
func renameModelObj(modelID int, name string, modelPack *pack.PackFile, srcDir string, errorf func(string, ...any)) string {
	existingFiles := pack.ListFilesExt(filepath.Join(srcDir, "models"), ".ob2")

	current := modelPack.GetByID(modelID)
	if !strings.HasPrefix(current, "model_") {
		return current
	}

	// Build collision-free name with obj_ prefix if needed.
	attempt := name
	if !strings.HasPrefix(name, "obj_") {
		attempt = "obj_" + name
	}
	i := 2
	for modelPack.GetByName(attempt) != -1 {
		base := name
		if !strings.HasPrefix(name, "obj_") {
			base = "obj_" + name
		}
		attempt = fmt.Sprintf("%si%d", base, i)
		i++
	}
	if attempt != name {
		name = attempt
	}

	// Move the file on disk (or report missing).
	// TS ObjConfig.ts:27-32.
	filePath := findFileInList(existingFiles, current+".ob2")
	if filePath != "" {
		dest := filepath.Join(srcDir, "models", "obj", name+".ob2")
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)
		_ = os.Rename(filePath, dest)
	} else if errorf != nil {
		errorf("Model not found on filesystem obj %s", current)
	}

	modelPack.Register(modelID, name)
	return name
}

// renameModelSpot renames a model file to the spot/ subdirectory.
// Collision suffix format: name_2, name_3, ...
//
// TS source: SpotAnimConfig.ts:12-39.
func renameModelSpot(modelID int, name string, modelPack *pack.PackFile, srcDir string, errorf func(string, ...any)) string {
	existingFiles := pack.ListFilesExt(filepath.Join(srcDir, "models"), ".ob2")

	current := modelPack.GetByID(modelID)
	if !strings.HasPrefix(current, "model_") {
		return current
	}

	// Build collision-free name with spot_ prefix if needed.
	attempt := name
	if !strings.HasPrefix(name, "spot_") {
		attempt = "spot_" + name
	}
	i := 2
	for modelPack.GetByName(attempt) != -1 {
		base := name
		if !strings.HasPrefix(name, "spot_") {
			base = "spot_" + name
		}
		attempt = fmt.Sprintf("%s_%d", base, i)
		i++
	}
	if attempt != name {
		name = attempt
	}

	// Move the file on disk (or report missing).
	// TS SpotAnimConfig.ts:27-32.
	filePath := findFileInList(existingFiles, current+".ob2")
	if filePath != "" {
		dest := filepath.Join(srcDir, "models", "spot", name+".ob2")
		_ = os.MkdirAll(filepath.Dir(dest), 0o755)
		_ = os.Rename(filePath, dest)
	} else if errorf != nil {
		errorf("Model not found on filesystem spot %s", current)
	}

	modelPack.Register(modelID, name)
	return name
}

// renameModelLoc strips _ld and shape suffixes from the model name.
// Does NOT move files; loc models are not relocated.
//
// TS source: LocConfig.ts:8-20.
func renameModelLoc(modelID int, shape int, modelPack *pack.PackFile) string {
	name := modelPack.GetByID(modelID)

	if strings.HasSuffix(name, "_ld") {
		name = name[:len(name)-3]
	}

	suffix := LocShapeSuffix[shape]
	if strings.HasSuffix(name, suffix) {
		name = name[:len(name)-2]
	}

	return name
}

// findFileInList returns the first path in list whose base filename equals basename.
// TS: existingFiles.find(x => x.endsWith(`/${model}.ob2`)).
func findFileInList(list []string, basename string) string {
	for _, f := range list {
		if strings.HasSuffix(f, "/"+basename) {
			return f
		}
	}
	return ""
}

// colorconvReverseHsl wraps colorconv.ReverseHsl.
// Alias kept here so all recol helpers import only this file.
func colorconvReverseHsl(hsl int) []int {
	return colorconv.ReverseHsl(hsl)
}

// firstOrZero returns (slice[0], true) if the slice is non-empty, else (0, false).
func firstOrZero(s []int) (int, bool) {
	if len(s) > 0 {
		return s[0], true
	}
	return 0, false
}

// recolEntry holds one src/dst HSL pair collected during an opcode walk.
type recolEntry struct {
	srcRaw int
	dstRaw int
}
