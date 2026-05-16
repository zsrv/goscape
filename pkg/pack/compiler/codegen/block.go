package codegen

// Block is an ordered list of instructions identified by a Label. Mirrors
// TS Block (Block.ts).
type Block struct {
	Label        *Label
	Instructions []Instruction
}

// NewBlock returns a Block with no instructions and the given label.
func NewBlock(label *Label) *Block {
	return &Block{Label: label}
}

// Add appends an instruction to the block.
func (b *Block) Add(ins Instruction) {
	b.Instructions = append(b.Instructions, ins)
}
