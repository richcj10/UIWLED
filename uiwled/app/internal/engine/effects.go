package engine

import (
	"math"
	"time"

	"github.com/richcj10/uiwled/internal/device"
)

// Every effect in this file has the same signature:
//
//	func(state *SegmentState, elapsed time.Duration, n int) []device.Color
//
// where n is the port count and the return is a 0-indexed slice of length n
// (the frame renderer 1-indexes it before pushing). Effect functions are
// pure: no I/O, no goroutines, no shared mutable state — they're safe to
// call from the ticker without additional locking.
//
// Effect IDs follow WLED numbering where possible so a future WLED web UI
// or third-party integration can address effects by their well-known ids.
const (
	FxSolid         = 0
	FxBlink         = 1
	FxBreathe       = 2
	FxWipe          = 3
	FxScan          = 4
	FxChase         = 5
	FxFade          = 6
	FxTheaterChase  = 7
	FxColorloop     = 8
	FxRainbow       = 9
	FxRainbowCycle  = 12
	FxRunning       = 15
	FxTwoDots       = 17
	FxSparkle       = 20
	FxStrobe        = 23
	FxPolice        = 26
	FxFireFlicker   = 27
	FxRainbowRunner = 29
	FxLoading       = 41
	FxChristmas     = 46
	FxRipple        = 50
	FxMeteor        = 65
	FxTwinkle       = 74
	FxFireworks     = 78
	FxHalloween     = 79
	FxRain          = 82
	FxSinelon       = 88
	FxBouncingBalls = 91
	FxPlasma        = 92
	FxComet         = 100
	FxPacifica      = 101
	FxSunrise       = 110
)

// EffectInfo is one row of the effect registry, JSON-serialised by
// /api/effects for the web UI and the HA integration's effect_list.
type EffectInfo struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// effectRegistry is the source of truth for what shows up in the effect
// picker. Effects registered here must have a corresponding case in
// renderEffect's dispatch switch; failing that they render as Solid.
var effectRegistry = []EffectInfo{
	{FxSolid, "Solid"},
	{FxBlink, "Blink"},
	{FxBreathe, "Breathe"},
	{FxWipe, "Wipe"},
	{FxScan, "Scan"},
	{FxChase, "Chase"},
	{FxFade, "Fade"},
	{FxTheaterChase, "Theater Chase"},
	{FxColorloop, "Colorloop"},
	{FxRainbow, "Rainbow"},
	{FxRainbowCycle, "Rainbow Cycle"},
	{FxRunning, "Running"},
	{FxTwoDots, "Two Dots"},
	{FxSparkle, "Sparkle"},
	{FxStrobe, "Strobe"},
	{FxPolice, "Police"},
	{FxFireFlicker, "Fire Flicker"},
	{FxRainbowRunner, "Rainbow Runner"},
	{FxLoading, "Loading"},
	{FxChristmas, "Christmas"},
	{FxRipple, "Ripple"},
	{FxMeteor, "Meteor"},
	{FxTwinkle, "Twinkle"},
	{FxFireworks, "Fireworks"},
	{FxHalloween, "Halloween"},
	{FxRain, "Rain"},
	{FxSinelon, "Sinelon"},
	{FxBouncingBalls, "Bouncing Balls"},
	{FxPlasma, "Plasma"},
	{FxComet, "Comet"},
	{FxPacifica, "Pacifica"},
	{FxSunrise, "Sunrise"},
}

// Effects returns the registered effect list. Read-only — the returned
// slice is the internal registry, not a copy.
func Effects() []EffectInfo { return effectRegistry }

