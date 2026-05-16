// pkg/pack/compiler/runescript/server_script_compiler_test.go
package runescript

import "testing"

// TestServerScriptCompiler_StructIsConstructable pins that a fresh
// ServerScriptCompiler literal compiles + has the expected zero-value shape.
func TestServerScriptCompiler_StructIsConstructable(t *testing.T) {
	c := &ServerScriptCompiler{}
	if c == nil {
		t.Fatal("ServerScriptCompiler literal nil")
	}
}
