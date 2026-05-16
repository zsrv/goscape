package codegen

// Block is an ordered list of instructions identified by a Label. Mirrors
// TS Block (Block.ts).
type Block struct {
	Label        *Label
	Instructions []Instruction
}

func NewBlock(label *Label) *Block {
	return &Block{Label: label}
}

func (b *Block) Add(ins Instruction) {
	b.Instructions = append(b.Instructions, ins)
}