// renderEffect dispatches to the correct effect function based on state.EffectID.
// All effects return a 0-indexed slice of length `n` (port count). The caller
// shifts to the switch's 1-indexed frame buffer.
func renderEffect(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	if !state.On || n <= 0 {
		return make([]device.Color, n)
	}
	switch state.EffectID {
	case FxBlink:
		return fxBlink(state, elapsed, n)
	case FxBreathe:
		return fxBreathe(state, elapsed, n)
	case FxWipe:
		return fxWipe(state, elapsed, n)
	case FxScan:
		return fxScan(state, elapsed, n)
	case FxChase:
		return fxChase(state, elapsed, n)
	case FxFade:
		return fxFade(state, elapsed, n)
	case FxTheaterChase:
		return fxTheaterChase(state, elapsed, n)
	case FxColorloop:
		return fxColorloop(state, elapsed, n)
	case FxRainbow:
		return fxRainbow(state, elapsed, n)
	case FxRainbowCycle:
		return fxRainbowCycle(state, elapsed, n)
	case FxRunning:
		return fxRunning(state, elapsed, n)
	case FxSparkle:
		return fxSparkle(state, elapsed, n)
	case FxStrobe:
		return fxStrobe(state, elapsed, n)
	case FxPolice:
		return fxPolice(state, elapsed, n)
	case FxFireFlicker:
		return fxFireFlicker(state, elapsed, n)
	case FxRainbowRunner:
		return fxRainbowRunner(state, elapsed, n)
	case FxMeteor:
		return fxMeteor(state, elapsed, n)
	case FxTwinkle:
		return fxTwinkle(state, elapsed, n)
	case FxHalloween:
		return fxHalloween(state, elapsed, n)
	case FxSinelon:
		return fxSinelon(state, elapsed, n)
	case FxPlasma:
		return fxPlasma(state, elapsed, n)
	case FxTwoDots:
		return fxTwoDots(state, elapsed, n)
	case FxLoading:
		return fxLoading(state, elapsed, n)
	case FxChristmas:
		return fxChristmas(state, elapsed, n)
	case FxRipple:
		return fxRipple(state, elapsed, n)
	case FxFireworks:
		return fxFireworks(state, elapsed, n)
	case FxRain:
		return fxRain(state, elapsed, n)
	case FxBouncingBalls:
		return fxBouncingBalls(state, elapsed, n)
	case FxComet:
		return fxComet(state, elapsed, n)
	case FxPacifica:
		return fxPacifica(state, elapsed, n)
	case FxSunrise:
		return fxSunrise(state, elapsed, n)
	default:
		return fxSolid(state, n)
	}
}

// -- helpers --

// speedToPeriod converts WLED speed (0-255) into an animation cycle period.
// Higher speed = shorter period. Range: 0 → 8s per cycle, 255 → 0.2s per cycle.
func speedToPeriod(speed uint8) time.Duration {
	// linear interp between 8s and 0.2s
	ms := 8000 - (int(speed)*7800)/255
	return time.Duration(ms) * time.Millisecond
}

func scaleColor(c device.Color, brightness uint8) device.Color {
	return device.Color{
		R: uint8(int(c.R) * int(brightness) / 255),
		G: uint8(int(c.G) * int(brightness) / 255),
		B: uint8(int(c.B) * int(brightness) / 255),
	}
}

func fillAll(n int, c device.Color) []device.Color {
	out := make([]device.Color, n)
	for i := range out {
		out[i] = c
	}
	return out
}

