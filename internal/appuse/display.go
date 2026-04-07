package appuse

import (
	"application-use/internal/cgo/macos/appuse_bridge"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type DisplayElement struct {
	IsAX   bool
	IsIcon bool
	Hint   string
	Name   string
	Frame  Rect
	// AX specific
	Role                string
	Label               string
	Identifier          string
	Description         string
	Help                string
	OriginalFingerprint string
}

func flattenAXElements(nodes []ElementNode) []ElementNode {
	var flat []ElementNode
	for _, node := range nodes {
		if node.Hint != "" {
			flat = append(flat, node)
		}
		flat = append(flat, flattenAXElements(node.Children)...)
	}
	return flat
}

// PrintSnapshot parses the JSON from the bridge and prints it in a hierarchical format.
// It returns the list of identified UI areas for caching.
func PrintSnapshot(jsonOutput string, debug bool) []AreaNode {
	var resp SnapshotResponse
	err := json.Unmarshal([]byte(jsonOutput), &resp)
	if err != nil {
		fmt.Printf("Error parsing snapshot: %v\n", err)
		return nil
	}

	fmt.Printf("Frontmost App: %s\n", resp.AppName)

	// Build fingerprint map from previous snapshot for diffing
	prevMap := make(map[string]bool)
	if prev := GetLatestSnapshotFromCache(resp.BundleID); prev != nil {
		// Store AX fingerprints
		var collectAX func([]ElementNode)
		collectAX = func(nodes []ElementNode) {
			for _, n := range nodes {
				prevMap[getFingerprint(n)] = true
				collectAX(n.Children)
			}
		}
		collectAX(prev.Elements)
		// Store OCR fingerprints
		for _, o := range prev.OCRElements {
			prevMap[getOCRFingerprint(o)] = true
		}
	}

	// Load screenshot for analysis
	const screenshotPath = "/tmp/application-use-current.png"
	file, err := os.Open(screenshotPath)
	if err != nil {
		fmt.Printf("Error opening screenshot: %v\n", err)
		return nil
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	if err != nil {
		fmt.Printf("Error decoding screenshot: %v\n", err)
		return nil
	}
	imgBounds := img.Bounds()

	var wx, wy, ww, wh float64
	if win := resp.FrontmostWindow; win != nil && win.Frame != nil {
		fmt.Println("--- Frontmost Window ---")
		wx, wy, ww, wh = win.Frame.X, win.Frame.Y, win.Frame.Width, win.Frame.Height
		fmt.Printf("  Title: %s\n", win.Title)
		fmt.Printf("  Frame: (%.0f, %.0f, %.0f, %.0f)\n", wx, wy, ww, wh)
	}

	// Calculate scale factors (logical points to image pixels)
	scaleX, scaleY := 1.0, 1.0
	if ww > 0 {
		scaleX = float64(imgBounds.Dx()) / ww
	}
	if wh > 0 {
		scaleY = float64(imgBounds.Dy()) / wh
	}

	// Unify into DisplayElement (Convert to pixels upfront)
	flatAX := flattenAXElements(resp.Elements)
	survivingOCR := resp.OCRElements
	var displayElements []DisplayElement

	for _, ax := range flatAX {
		// Filter elements outside window bounds (allowing 1pt tolerance for rounding)
		if ax.Frame.X < wx-1 || ax.Frame.Y < wy-1 ||
			ax.Frame.X+ax.Frame.Width > wx+ww+1 ||
			ax.Frame.Y+ax.Frame.Height > wy+wh+1 {
			continue
		}

		name := ax.Title
		if name == "" {
			name = "unnamed"
		}
		// Convert screen points to window-relative pixels
		px := (ax.Frame.X - wx) * scaleX
		py := (ax.Frame.Y - wy) * scaleY
		pw := ax.Frame.Width * scaleX
		ph := ax.Frame.Height * scaleY

		de := DisplayElement{
			IsAX:                true,
			Hint:                ax.Hint,
			Name:                name,
			Role:                ax.Role,
			Label:               ax.Label,
			Identifier:          ax.Identifier,
			Description:         ax.Description,
			Help:                ax.Help,
			OriginalFingerprint: getFingerprint(ax),
		}
		de.Frame.X, de.Frame.Y, de.Frame.Width, de.Frame.Height = px, py, pw, ph
		displayElements = append(displayElements, de)
	}

	for _, ocr := range survivingOCR {
		// Filter OCR elements outside window bounds
		if ocr.Frame.X < wx-1 || ocr.Frame.Y < wy-1 ||
			ocr.Frame.X+ocr.Frame.Width > wx+ww+1 ||
			ocr.Frame.Y+ocr.Frame.Height > wy+wh+1 {
			continue
		}

		// Convert screen points to window-relative pixels
		px := (ocr.Frame.X - wx) * scaleX
		py := (ocr.Frame.Y - wy) * scaleY
		pw := ocr.Frame.Width * scaleX
		ph := ocr.Frame.Height * scaleY

		de := DisplayElement{
			IsAX:                false,
			Hint:                ocr.Hint,
			Name:                ocr.Name,
			OriginalFingerprint: getOCRFingerprint(ocr),
		}
		de.Frame.X, de.Frame.Y, de.Frame.Width, de.Frame.Height = px, py, pw, ph
		displayElements = append(displayElements, de)
	}

	for _, icon := range resp.IconElements {
		// Filter icon elements outside window bounds
		if icon.Frame.X < wx-1 || icon.Frame.Y < wy-1 ||
			icon.Frame.X+icon.Frame.Width > wx+ww+1 ||
			icon.Frame.Y+icon.Frame.Height > wy+wh+1 {
			continue
		}

		// Convert screen points to window-relative pixels
		px := (icon.Frame.X - wx) * scaleX
		py := (icon.Frame.Y - wy) * scaleY
		pw := icon.Frame.Width * scaleX
		ph := icon.Frame.Height * scaleY

		de := DisplayElement{
			IsAX:                false,
			IsIcon:              true,
			Hint:                icon.Hint,
			Name:                fmt.Sprintf("Icon (%.0f%%)", icon.Confidence*100),
			OriginalFingerprint: getIconFingerprint(icon),
		}
		de.Frame.X, de.Frame.Y, de.Frame.Width, de.Frame.Height = px, py, pw, ph
		displayElements = append(displayElements, de)
	}

	var areas []AreaNode
	if len(displayElements) > 0 {
		areas = printAreaHierarchy(img, imgBounds, displayElements, wx, wy, ww, wh, scaleX, scaleY, prevMap, debug)
	}

	if resp.Caret != nil {
		fmt.Printf("\n[INPUT] Keyboard focus detected (Caret).\n")
		// Convert absolute caret to window-relative pixels
		cx := (resp.Caret.X - wx) * scaleX
		cy := (resp.Caret.Y - wy) * scaleY
		cw := resp.Caret.Width * scaleX
		ch := resp.Caret.Height * scaleY
		caretRect := Rect{X: cx, Y: cy, Width: cw, Height: ch}

		var bestElement *DisplayElement
		for i := range displayElements {
			if contains(displayElements[i].Frame, caretRect) {
				if bestElement == nil || (displayElements[i].Frame.Width*displayElements[i].Frame.Height < bestElement.Frame.Width*bestElement.Frame.Height) {
					bestElement = &displayElements[i]
				}
			}
		}

		var bestArea *AreaNode
		for i := range areas {
			af := areas[i].Frame
			if resp.Caret.X >= af.X-0.5 && resp.Caret.Y >= af.Y-0.5 &&
				resp.Caret.X+resp.Caret.Width <= af.X+af.Width+0.5 &&
				resp.Caret.Y+resp.Caret.Height <= af.Y+af.Height+0.5 {
				if bestArea == nil || (af.Width*af.Height < bestArea.Frame.Width*bestArea.Frame.Height) {
					bestArea = &areas[i]
				}
			}
		}

		if bestElement != nil {
			cleanName := strings.Replace(bestElement.Name, " (via OCR)", "", 1)
			fmt.Printf("   - Element: '%s' [%s] (Role: %s)\n", cleanName, bestElement.Hint, bestElement.Role)
		}
		if bestArea != nil {
			fmt.Printf("   - Area: %s [%s]\n", bestArea.Name, bestArea.Hint)
		}
	}

	appuse_bridge.ShowOverlay()
	return areas
}

func getFingerprint(node ElementNode) string {
	return fmt.Sprintf("%s|%s|%s|%s", node.Role, node.Title, node.Identifier, node.Description)
}

func getOCRFingerprint(ocr OCRElement) string {
	return "OCR|" + ocr.Name
}

func getIconFingerprint(icon IconElement) string {
	return fmt.Sprintf("ICON|%.0f,%.0f,%.0f,%.0f", icon.Frame.X, icon.Frame.Y, icon.Frame.Width, icon.Frame.Height)
}

type displayTreeNode struct {
	e        DisplayElement
	children []*displayTreeNode
}

func buildElementTree(elements []DisplayElement) []*displayTreeNode {
	// 1. Sort by area descending (larger first). Tie-break with IsAX (parents are often functional containers)
	sort.Slice(elements, func(i, j int) bool {
		areaI := elements[i].Frame.Width * elements[i].Frame.Height
		areaJ := elements[j].Frame.Width * elements[j].Frame.Height
		if areaI != areaJ {
			return areaI > areaJ
		}
		if elements[i].IsAX != elements[j].IsAX {
			return elements[i].IsAX
		}
		return i < j
	})

	var roots []*displayTreeNode
	for _, e := range elements {
		node := &displayTreeNode{e: e}
		if !insertNode(roots, node) {
			roots = append(roots, node)
		}
	}
	sortTreeNodes(roots)
	return roots
}

func insertNode(candidates []*displayTreeNode, node *displayTreeNode) bool {
	// Try to find the smallest candidate that contains this node
	// Candidates are already sorted by area descending, so the first one that fits is the "most likely" parent
	// However, we actually want the *smallest* container, so we should look at existing containers' children too.
	for _, c := range candidates {
		if contains(c.e.Frame, node.e.Frame) && c.e.Hint != node.e.Hint {
			// Found a container. Check if it fits in any of c's children
			if !insertNode(c.children, node) {
				c.children = append(c.children, node)
				sortTreeNodes(c.children)
			}
			return true
		}
	}
	return false
}

func contains(parent, child Rect) bool {
	return child.X >= parent.X-0.5 &&
		child.Y >= parent.Y-0.5 &&
		child.X+child.Width <= parent.X+parent.Width+0.5 &&
		child.Y+child.Height <= parent.Y+parent.Height+0.5
}

func sortTreeNodes(nodes []*displayTreeNode) {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].e.Frame.Y != nodes[j].e.Frame.Y {
			return nodes[i].e.Frame.Y < nodes[j].e.Frame.Y
		}
		return nodes[i].e.Frame.X < nodes[j].e.Frame.X
	})
}

