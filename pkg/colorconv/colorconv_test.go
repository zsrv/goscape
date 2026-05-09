package colorconv

import (
	"slices"
	"testing"
)

// T1.1 — rgb24to15 golden values.
// TS: ((r>>3)<<10) + ((g>>3)<<5) + (b>>3)
func TestRgb24to15_Goldens(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0xFF0000, 0x7C00},
		{0x00FF00, 0x03E0},
		{0xFFFF00, 0x7FE0},
		{0x000000, 0x0000},
		{0xFFFFFF, 0x7FFF},
	}
	for _, tt := range tests {
		got := Rgb24to15(tt.input)
		if got != tt.want {
			t.Errorf("Rgb24to15(0x%06X) = 0x%04X, want 0x%04X", tt.input, got, tt.want)
		}
	}
}

// T1.2 — rgb15to24 golden values.
// TS: ((r<<3)<<16) + ((g<<3)<<8) + (b<<3)
func TestRgb15to24_Goldens(t *testing.T) {
	tests := []struct {
		input int
		want  int
	}{
		{0x7C00, 0xF80000},
		{0x03E0, 0x00F800},
		{0x7FFF, 0xF8F8F8},
	}
	for _, tt := range tests {
		got := Rgb15to24(tt.input)
		if got != tt.want {
			t.Errorf("Rgb15to24(0x%04X) = 0x%06X, want 0x%06X", tt.input, got, tt.want)
		}
	}
}

// T1.3 — hsl24to16 branch coverage.
// Algorithm (TS verbatim):
//
//	if (lightness > 243) saturation >>= 4;
//	else if (lightness > 217) saturation >>= 3;
//	else if (lightness > 192) saturation >>= 2;
//	else if (lightness > 179) saturation >>= 1;
//	return (((hue & 0xff) >> 2) << 10) + ((saturation >> 5) << 7) + (lightness >> 1);
//
// All cases: hue=128, saturation=128.
// base hue term = ((128 & 0xff) >> 2) << 10 = (32 << 10) = 32768.
//
//   - L=244 (>243): sat=128>>4=8;  (8>>5)<<7=0;  244>>1=122 → 32768+0+122=32890
//   - L=218 (>217): sat=128>>3=16; (16>>5)<<7=0; 218>>1=109 → 32768+0+109=32877
//   - L=193 (>192): sat=128>>2=32; (32>>5)<<7=128; 193>>1=96 → 32768+128+96=32992
//   - L=180 (>179): sat=128>>1=64; (64>>5)<<7=256; 180>>1=90 → 32768+256+90=33114
//   - L=100 (default): sat=128; (128>>5)<<7=512; 100>>1=50 → 32768+512+50=33330
func TestHsl24to16_BranchTable(t *testing.T) {
	const hue, sat = 128, 128
	tests := []struct {
		lightness int
		want      int
	}{
		{244, 32890}, // sat >>= 4
		{218, 32877}, // sat >>= 3
		{193, 32992}, // sat >>= 2
		{180, 33114}, // sat >>= 1
		{100, 33330}, // no shift (default)
	}
	for _, tt := range tests {
		got := Hsl24to16(hue, sat, tt.lightness)
		if got != tt.want {
			t.Errorf("Hsl24to16(%d, %d, %d) = %d, want %d", hue, sat, tt.lightness, got, tt.want)
		}
	}
}