// hsvToRGB converts hue [0..360), saturation and value [0..1] to RGB.
func hsvToRGB(h, s, v float64) device.Color {
	if s == 0 {
		x := uint8(v * 255)
		return device.Color{R: x, G: x, B: x}
	}
	hh := math.Mod(h, 360) / 60
	i := int(hh)
	f := hh - float64(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float64
	switch i {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return device.Color{R: uint8(r * 255), G: uint8(g * 255), B: uint8(b * 255)}
}

// blend mixes two colors: `a` at weight (1-t), `b` at weight t (t in [0..1]).
func blend(a, b device.Color, t float64) device.Color {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return device.Color{
		R: uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		G: uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		B: uint8(float64(a.B)*(1-t) + float64(b.B)*t),
	}
}

// -- effect implementations --

func fxSolid(state *SegmentState, n int) []device.Color {
	return fillAll(n, scaleColor(state.Colors[0], state.Brightness))
}

func fxBlink(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	on := (elapsed % period) < period/2
	if !on {
		return make([]device.Color, n)
	}
	return fillAll(n, scaleColor(state.Colors[0], state.Brightness))
}

func fxBreathe(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 2 // slower feels more "breath"
	phase := float64(elapsed%period) / float64(period) // 0..1
	// smooth sinusoidal: 0 → dim, 0.5 → bright, 1 → dim
	brightness := (math.Sin(phase*2*math.Pi-math.Pi/2) + 1) / 2
	c := scaleColor(state.Colors[0], uint8(float64(state.Brightness)*brightness))
	return fillAll(n, c)
}

func fxWipe(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := float64(elapsed%period) / float64(period)
	head := int(phase * float64(n*2))
	primary := scaleColor(state.Colors[0], state.Brightness)
	secondary := scaleColor(state.Colors[1], state.Brightness)
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		if head < n {
			if i <= head {
				out[i] = primary
			} else {
				out[i] = secondary
			}
		} else {
			// second half: wipe secondary back to primary
			back := head - n
			if i <= back {
				out[i] = secondary
			} else {
				out[i] = primary
			}
		}
	}
	return out
}

func fxChase(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := float64(elapsed%period) / float64(period)
	head := int(phase * float64(n))
	// Intensity controls tail length (0 = single dot, 255 = long tail).
	tail := 1 + int(state.Intensity)*n/(2*255)
	primary := scaleColor(state.Colors[0], state.Brightness)
	bg := scaleColor(state.Colors[1], state.Brightness/8) // dim background
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		out[i] = bg
	}
	for t := 0; t <= tail; t++ {
		idx := (head - t + n) % n
		fade := 1.0 - float64(t)/float64(tail+1)
		out[idx] = blend(bg, primary, fade)
	}
	return out
}

func fxFade(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 2
	phase := float64(elapsed%period) / float64(period)
	t := (math.Sin(phase*2*math.Pi) + 1) / 2 // 0..1..0
	primary := scaleColor(state.Colors[0], state.Brightness)
	secondary := scaleColor(state.Colors[1], state.Brightness)
	return fillAll(n, blend(primary, secondary, t))
}

func fxColorloop(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 4
	phase := float64(elapsed%period) / float64(period)
	c := hsvToRGB(phase*360, 1.0, float64(state.Brightness)/255)
	return fillAll(n, c)
}

func fxRainbow(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		hue := 360 * float64(i) / float64(n)
		out[i] = hsvToRGB(hue, 1.0, float64(state.Brightness)/255)
	}
	_ = elapsed
	return out
}

func fxRainbowCycle(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 4
	offsetDeg := 360 * float64(elapsed%period) / float64(period)
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		hue := offsetDeg + 360*float64(i)/float64(n)
		out[i] = hsvToRGB(hue, 1.0, float64(state.Brightness)/255)
	}
	return out
}

// fxScan: single bright dot bounces back and forth across the strip.
func fxScan(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := float64(elapsed%period) / float64(period)
	// triangle wave: 0..1..0
	pos := phase * 2
	if pos > 1 {
		pos = 2 - pos
	}
	head := int(pos * float64(n-1))
	primary := scaleColor(state.Colors[0], state.Brightness)
	bg := scaleColor(state.Colors[1], state.Brightness/16)
	out := fillAll(n, bg)
	if head >= 0 && head < n {
		out[head] = primary
	}
	return out
}

// fxTheaterChase: every third jack lit, pattern shifts. Marquee style.
func fxTheaterChase(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := int(float64(elapsed%period)/float64(period)*3) % 3
	primary := scaleColor(state.Colors[0], state.Brightness)
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		if (i+phase)%3 == 0 {
			out[i] = primary
		}
	}
	return out
}

// fxRunning: a short bright segment travels across the strip.
func fxRunning(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := float64(elapsed%period) / float64(period)
	head := int(phase * float64(n))
	segLen := 1 + int(state.Intensity)*n/(4*255) // 1..n/4
	primary := scaleColor(state.Colors[0], state.Brightness)
	bg := scaleColor(state.Colors[1], state.Brightness/16)
	out := fillAll(n, bg)
	for k := 0; k < segLen; k++ {
		out[(head+k)%n] = primary
	}
	return out
}