func printAreaHierarchy(img image.Image, imgBounds image.Rectangle, elements []DisplayElement, winX, winY, winW, winH, scaleX, scaleY float64, prevMap map[string]bool, debug bool) []AreaNode {
	fmt.Println("--- UI Areas & Elements ---")
	var areas []AreaNode
	areaCount := 0

	getHint := func() string {
		hint := string('a' + rune(areaCount%26))
		if areaCount >= 26 {
			hint = string('a'+rune(areaCount/26-1)) + string('a'+rune(areaCount%26))
		}
		areaCount++
		return hint
	}

	var printElementNode func(*displayTreeNode, string, string)
	printElementNode = func(n *displayTreeNode, baseIndent, extraSpace string) {
		e := n.e
		ocrChar := " "
		if e.IsIcon {
			ocrChar = "#"
		} else if !e.IsAX || strings.Contains(e.Name, " (via OCR)") {
			ocrChar = "*"
		}
		diffChar := " "
		if len(prevMap) > 0 && !prevMap[e.OriginalFingerprint] {
			diffChar = "+"
		}

		hint := e.Hint
		if hint == "" {
			hint = "?"
		}

		cleanName := strings.Replace(e.Name, " (via OCR)", "", 1)
		// Back to logical points for presentation
		fmt.Printf("%s%s[%s] Role: %s, Name: '%s', Frame: (%.0f, %.0f, %.0f, %.0f)\n",
			baseIndent, ocrChar+diffChar+extraSpace, hint, e.Role, cleanName,
			e.Frame.X/scaleX, e.Frame.Y/scaleY, e.Frame.Width/scaleX, e.Frame.Height/scaleY)

		for _, child := range n.children {
			printElementNode(child, baseIndent+"   ", extraSpace)
		}
	}

	var debugImg draw.Image
	if debug {
		rgba := image.NewRGBA(imgBounds)
		draw.Draw(rgba, imgBounds, img, image.Point{}, draw.Src)
		debugImg = rgba

		// Draw bounding boxes for all elements
		green := color.RGBA{0, 255, 0, 255}
		yellow := color.RGBA{255, 255, 0, 255}
		red := color.RGBA{255, 0, 0, 255}
		for _, e := range elements {
			c := green
			if e.IsIcon {
				c = red
			} else if !e.IsAX {
				c = yellow
			}
			drawRect(debugImg, e.Frame, c)
		}
	}

	splitRegion := func(regionBounds image.Rectangle, regionalElements []DisplayElement, horizontal bool) (int, []Interval, []Interval) {
		// Pass threshold 10
		s := findSeparatorsInRegion(img, debugImg, regionalElements, regionBounds, horizontal, 10, debug)
		var min, max int
		if horizontal {
			min, max = regionBounds.Min.Y, regionBounds.Max.Y
		} else {
			min, max = regionBounds.Min.X, regionBounds.Max.X
		}
		intervals := splitToIntervals(min, max, s)

		meaningfulCount := 0
		for _, interval := range intervals {
			hasElement := false
			for _, e := range regionalElements {
				var overlap float64
				if horizontal {
					overlap = math.Max(0, math.Min(e.Frame.Y+e.Frame.Height, float64(interval.End))-math.Max(e.Frame.Y, float64(interval.Start)))
				} else {
					overlap = math.Max(0, math.Min(e.Frame.X+e.Frame.Width, float64(interval.End))-math.Max(e.Frame.X, float64(interval.Start)))
				}
				if overlap > 0 {
					hasElement = true
					break
				}
			}
			if hasElement {
				meaningfulCount++
			}
		}
		// Return ALL intervals, not just meaningful ones
		return meaningfulCount, intervals, s
	}

	hCount, hIntervals, hSeps := splitRegion(imgBounds, elements, true)
	vCount, vIntervals, vSeps := splitRegion(imgBounds, elements, false)

	if debug && debugImg != nil {
		red := color.RGBA{255, 0, 0, 255}
		// Draw Level 1 separator blocks (Horizontal)
		for _, sep := range hSeps {
			for y := sep.Start; y <= sep.End; y++ {
				if y >= imgBounds.Min.Y && y < imgBounds.Max.Y {
					for x := imgBounds.Min.X; x < imgBounds.Max.X; x++ {
						debugImg.Set(x, y, red)
					}
				}
			}
		}

		// Draw Level 1 separator blocks (Vertical)
		blue := color.RGBA{0, 0, 255, 255}
		for _, sep := range vSeps {
			for x := sep.Start; x <= sep.End; x++ {
				if x >= imgBounds.Min.X && x < imgBounds.Max.X {
					for y := imgBounds.Min.Y; y < imgBounds.Max.Y; y++ {
						debugImg.Set(x, y, blue)
					}
				}
			}
		}
	}

	// Dynamic split choice: pick the one with more meaningful areas.
	// Default to horizontal if equal or higher.
	horizontalSplit := true
	level1Intervals := hIntervals
	if vCount > hCount {
		horizontalSplit = false
		level1Intervals = vIntervals
	}

	var assignments map[int][]DisplayElement
	if horizontalSplit {
		assignments = assignElementsByY(elements, level1Intervals)
	} else {
		assignments = assignElementsByX(elements, level1Intervals)
	}

	l1AreaCounter := 0
	for i, l1Interval := range level1Intervals {
		l1Elements := assignments[i]

		l1Label := labelForIndex(l1AreaCounter, len(level1Intervals), horizontalSplit)
		l1AreaCounter++
		l1Hint := getHint()

		l1Y0 := float64(l1Interval.Start) / scaleY
		l1Y1 := float64(l1Interval.End) / scaleY
		l1X0 := float64(imgBounds.Min.X) / scaleX
		l1X1 := float64(imgBounds.Max.X) / scaleX
		if !horizontalSplit {
			l1X0, l1X1 = float64(l1Interval.Start)/scaleX, float64(l1Interval.End)/scaleX
			l1Y0, l1Y1 = float64(imgBounds.Min.Y)/scaleY, float64(imgBounds.Max.Y)/scaleY
		}

		fmt.Printf("%s [%s]: Frame (%.0f, %.0f, %.0f, %.0f)\n", l1Label+" Area", l1Hint, l1X0, l1Y0, l1X1, l1Y1)

		areas = append(areas, AreaNode{
			Name: l1Label + " Area",
			Hint: l1Hint,
			Frame: struct {
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
			}{
				X:      winX + l1X0,
				Y:      winY + l1Y0,
				Width:  l1X1 - l1X0,
				Height: l1Y1 - l1Y0,
			},
		})

		// LEVEL 2 PARTITIONING
		l2Horizontal := !horizontalSplit
		// Restrict bounds to current level 1 area
		l1SubBounds := imgBounds
		if horizontalSplit {
			l1SubBounds.Min.Y = l1Interval.Start
			l1SubBounds.Max.Y = l1Interval.End
		} else {
			l1SubBounds.Min.X = l1Interval.Start
			l1SubBounds.Max.X = l1Interval.End
		}

		l2Count, l2Intervals, _ := splitRegion(l1SubBounds, l1Elements, l2Horizontal)

		if l2Count > 1 {
			var l2Assignments map[int][]DisplayElement
			if l2Horizontal {
				l2Assignments = assignElementsByY(l1Elements, l2Intervals)
			} else {
				l2Assignments = assignElementsByX(l1Elements, l2Intervals)
			}

			for j, l2Interval := range l2Intervals {
				l2Elements := l2Assignments[j]

				if len(l2Elements) == 0 {
					continue
				}

				l2Label := labelForIndex(j, len(l2Intervals), l2Horizontal)
				l2Hint := getHint()

				l2Y0 := float64(l2Interval.Start) / scaleY
				l2Y1 := float64(l2Interval.End) / scaleY
				l2X0 := float64(l1SubBounds.Min.X) / scaleX
				l2X1 := float64(l1SubBounds.Max.X) / scaleX
				if !l2Horizontal {
					l2X0, l2X1 = float64(l2Interval.Start)/scaleX, float64(l2Interval.End)/scaleX
					l2Y0, l2Y1 = float64(l1SubBounds.Min.Y)/scaleY, float64(l1SubBounds.Max.Y)/scaleY
				}

				fmt.Printf("  %s [%s]: Frame (%.0f, %.0f, %.0f, %.0f)\n", l2Label+" Area", l2Hint, l2X0, l2Y0, l2X1, l2Y1)

				areas = append(areas, AreaNode{
					Name: l1Label + " " + l2Label + " Area",
					Hint: l2Hint,
					Frame: struct {
						X      float64 `json:"x"`
						Y      float64 `json:"y"`
						Width  float64 `json:"width"`
						Height float64 `json:"height"`
					}{
						X:      winX + l2X0,
						Y:      winY + l2Y0,
						Width:  l2X1 - l2X0,
						Height: l2Y1 - l2Y0,
					},
				})

				l2Tree := buildElementTree(l2Elements)
				for _, node := range l2Tree {
					printElementNode(node, "     ", " ")
				}
			}
		} else {
			// No meaningful sub-split, build and print tree for Level 1 elements directly.
			l1Tree := buildElementTree(l1Elements)
			for _, node := range l1Tree {
				printElementNode(node, "  ", " ")
			}
		}
	}

	if debug && debugImg != nil {
		outPath, _ := filepath.Abs("debug.png")
		f, err := os.Create(outPath)
		if err == nil {
			png.Encode(f, debugImg)
			f.Close()
			fmt.Printf("\n[DEBUG] Debug image saved to: %s\n", outPath)
		}
	}
	return areas
}

