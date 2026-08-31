package pixpack

import "slices"

// generatePalette walks the Bitmap pixels and returns the unique RGB
// values encountered, with 0xff00ff reserved as the first entry
// (transparency sentinel — TS PixPack.ts:114-133).
func generatePalette(img *Bitmap) []int32 {
	colors := []int32{0xff00ff}
	for j := range img.Width * img.Height {
		pos := j * 4
		red := int32(img.Data[pos+0])
		green := int32(img.Data[pos+1])
		blue := int32(img.Data[pos+2])
		rgb := (red << 16) | (green << 8) | blue
		if rgb == 0xff00ff {
			continue
		}
		if !slices.Contains(colors, rgb) {
			colors = append(colors, rgb)
		}
	}
	return colors
}

// GeneratePixelOrder returns 0 for row-major and 1 for column-major by
// counting colour transitions along each traversal of the cropped rectangle
// and preferring whichever has fewer.
//
// Ports TS PixPack.ts:8-60 @1d25566c. Engine-TS 8139461a replaced the old
// signed-delta sum with this transition count AND flipped the return polarity
// (`columnMajorScore < rowMajorScore ? 0 : 1` became
// `columnTransitions < rowTransitions ? 1 : 0`). Both halves changed: getting
// only one right yields plausible-looking but wrong output.
//
// The old asymmetric sampling (row pass stepping j += 4, column pass j++) is
// gone too; both traversals now visit every pixel of the rectangle, and the
// first pixel seeds the "previous" values without scoring a transition.
func GeneratePixelOrder(img *Bitmap, left, top, width, height int) int {
	rowTransitions := 0
	columnTransitions := 0
	previousRow := int32(-1)
	previousColumn := int32(-1)

	for i := range width * height {
		rowPos := (left + i%width + (top+i/width)*img.Width) * 4
		columnPos := (left + i/height + (top+i%height)*img.Width) * 4

		row := int32(img.Data[rowPos])<<16 | int32(img.Data[rowPos+1])<<8 | int32(img.Data[rowPos+2])
		column := int32(img.Data[columnPos])<<16 | int32(img.Data[columnPos+1])<<8 | int32(img.Data[columnPos+2])

		if i > 0 {
			if row != previousRow {
				rowTransitions++
			}
			if column != previousColumn {
				columnTransitions++
			}
		}
		previousRow = row
		previousColumn = column
	}

	if columnTransitions < rowTransitions {
		return 1
	}
	return 0
}
