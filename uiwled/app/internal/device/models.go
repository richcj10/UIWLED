package device

// modelLayouts maps the model string returned by `mca-cli-op info` to a 2D
// visual jack layout. Layout[row][col] is the 1-based physical port index
// (matching how UniFi numbers ports and how /proc/led/led_color addresses
// them). Used by both the web UI (to draw the jack grid) and the engine
// (to know how many jacks each switch has).
//
// The 48-port model interleaves odd/even ports across two rows to match the
// physical faceplate: row 1 is ports 1, 3, 5, ..., 47, row 2 is 2, 4, ..., 48.
//
// Layouts originally ported from robherley/etherlighter.
var modelLayouts = map[string][][]int{
	"USW-Pro-Max-16-PoE": {toRange(1, 16, 1)},
	"USW-Pro-Max-16":     {toRange(1, 16, 1)},
	"USW-Pro-Max-24-PoE": {toRange(1, 24, 1)},
	"USW-Pro-Max-24":     {toRange(1, 24, 1)},
	"USW-Pro-Max-48-PoE": {toRange(1, 47, 2), toRange(2, 48, 2)},
	"USW-Pro-Max-48":     {toRange(1, 47, 2), toRange(2, 48, 2)},
}

// LayoutFor returns the jack layout for the given UniFi model, or nil if
// the model isn't in the table. Nil means the engine will skip rendering.
func LayoutFor(model string) [][]int {
	if l, ok := modelLayouts[model]; ok {
		return l
	}
	return nil
}

// toRange returns [start, start+skip, start+2*skip, ..., end] inclusive.
func toRange(start, end, skip int) []int {
	r := make([]int, 0, (end-start)/skip+1)
	for i := start; i <= end; i += skip {
		r = append(r, i)
	}
	return r
}