func labelType(horizontal bool) string {
	if horizontal {
		return "Row"
	}
	return "Column"
}

func labelForIndex(idx, total int, horizontal bool) string {
	if total == 1 {
		return "Main"
	}
	prefix := labelType(horizontal)
	return fmt.Sprintf("%s %d", prefix, idx+1)
}

type Interval struct {
	Start, End int
}

func colorsEqual(c1, c2 color.Color, debug bool, x, y int) bool {
	r1, g1, b1, a1 := c1.RGBA()
	r2, g2, b2, a2 := c2.RGBA()

	absDiff := func(v1, v2 uint32) uint32 {
		if v1 > v2 {
			return v1 - v2
		}
		return v2 - v1
	}

	const threshold = 2000 // User's experimental value
	dr, dg, db, da := absDiff(r1, r2), absDiff(g1, g2), absDiff(b1, b2), absDiff(a1, a2)
	match := dr <= threshold && dg <= threshold && db <= threshold && da <= threshold
	return match
}

func getRowX(img image.Image, y int, bounds image.Rectangle, debug bool) (int, int, int) {
	width := bounds.Dx()
	centerX := bounds.Min.X + (width / 2)
	centerColor := img.At(centerX, y)

	l, r := 0, 0
	for x := centerX - 1; x >= bounds.Min.X; x-- {
		if colorsEqual(img.At(x, y), centerColor, debug, x, y) {
			l++
		} else {
			break
		}
	}
	for x := centerX + 1; x < bounds.Max.X; x++ {
		if colorsEqual(img.At(x, y), centerColor, debug, x, y) {
			r++
		} else {
			break
		}
	}
	return l + r + 1, centerX - l, centerX + r
}