// fxSparkle: primary color everywhere, occasional bright white sparkle.
func fxSparkle(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	primary := scaleColor(state.Colors[0], state.Brightness)
	out := fillAll(n, primary)
	// Sparkle position/density from intensity + elapsed.
	density := 1 + int(state.Intensity)*n/(8*255)
	seed := uint32(elapsed.Milliseconds() / 60) // new pattern every 60ms
	for k := 0; k < density; k++ {
		seed = seed*1664525 + 1013904223
		out[int(seed)%n] = device.Color{R: 255, G: 255, B: 255}
	}
	return out
}

// fxStrobe: all jacks flash bright white on short pulses.
func fxStrobe(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := float64(elapsed%period) / float64(period)
	if phase < 0.06 { // short pulse
		return fillAll(n, scaleColor(device.Color{R: 255, G: 255, B: 255}, state.Brightness))
	}
	return make([]device.Color, n)
}

// fxPolice: red/blue alternating halves, swaps periodically.
func fxPolice(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := int(float64(elapsed%period)/float64(period)*2) % 2
	red := scaleColor(device.Color{R: 255}, state.Brightness)
	blue := scaleColor(device.Color{B: 255}, state.Brightness)
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		leftHalf := i < n/2
		if (leftHalf && phase == 0) || (!leftHalf && phase == 1) {
			out[i] = red
		} else {
			out[i] = blue
		}
	}
	return out
}

// fxFireFlicker: chaotic red-orange, per-jack independent flicker.
func fxFireFlicker(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	base := device.Color{R: 255, G: 100, B: 0}
	out := make([]device.Color, n)
	seed := uint32(elapsed.Milliseconds() / 80) // 12 fps flicker
	for i := 0; i < n; i++ {
		s := seed ^ uint32(i)*2654435761
		s = s*1664525 + 1013904223
		flicker := 128 + int(s%128) // 128..255
		c := scaleColor(base, uint8(flicker*int(state.Brightness)/255))
		out[i] = c
	}
	return out
}

// fxRainbowRunner: rainbow gradient scrolls across the strip.
func fxRainbowRunner(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 2
	offset := 360 * float64(elapsed%period) / float64(period)
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		hue := offset + 360*float64(i)/float64(n)*2
		out[i] = hsvToRGB(hue, 1.0, float64(state.Brightness)/255)
	}
	return out
}

// fxMeteor: bright head with fading tail, wraps around.
func fxMeteor(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := float64(elapsed%period) / float64(period)
	head := int(phase * float64(n))
	tailLen := 1 + int(state.Intensity)*n/(2*255)
	primary := scaleColor(state.Colors[0], state.Brightness)
	out := make([]device.Color, n)
	for t := 0; t <= tailLen; t++ {
		idx := (head - t + n) % n
		fade := 1.0 - float64(t)/float64(tailLen+1)
		out[idx] = scaleColor(primary, uint8(fade*255))
	}
	return out
}

// fxHalloween: orange, purple, green mix — spooky palette.
func fxHalloween(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 3
	shift := int(float64(elapsed%period) / float64(period) * 3)
	palette := []device.Color{
		scaleColor(device.Color{R: 255, G: 100, B: 0}, state.Brightness), // orange
		scaleColor(device.Color{R: 128, G: 0, B: 200}, state.Brightness), // purple
		scaleColor(device.Color{R: 0, G: 200, B: 40}, state.Brightness),  // green
	}
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		out[i] = palette[(i+shift)%3]
	}
	return out
}

// fxSinelon: single dot follows a sine wave position along the strip.
func fxSinelon(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 2
	phase := float64(elapsed%period) / float64(period)
	pos := (math.Sin(phase*2*math.Pi) + 1) / 2 // 0..1
	head := int(pos * float64(n-1))
	primary := scaleColor(state.Colors[0], state.Brightness)
	bg := scaleColor(state.Colors[1], state.Brightness/24)
	out := fillAll(n, bg)
	if head >= 0 && head < n {
		out[head] = primary
	}
	return out
}

// fxPlasma: overlapping sine waves produce a color-shifting wave pattern.
func fxPlasma(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	t := float64(elapsed.Milliseconds()) / 1000
	speedScale := 0.3 + float64(state.Speed)/128
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		x := float64(i) / float64(n)
		v := math.Sin(x*10+t*speedScale) + math.Sin((x*6+t*speedScale*1.3)+math.Sin(t*0.5)*3)
		hue := math.Mod((v+2)*90, 360)
		out[i] = hsvToRGB(hue, 1.0, float64(state.Brightness)/255)
	}
	return out
}

