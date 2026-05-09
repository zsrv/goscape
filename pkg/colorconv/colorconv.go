// Package colorconv ports LostCityRS/Engine-TS/src/util/ColorConversion.ts.
package colorconv

// RGB15_HSL16 maps each 15-bit RGB value to its HSL16 encoding.
var RGB15_HSL16 [32768]int32

func init() {
	for rgb := range 32768 {
		RGB15_HSL16[rgb] = int32(Rgb15toHsl16(rgb))
	}
}

func Hsl24to16(hue, saturation, lightness int) int {
	if lightness > 243 {
		saturation >>= 4
	} else if lightness > 217 {
		saturation >>= 3
	} else if lightness > 192 {
		saturation >>= 2
	} else if lightness > 179 {
		saturation >>= 1
	}
	return (((hue & 0xff) >> 2) << 10) + ((saturation >> 5) << 7) + (lightness >> 1)
}

func Rgb15to24(rgb int) int {
	r := (rgb >> 10) & 0x1f
	g := (rgb >> 5) & 0x1f
	b := rgb & 0x1f
	return ((r << 3) << 16) + ((g << 3) << 8) + (b << 3)
}

func Rgb15toHsl16(rgb int) int {
	r := (rgb >> 10) & 0x1f
	g := (rgb >> 5) & 0x1f
	b := rgb & 0x1f
	red := float64(r) / 31.0
	green := float64(g) / 31.0
	blue := float64(b) / 31.0
	return RgbToHsl(red, green, blue)
}

func Rgb24to15(rgb int) int {
	r := (rgb >> 16) & 0xff
	g := (rgb >> 8) & 0xff
	b := rgb & 0xff
	return ((r >> 3) << 10) + ((g >> 3) << 5) + (b >> 3)
}

func Rgb24toHsl16(rgb int) int {
	r := (rgb >> 16) & 0xff
	g := (rgb >> 8) & 0xff
	b := rgb & 0xff
	red := float64(r) / 256.0
	green := float64(g) / 256.0
	blue := float64(b) / 256.0
	return RgbToHsl(red, green, blue)
}

func RgbToHsl(red, green, blue float64) int {
	min := red
	if green < min {
		min = green
	}
	if blue < min {
		min = blue
	}

	max := red
	if green > max {
		max = green
	}
	if blue > max {
		max = blue
	}

	hNorm := 0.0
	sNorm := 0.0
	lNorm := (min + max) / 2.0

	if min != max {
		if lNorm < 0.5 {
			sNorm = (max - min) / (max + min)
		} else if lNorm >= 0.5 {
			sNorm = (max - min) / (2.0 - max - min)
		}

		if red == max {
			hNorm = (green - blue) / (max - min)
		} else if green == max {
			hNorm = (blue-red)/(max-min) + 2.0
		} else if blue == max {
			hNorm = (red-green)/(max-min) + 4.0
		}
	}

	hNorm /= 6.0

	hue := int(hNorm * 256.0)
	saturation := int(sNorm * 256.0)
	lightness := int(lNorm * 256.0)

	if saturation < 0 {
		saturation = 0
	} else if saturation > 255 {
		saturation = 255
	}

	if lightness < 0 {
		lightness = 0
	} else if lightness > 255 {
		lightness = 255
	}

	return Hsl24to16(hue, saturation, lightness)
}

func ReverseHsl(hsl int) []int {
	var possible []int
	for rgb := range 32768 {
		if int(RGB15_HSL16[rgb]) == hsl {
			possible = append(possible, rgb)
		}
	}
	return possible
}
