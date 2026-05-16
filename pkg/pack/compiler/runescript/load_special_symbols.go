// pkg/pack/compiler/runescript/load_special_symbols.go
package runescript

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/pack/compiler/pointer"
)

// LoadSpecialSymbols populates commandPointers from commandInfo and seeds
// SymbolMapper command + script maps. Mirrors TS
// src/runescript/ServerScriptCompilerApplication.ts L93-127.
//
// Iteration is sorted by numeric id (NAI-210-D-LOADER-SORTED-ITERATION) for
// reproducibility.
func LoadSpecialSymbols(
	commandInfo, scriptInfo *CompilerTypeInfo,
	mapper *SymbolMapper,
	commandPointers map[string]*pointer.PointerHolder,
	checkPointers bool,
) error {
	commandKeys := sortedNumericKeys(commandInfo.Map)
	for _, key := range commandKeys {
		name := commandInfo.Map[key]
		id, err := strconv.Atoi(key)
		if err != nil {
			return fmt.Errorf("LoadSpecialSymbols: invalid command id %q: %w", key, err)
		}

		hasPtrInfo := commandInfo.Require[key] != "" ||
			commandInfo.Set[key] != "" ||
			commandInfo.Corrupt[key] != ""

		if checkPointers && hasPtrInfo {
			required, err := parsePointerList(commandInfo.Require[key])
			if err != nil {
				return fmt.Errorf("command %q Require: %w", name, err)
			}
			required2, err := parsePointerList(commandInfo.Require2[key])
			if err != nil {
				return fmt.Errorf("command %q Require2: %w", name, err)
			}
			setter, err := parsePointerList(commandInfo.Set[key])
			if err != nil {
				return fmt.Errorf("command %q Set: %w", name, err)
			}
			setter2, err := parsePointerList(commandInfo.Set2[key])
			if err != nil {
				return fmt.Errorf("command %q Set2: %w", name, err)
			}
			corrupted, err := parsePointerList(commandInfo.Corrupt[key])
			if err != nil {
				return fmt.Errorf("command %q Corrupt: %w", name, err)
			}
			corrupted2, err := parsePointerList(commandInfo.Corrupt2[key])
			if err != nil {
				return fmt.Errorf("command %q Corrupt2: %w", name, err)
			}
			conditionalSet := commandInfo.Conditional[key]

			commandPointers[name] = &pointer.PointerHolder{
				Required:       required,
				Set:            setter,
				ConditionalSet: conditionalSet,
				Corrupted:      corrupted,
			}
			if required2.Len() > 0 || setter2.Len() > 0 || corrupted2.Len() > 0 {
				commandPointers["."+name] = &pointer.PointerHolder{
					Required:       required2,
					Set:            setter2,
					ConditionalSet: conditionalSet,
					Corrupted:      corrupted2,
				}
			}
		}

		mapper.PutCommand(id, name)
	}

	scriptKeys := sortedNumericKeys(scriptInfo.Map)
	for _, key := range scriptKeys {
		name := scriptInfo.Map[key]
		id, err := strconv.Atoi(key)
		if err != nil {
			return fmt.Errorf("LoadSpecialSymbols: invalid script id %q: %w", key, err)
		}
		mapper.PutScript(id, name)
	}
	return nil
}

// parsePointerList resolves a comma-separated pointer name list. Empty
// strings and the literal "none" produce an empty set (TS L121-122).
// Unknown names return error (TS L131 throws).
func parsePointerList(s string) (*pointer.PointerSet, error) {
	if s == "" || s == "none" {
		return pointer.NewPointerSet(), nil
	}
	ps := pointer.NewPointerSet()
	for name := range strings.SplitSeq(s, ",") {
		p := pointer.ForName(strings.TrimSpace(name))
		if p == nil {
			return nil, fmt.Errorf("invalid pointer name: %s", name)
		}
		ps.Add(p)
	}
	return ps, nil
}