// fxTwoDots: two dots move in opposite directions, colors 0 and 1.
func fxTwoDots(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := float64(elapsed%period) / float64(period)
	a := int(phase * float64(n))
	b := int((1-phase) * float64(n))
	primary := scaleColor(state.Colors[0], state.Brightness)
	secondary := scaleColor(state.Colors[1], state.Brightness)
	bg := scaleColor(state.Colors[2], state.Brightness/16)
	out := fillAll(n, bg)
	if a >= 0 && a < n {
		out[a] = primary
	}
	if b >= 0 && b < n {
		out[b] = secondary
	}
	return out
}

// fxLoading: a progress bar that fills, resets, and repeats.
func fxLoading(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 2
	phase := float64(elapsed%period) / float64(period)
	filled := int(phase * float64(n))
	primary := scaleColor(state.Colors[0], state.Brightness)
	bg := scaleColor(state.Colors[1], state.Brightness/16)
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		if i < filled {
			out[i] = primary
		} else {
			out[i] = bg
		}
	}
	return out
}

// fxChristmas: red/green alternating with occasional white sparkles.
func fxChristmas(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 2
	shift := int(float64(elapsed%period)/float64(period)*2) % 2
	red := scaleColor(device.Color{R: 220, G: 20, B: 30}, state.Brightness)
	green := scaleColor(device.Color{R: 20, G: 200, B: 40}, state.Brightness)
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		if (i+shift)%2 == 0 {
			out[i] = red
		} else {
			out[i] = green
		}
	}
	// Sparkles based on intensity.
	density := 1 + int(state.Intensity)*n/(20*255)
	seed := uint32(elapsed.Milliseconds() / 90)
	for k := 0; k < density; k++ {
		seed = seed*1664525 + 1013904223
		out[int(seed)%n] = device.Color{R: 255, G: 255, B: 255}
	}
	return out
}

// fxRipple: expanding "rings" from a fixed center. Simple 1D interpretation.
func fxRipple(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := float64(elapsed%period) / float64(period)
	center := n / 2
	radius := int(phase * float64(n))
	primary := scaleColor(state.Colors[0], state.Brightness)
	bg := scaleColor(state.Colors[1], state.Brightness/24)
	out := fillAll(n, bg)
	// Two symmetric expanding points from center.
	left := center - radius
	right := center + radius
	if left >= 0 && left < n {
		out[left] = primary
	}
	if right >= 0 && right < n {
		out[right] = primary
	}
	return out
}

// fxFireworks: rare bursts at random positions, quick fade to black.
func fxFireworks(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	out := make([]device.Color, n)
	// Each frame, per-jack chance to be "exploding" derived from a hashed clock bucket.
	burstDensity := 1 + int(state.Intensity)*n/(30*255)
	seed := uint32(elapsed.Milliseconds() / 60)
	for k := 0; k < burstDensity; k++ {
		seed = seed*1664525 + 1013904223
		idx := int(seed) % n
		// Vary hue by burst position.
		hue := math.Mod(float64(idx*47), 360)
		out[idx] = hsvToRGB(hue, 1.0, float64(state.Brightness)/255)
	}
	return out
}

// fxRain: random drops falling — bright pixels appear and dim over frames.
func fxRain(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	out := make([]device.Color, n)
	primary := scaleColor(state.Colors[0], state.Brightness)
	density := 1 + int(state.Intensity)*n/(15*255)
	// Overlay drops from a short trailing window.
	for age := 0; age < 4; age++ {
		bucket := uint32((elapsed.Milliseconds()/80 - int64(age)))
		seed := bucket * 2654435761
		for k := 0; k < density; k++ {
			seed = seed*1664525 + 1013904223
			idx := int(seed) % n
			decay := 1.0 - float64(age)/4.0
			out[idx] = scaleColor(primary, uint8(decay*255))
		}
	}
	return out
}