func getColX(img image.Image, x int, bounds image.Rectangle, debug bool) (int, int, int) {
	height := bounds.Dy()
	centerY := bounds.Min.Y + (height / 2)
	centerColor := img.At(x, centerY)
	u, d := 0, 0
	for y := centerY - 1; y >= bounds.Min.Y; y-- {
		if colorsEqual(img.At(x, y), centerColor, debug, x, y) {
			u++
		} else {
			break
		}
	}
	for y := centerY + 1; y < bounds.Max.Y; y++ {
		if colorsEqual(img.At(x, y), centerColor, debug, x, y) {
			d++
		} else {
			break
		}
	}
	return u + d + 1, centerY - u, centerY + d
}

func findSeparatorsInRegion(img image.Image, debugImg draw.Image, elements []DisplayElement, bounds image.Rectangle, horizontal bool, threshold int, debug bool) []Interval {
	var seps []Interval
	inSep := false
	start := 0
	limit := bounds.Max.Y
	total := bounds.Dx()
	loopStart := bounds.Min.Y
	if !horizontal {
		limit = bounds.Max.X
		total = bounds.Dy()
		loopStart = bounds.Min.X
	}

	for i := loopStart; i < limit; i++ {
		var xLen, startX, endX int
		if horizontal {
			xLen, startX, endX = getRowX(img, i, bounds, debug)
			if debug && debugImg != nil {
				cyan := color.RGBA{0, 255, 255, 128}
				for x := startX; x <= endX; x++ {
					if x >= bounds.Min.X && x < bounds.Max.X {
						debugImg.Set(x, i, cyan)
					}
				}
			}
		} else {
			xLen, startX, endX = getColX(img, i, bounds, debug)
			if debug && debugImg != nil {
				magenta := color.RGBA{255, 0, 255, 128}
				for y := startX; y <= endX; y++ {
					if y >= bounds.Min.Y && y < bounds.Max.Y {
						debugImg.Set(i, y, magenta)
					}
				}
			}
		}

		diff := total - xLen
		canDraw := diff < threshold

		if canDraw {
			if !inSep {
				inSep = true
				start = i
			}
		} else {
			if inSep {
				seps = append(seps, Interval{start, i - 1})
				inSep = false
			}
		}
	}
	if inSep {
		seps = append(seps, Interval{start, limit - 1})
	}
	return seps
}

