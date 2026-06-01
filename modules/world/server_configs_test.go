package world

import (
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

// TestServerConfigsView_CategoryType pins the adapter's contract
// across the four sibling-pattern paths: nil server, nil registry,
// OOB, in-range.
func TestServerConfigsView_CategoryType_NilServer(t *testing.T) {
	c := serverConfigsView{s: nil}
	if got := c.CategoryType(0); got != nil {
		t.Errorf("CategoryType with nil server: got %v, want nil", got)
	}
}

func TestServerConfigsView_CategoryType_NilRegistry(t *testing.T) {
	c := serverConfigsView{s: &Server{categoryTypes: nil}}
	if got := c.CategoryType(0); got != nil {
		t.Errorf("CategoryType with nil registry: got %v, want nil", got)
	}
}

func TestServerConfigsView_CategoryType_OOB(t *testing.T) {
	s := &Server{
		categoryTypes: &objtype.CategoryTypeConfigs{
			Configs: []*objtype.CategoryType{
				objtype.NewCategoryType(0),
				objtype.NewCategoryType(1),
			},
		},
	}
	c := serverConfigsView{s: s}
	for _, id := range []int{-1, -2, 2, 100} {
		if got := c.CategoryType(id); got != nil {
			t.Errorf("CategoryType(%d) OOB: got %v, want nil", id, got)
		}
	}
}

func TestServerConfigsView_CategoryType_InRange(t *testing.T) {
	c0 := objtype.NewCategoryType(0)
	c0.DebugName = "zero"
	c1 := objtype.NewCategoryType(1)
	c1.DebugName = "one"
	s := &Server{
		categoryTypes: &objtype.CategoryTypeConfigs{
			Configs: []*objtype.CategoryType{c0, c1},
		},
	}
	c := serverConfigsView{s: s}
	if got := c.CategoryType(0); got != c0 {
		t.Errorf("CategoryType(0): got %v, want %v", got, c0)
	}
	if got := c.CategoryType(1); got != c1 {
		t.Errorf("CategoryType(1): got %v, want %v", got, c1)
	}
}
