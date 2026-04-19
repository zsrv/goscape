package rotation

func Rotate(angle int, dimensionA int, dimensionB int) int {
	if angle&0x1 != 0 {
		return dimensionB
	}
	return dimensionA
}

func RotateFlags(angle int, blockAccessFlags int) int {
	if angle == 0 {
		return blockAccessFlags
	}
	return ((blockAccessFlags << angle) & 0xF) | (blockAccessFlags >> (4 - angle))
}