// T1.4 — RgbToHsl golden values (full TS rgbToHsl + hsl24to16 pipeline).
//
// Computed by executing the TS algorithm exactly:
//
//   - (1.0, 0.0, 0.0) pure red:
//     min=0,max=1,lNorm=0.5; sNorm=1/(2-1-0)=1.0; hNorm=(0-0)/1=0; hNorm/6=0;
//     hue=0, sat=256→255, light=128; hsl24to16(0,255,128): no shift;
//     0 + (255>>5)<<7=896 + 64 = 960
//
//   - (0.0, 1.0, 0.0) pure green:
//     lNorm=0.5; sNorm=1.0; hNorm=(0-0)/1+2=2; hNorm/6=0.3333;
//     hue=int(0.3333*256)=85, sat=255, light=128; hsl24to16(85,255,128): no shift;
//     (85>>2)<<10=21*1024=21504 + 896 + 64 = 22464
//
//   - (0.0, 0.0, 1.0) pure blue:
//     lNorm=0.5; sNorm=1.0; hNorm=(0-0)/1+4=4; hNorm/6=0.6666;
//     hue=int(0.6666*256)=170, sat=255, light=128; hsl24to16(170,255,128): no shift;
//     (170>>2)<<10=42*1024=43008 + 896 + 64 = 43968
//
//   - (0.5, 0.5, 0.5) achromatic:
//     min=max=0.5; hNorm=sNorm=0; lNorm=0.5; light=128;
//     hsl24to16(0,0,128): 0+0+64=64
//
//   - (1.0, 0.0, 0.5) negative-hue (red==max, green<blue):
//     min=0,max=1,lNorm=0.5; sNorm=1/(2-1-0)=1.0;
//     hNorm=(0-0.5)/1=-0.5; hNorm/6=-0.08333; hue=int(-21.333)=-21;
//     sat=255, light=int(0.5*256)=128;
//     hsl24to16(-21,255,128): 128 not>179 → no shift;
//     ((-21&0xff)>>2)<<10=(235>>2)<<10=58<<10=59392 + 896 + 64 = 60352
func TestRgbToHsl_Goldens(t *testing.T) {
	tests := []struct {
		name        string
		red         float64
		green       float64
		blue        float64
		want        int
	}{
		{"pure red", 1.0, 0.0, 0.0, 960},
		{"pure green", 0.0, 1.0, 0.0, 22464},
		{"pure blue", 0.0, 0.0, 1.0, 43968},
		{"achromatic", 0.5, 0.5, 0.5, 64},
		{"negative hue (red=max, green<blue)", 1.0, 0.0, 0.5, 60352},
	}
	for _, tt := range tests {
		got := RgbToHsl(tt.red, tt.green, tt.blue)
		if got != tt.want {
			t.Errorf("RgbToHsl(%v, %v, %v) [%s] = %d, want %d",
				tt.red, tt.green, tt.blue, tt.name, got, tt.want)
		}
	}
}

// T1.5 — Rgb24toHsl16 delegates to RgbToHsl.
// Sentinel: 0xFF8040 → r=255,g=128,b=64 → red=255/256.0, green=128/256.0, blue=64/256.0.
func TestRgb24toHsl16_DelegatesToRgbToHsl(t *testing.T) {
	const rgb24 = 0xFF8040
	r := float64(0xFF) / 256.0
	g := float64(0x80) / 256.0
	b := float64(0x40) / 256.0
	want := RgbToHsl(r, g, b)
	got := Rgb24toHsl16(rgb24)
	if got != want {
		t.Errorf("Rgb24toHsl16(0x%06X) = %d, want %d (RgbToHsl(%v,%v,%v))",
			rgb24, got, want, r, g, b)
	}
}

// T1.6 — Rgb15toHsl16 delegates to RgbToHsl.
// Sentinel: 0x7FFF (all 5-bit channels = 31) → red=green=blue=31/31=1.0.
func TestRgb15toHsl16_DelegatesToRgbToHsl(t *testing.T) {
	const rgb15 = 0x7FFF
	want := RgbToHsl(31.0/31.0, 31.0/31.0, 31.0/31.0)
	got := Rgb15toHsl16(rgb15)
	if got != want {
		t.Errorf("Rgb15toHsl16(0x%04X) = %d, want %d", rgb15, got, want)
	}
}

// T1.7 — RGB15_HSL16 table is populated by init.
func TestRGB15HSL16_PopulatedByInit(t *testing.T) {
	if len(RGB15_HSL16) != 32768 {
		t.Fatalf("len(RGB15_HSL16) = %d, want 32768", len(RGB15_HSL16))
	}
	want0 := int32(RgbToHsl(0, 0, 0))
	if RGB15_HSL16[0] != want0 {
		t.Errorf("RGB15_HSL16[0] = %d, want %d", RGB15_HSL16[0], want0)
	}
	want32767 := int32(RgbToHsl(31.0/31.0, 31.0/31.0, 31.0/31.0))
	if RGB15_HSL16[32767] != want32767 {
		t.Errorf("RGB15_HSL16[32767] = %d, want %d", RGB15_HSL16[32767], want32767)
	}
}

// T1.8 — ReverseHsl returns all rgb15 indexes whose HSL16 matches the query.
// Uses two known values from the populated table.
func TestReverseHsl_RoundTripFromTable(t *testing.T) {
	h0 := int(RGB15_HSL16[0])
	if got := ReverseHsl(h0); !slices.Contains(got, 0) {
		t.Errorf("ReverseHsl(%d) does not contain 0; got %v", h0, got)
	}

	h1000 := int(RGB15_HSL16[1000])
	if got := ReverseHsl(h1000); !slices.Contains(got, 1000) {
		t.Errorf("ReverseHsl(%d) does not contain 1000; got %v", h1000, got)
	}
}