// fxBouncingBalls: 3 balls of the 3 palette colors, each with its own phase,
// following a |sin| trajectory (gravity feel) along the strip.
func fxBouncingBalls(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	out := make([]device.Color, n)
	t := float64(elapsed.Milliseconds()) / 1000
	speedScale := 0.5 + float64(state.Speed)/128
	for k := 0; k < 3; k++ {
		phase := t*speedScale + float64(k)*0.7
		// Absolute sine gives a bounce shape.
		pos := math.Abs(math.Sin(phase))
		idx := int(pos * float64(n-1))
		if idx >= 0 && idx < n {
			out[idx] = scaleColor(state.Colors[k%3], state.Brightness)
		}
	}
	return out
}

// fxComet: bright head with long fading tail; leaves a smoother trail than Chase.
func fxComet(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed)
	phase := float64(elapsed%period) / float64(period)
	head := int(phase * float64(n))
	// Long tail — 40-80% of strip.
	tailLen := n/2 + int(state.Intensity)*n/(4*255)
	if tailLen < 3 {
		tailLen = 3
	}
	primary := scaleColor(state.Colors[0], state.Brightness)
	out := make([]device.Color, n)
	for t := 0; t <= tailLen && t < n; t++ {
		idx := (head - t + n) % n
		fade := math.Pow(1.0-float64(t)/float64(tailLen+1), 2) // quadratic falloff
		out[idx] = scaleColor(primary, uint8(fade*255))
	}
	return out
}

// fxPacifica: layered slow blue/cyan waves — meant to evoke ocean.
func fxPacifica(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	t := float64(elapsed.Milliseconds()) / 1000
	speedScale := 0.15 + float64(state.Speed)/384 // slow by default
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		x := float64(i) / float64(n)
		wave1 := math.Sin(x*8 + t*speedScale)
		wave2 := math.Sin(x*5 + t*speedScale*1.4 + 1.7)
		wave3 := math.Sin(x*13 + t*speedScale*0.7)
		combined := (wave1 + wave2*0.6 + wave3*0.4 + 2) / 4 // 0..1
		// Blue-to-cyan-to-white palette based on combined.
		hue := 190 + combined*30 // 190..220 (cyan-blue range)
		sat := 1.0 - combined*0.7
		val := 0.3 + combined*0.7
		c := hsvToRGB(hue, sat, val)
		out[i] = scaleColor(c, state.Brightness)
	}
	return out
}

// fxSunrise: fade from black to sunrise palette across the strip.
func fxSunrise(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	period := speedToPeriod(state.Speed) * 4
	phase := float64(elapsed%period) / float64(period)
	// Ping-pong: 0→1 for sunrise, 1→0 for sunset.
	pos := phase * 2
	if pos > 1 {
		pos = 2 - pos
	}
	out := make([]device.Color, n)
	for i := 0; i < n; i++ {
		// Position along strip determines base hue (deep red → orange → yellow → warm white).
		x := float64(i) / float64(n-1)
		hue := 5 + x*50           // 5..55
		sat := 1.0 - x*0.4        // 1.0..0.6
		val := math.Min(pos+x*0.2, 1.0) * (float64(state.Brightness) / 255)
		out[i] = hsvToRGB(hue, sat, val)
	}
	return out
}

func fxTwinkle(state *SegmentState, elapsed time.Duration, n int) []device.Color {
	// Deterministic per-port twinkle: cycle offset per port hashed by index.
	period := speedToPeriod(state.Speed) * 3
	out := make([]device.Color, n)
	primary := scaleColor(state.Colors[0], state.Brightness)
	for i := 0; i < n; i++ {
		// pseudo-random phase per port (stable across frames)
		phaseOffset := time.Duration((i*2654435761)%int(period.Milliseconds())) * time.Millisecond
		pElapsed := (elapsed + phaseOffset) % period
		t := float64(pElapsed) / float64(period)
		// Intensity: higher = more ports lit at once (shorter dark window)
		threshold := 1.0 - float64(state.Intensity)/512.0 // 0.5..1
		if t < threshold {
			out[i] = device.Color{}
		} else {
			// short bright pulse in the tail portion of the cycle
			pulse := (t - threshold) / (1 - threshold)
			bright := math.Sin(pulse * math.Pi)
			out[i] = scaleColor(primary, uint8(bright*255))
		}
	}
	return out
}