func splitToIntervals(min, max int, seps []Interval) []Interval {
	var intervals []Interval
	current := min
	for _, sep := range seps {
		if sep.Start > current {
			intervals = append(intervals, Interval{current, sep.Start - 1})
		}
		current = sep.End + 1
	}
	if current < max {
		intervals = append(intervals, Interval{current, max})
	}
	return intervals
}

func isIntersecting(coord int, horizontal bool, elements []DisplayElement, debug bool) (bool, string) {
	fcoord := float64(coord)
	for _, e := range elements {
		var min, max float64
		if horizontal {
			min, max = e.Frame.Y, e.Frame.Y+e.Frame.Height
		} else {
			min, max = e.Frame.X, e.Frame.X+e.Frame.Width
		}
		if fcoord >= min-0.1 && fcoord <= max+0.1 {
			// If we want more detail, we can add it here, but it might be too spammy.
			// Let's only log if debug is on (we'd need to pass debug to this func).
			return true, e.Name
		}
	}
	return false, ""
}

func drawRect(img draw.Image, r Rect, c color.Color) {
	x0, y0 := int(r.X), int(r.Y)
	x1, y1 := int(r.X+r.Width), int(r.Y+r.Height)

	bounds := img.Bounds()

	// Draw horizontal lines
	for x := x0; x < x1; x++ {
		if x >= bounds.Min.X && x < bounds.Max.X {
			if y0 >= bounds.Min.Y && y0 < bounds.Max.Y {
				img.Set(x, y0, c)
			}
			if y1-1 >= bounds.Min.Y && y1-1 < bounds.Max.Y {
				img.Set(x, y1-1, c)
			}
		}
	}
	// Draw vertical lines
	for y := y0; y < y1; y++ {
		if y >= bounds.Min.Y && y < bounds.Max.Y {
			if x0 >= bounds.Min.X && x0 < bounds.Max.X {
				img.Set(x0, y, c)
			}
			if x1-1 >= bounds.Min.X && x1-1 < bounds.Max.X {
				img.Set(x1-1, y, c)
			}
		}
	}
}

