package server

import (
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// RenderQRToTerminal encodes content as a scannable QR code rendered with
// ANSI SGR color cells. Each QR module becomes two cells wide and one cell
// tall — roughly square given typical 2:1 terminal cell aspect ratios.
//
// Set pixels are drawn as black-on-white space pairs and unset pixels as
// white-on-white, so the result is dark-on-light regardless of the
// terminal's theme.  A quiet zone of 2 modules surrounds the code.
func RenderQRToTerminal(content string) (string, error) {
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return "", err
	}
	bm := cropQuietZone(q.Bitmap())

	const (
		quiet    = 2
		setCell  = "\x1b[40m  \x1b[0m"  // black background, 2 cells
		unsetCell = "\x1b[107m  \x1b[0m" // bright-white background, 2 cells
	)

	var b strings.Builder
	size := len(bm) + quiet*2

	// Top quiet zone
	for y := 0; y < quiet; y++ {
		for x := 0; x < size; x++ {
			b.WriteString(unsetCell)
		}
		b.WriteByte('\n')
	}
	// Data rows
	for y := 0; y < len(bm); y++ {
		// Left quiet
		for x := 0; x < quiet; x++ {
			b.WriteString(unsetCell)
		}
		for x := 0; x < len(bm[y]); x++ {
			if bm[y][x] {
				b.WriteString(setCell)
			} else {
				b.WriteString(unsetCell)
			}
		}
		// Right quiet
		for x := 0; x < quiet; x++ {
			b.WriteString(unsetCell)
		}
		b.WriteByte('\n')
	}
	// Bottom quiet zone
	for y := 0; y < quiet; y++ {
		for x := 0; x < size; x++ {
			b.WriteString(unsetCell)
		}
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// cropQuietZone removes any uniformly-false border rows/columns from the
// bitmap so we can apply a consistent quiet zone ourselves.
func cropQuietZone(bm [][]bool) [][]bool {
	if len(bm) == 0 {
		return bm
	}
	top := 0
	for top < len(bm) && rowIsBlank(bm[top]) {
		top++
	}
	bottom := len(bm)
	for bottom > top && rowIsBlank(bm[bottom-1]) {
		bottom--
	}
	if top >= bottom {
		return bm
	}
	cropped := bm[top:bottom]
	width := len(cropped[0])
	left := 0
	for left < width && colIsBlank(cropped, left) {
		left++
	}
	right := width
	for right > left && colIsBlank(cropped, right-1) {
		right--
	}
	out := make([][]bool, len(cropped))
	for i, row := range cropped {
		out[i] = row[left:right]
	}
	return out
}

func rowIsBlank(r []bool) bool {
	for _, v := range r {
		if v {
			return false
		}
	}
	return true
}

func colIsBlank(bm [][]bool, c int) bool {
	for _, r := range bm {
		if r[c] {
			return false
		}
	}
	return true
}
