package pixpack

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
		seen := false
		for _, c := range colors {
			if c == rgb {
				seen = true
				break
			}
		}
		if !seen {
			colors = append(colors, rgb)
		}
	}
	return colors
}

// GeneratePixelOrder returns 1 for row-major and 0 for column-major
// based on cumulative absolute RGB-delta minimization (TS PixPack.ts:8-32).
//
// Note: TS iterates j += 4 in the row-major pass (skipping 3 of every
// 4 pixels) but j++ in the column-major pass. This asymmetry is
// preserved verbatim for byte-faithful score computation.
func GeneratePixelOrder(img *Bitmap) int {
	rowMajorScore := int64(0)
	columnMajorScore := int64(0)

	prev := int64(0)
	for j := 0; j < img.Width*img.Height; j += 4 {
		pos := j * 4
		current := int64(img.Data[pos+0])<<16 | int64(img.Data[pos+1])<<8 | int64(img.Data[pos+2])
		rowMajorScore += current - prev
		prev = current
	}

	prev = 0
	for x := range img.Width {
		for y := range img.Height {
			pos := (x + y*img.Width) * 4
			current := int64(img.Data[pos+0])<<16 | int64(img.Data[pos+1])<<8 | int64(img.Data[pos+2])
			columnMajorScore += current - prev
			prev = current
		}
	}

	if columnMajorScore < rowMajorScore {
		return 0
	}
	return 1
}