func assignElementsByY(elements []DisplayElement, intervals []Interval) map[int][]DisplayElement {
	assignments := make(map[int][]DisplayElement)
	for _, e := range elements {
		bestIdx := -1
		maxOverlap := -1.0
		for i, interval := range intervals {
			overlap := math.Max(0, math.Min(e.Frame.Y+e.Frame.Height, float64(interval.End))-math.Max(e.Frame.Y, float64(interval.Start)))
			if overlap > maxOverlap {
				maxOverlap = overlap
				bestIdx = i
			}
		}
		if bestIdx != -1 && maxOverlap > 0 {
			assignments[bestIdx] = append(assignments[bestIdx], e)
		}
	}
	return assignments
}

func assignElementsByX(elements []DisplayElement, intervals []Interval) map[int][]DisplayElement {
	assignments := make(map[int][]DisplayElement)
	for _, e := range elements {
		bestIdx := -1
		maxOverlap := -1.0
		for i, interval := range intervals {
			overlap := math.Max(0, math.Min(e.Frame.X+e.Frame.Width, float64(interval.End))-math.Max(e.Frame.X, float64(interval.Start)))
			if overlap > maxOverlap {
				maxOverlap = overlap
				bestIdx = i
			}
		}
		if bestIdx != -1 && maxOverlap > 0 {
			assignments[bestIdx] = append(assignments[bestIdx], e)
		}
	}
	return assignments
}
