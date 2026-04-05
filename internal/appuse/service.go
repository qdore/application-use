package appuse

import (
	"application-use/internal/cgo/macos/appuse_bridge"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/text/unicode/norm"
)

var Version = "0.0.0-dev"

type ClickMode int

const (
	ClickLeft ClickMode = iota
	ClickRight
	ClickDouble
)

func TakeSnapshot() string {
	return appuse_bridge.TriggerSnapshot()
}

func ClearSnapshot() {
	appuse_bridge.ClearSnapshot()
}

func GetCachePath() string {
	return filepath.Join(os.TempDir(), ".application-use.cache")
}

func SaveCache(jsonOutput string, areas []AreaNode) {
	var newSnap SnapshotResponse
	if err := json.Unmarshal([]byte(jsonOutput), &newSnap); err != nil {
		fmt.Printf("Warning: failed to parse new snapshot for cache: %v\n", err)
		return
	}
	// Inject calculated areas
	newSnap.Areas = areas

	// 1. Load existing cache
	var snapshots []SnapshotResponse
	data, err := os.ReadFile(GetCachePath())
	if err == nil {
		// Support both old single-object format and new list format
		if err := json.Unmarshal(data, &snapshots); err != nil {
			var single SnapshotResponse
			if err := json.Unmarshal(data, &single); err == nil {
				snapshots = []SnapshotResponse{single}
			} else {
				snapshots = []SnapshotResponse{}
			}
		}
	}

	// 2. Remove existing entry for this app if it exists (to move to front)
	var filtered []SnapshotResponse
	for _, s := range snapshots {
		// Use BundleID for reliable comparison if available
		if (s.BundleID != "" && s.BundleID != newSnap.BundleID) ||
			(s.BundleID == "" && s.AppName != newSnap.AppName) {
			filtered = append(filtered, s)
		}
	}

	// 3. Add new snap to the front
	snapshots = append([]SnapshotResponse{newSnap}, filtered...)

	// 4. Limit to 5
	if len(snapshots) > 5 {
		snapshots = snapshots[:5]
	}

	// 5. Save
	newData, _ := json.MarshalIndent(snapshots, "", "  ")
	err = os.WriteFile(GetCachePath(), newData, 0644)
	if err != nil {
		fmt.Printf("Warning: failed to save cache: %v\n", err)
	}
}

// GetMostRecentApp returns the name and bundleID of the most recently used app in the cache.
func GetMostRecentApp() (string, string) {
	data, err := os.ReadFile(GetCachePath())
	if err != nil {
		return "", ""
	}
	var snapshots []SnapshotResponse
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return "", ""
	}
	if len(snapshots) > 0 {
		return snapshots[0].AppName, snapshots[0].BundleID
	}
	return "", ""
}

// GetLatestSnapshotFromCache returns the most recent snapshot for the given bundleID currently in cache.
// Since SaveCache is now called after PrintSnapshot, this effectively returns the "previous" snapshot.
func GetLatestSnapshotFromCache(bundleID string) *SnapshotResponse {
	if bundleID == "" {
		return nil
	}
	data, err := os.ReadFile(GetCachePath())
	if err != nil {
		return nil
	}
	var snapshots []SnapshotResponse
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return nil
	}

	for _, s := range snapshots {
		if s.BundleID == bundleID {
			return &s
		}
	}
	return nil
}

func FindHint(hint string, targetAppName string) (float64, float64, bool) {
	data, err := os.ReadFile(GetCachePath())
	if err != nil {
		fmt.Printf("Error: cache not found. Please run 'snapshot' first.\n")
		return 0, 0, false
	}

	var snapshots []SnapshotResponse
	if err := json.Unmarshal(data, &snapshots); err != nil {
		// Support old single-object format
		var single SnapshotResponse
		if err := json.Unmarshal(data, &single); err == nil {
			snapshots = []SnapshotResponse{single}
		} else {
			fmt.Printf("Error: failed to parse cache.\n")
			return 0, 0, false
		}
	}

	if len(snapshots) == 0 {
		fmt.Printf("Error: cache is empty.\n")
		return 0, 0, false
	}

	// Select snapshot: either by app name or the most recent one
	var resp SnapshotResponse
	if targetAppName != "" {
		targetAppNameLower := strings.ToLower(norm.NFC.String(targetAppName))

		// 1. Try to resolve the app to get its bundle ID for more reliable matching
		targetBundleID := ""
		apps, _ := Search(targetAppName)
		if len(apps) > 0 {
			targetBundleID = appuse_bridge.GetBundleIdentifier(apps[0].Path)
		}

		found := false
		// 2. Try matching by BundleID first
		if targetBundleID != "" {
			for _, s := range snapshots {
				if s.BundleID == targetBundleID {
					resp = s
					found = true
					break
				}
			}
		}

		// 3. Fallback to name matching if not found by bundle ID
		if !found {
			for _, s := range snapshots {
				if s.AppName == "" {
					continue
				}
				appNameLower := strings.ToLower(norm.NFC.String(s.AppName))
				if appNameLower == targetAppNameLower || strings.Contains(appNameLower, targetAppNameLower) || strings.Contains(targetAppNameLower, appNameLower) {
					resp = s
					found = true
					break
				}
			}
		}

		if !found {
			fmt.Printf("Error: no cached snapshot for app '%s'. Please run 'snapshot --appName %s' first.\n", targetAppName, targetAppName)
			return 0, 0, false
		}
	} else {
		resp = snapshots[0]
	}

	if resp.AppName == "" {
		fmt.Printf("Warning: empty app name in cached snapshot.\n")
	}

	hint = strings.ToUpper(hint)

	// Search in AX elements
	var searchElements func([]ElementNode) (float64, float64, bool)
	searchElements = func(nodes []ElementNode) (float64, float64, bool) {
		for _, node := range nodes {
			if strings.ToUpper(node.Hint) == hint {
				return node.Frame.X + node.Frame.Width/2, node.Frame.Y + node.Frame.Height/2, true
			}
			if x, y, found := searchElements(node.Children); found {
				return x, y, true
			}
		}
		return 0, 0, false
	}

	if x, y, found := searchElements(resp.Elements); found {
		return x, y, true
	}

	// Search in OCR elements
	for _, ocr := range resp.OCRElements {
		if strings.ToUpper(ocr.Hint) == hint {
			return ocr.Frame.X + ocr.Frame.Width/2, ocr.Frame.Y + ocr.Frame.Height/2, true
		}
	}

	// Search in Icon elements
	for _, icon := range resp.IconElements {
		if strings.ToUpper(icon.Hint) == hint {
			return icon.Frame.X + icon.Frame.Width/2, icon.Frame.Y + icon.Frame.Height/2, true
		}
	}

	return 0, 0, false
}

func Click(hint string, mode ClickMode, targetAppName string) error {
	x, y, found := FindHint(hint, targetAppName)
	if !found {
		return fmt.Errorf("hint '%s' not found", hint)
	}
	switch mode {
	case ClickRight:
		appuse_bridge.RightClickAt(x, y)
	case ClickDouble:
		appuse_bridge.DoubleClickAt(x, y)
	default:
		appuse_bridge.ClickAt(x, y)
	}
	return nil
}

func Type(hint, text, appName string) error {
	var fillX, fillY float64

	if hint != "" {
		x, y, found := FindHint(hint, appName)
		if !found {
			return fmt.Errorf("hint '%s' not found", hint)
		}
		fillX = x
		fillY = y
	} else {
		// Pass negative coordinates to indicate "use current focus"
		fillX = -1
		fillY = -1
	}

	// Support escape sequences (specifically \n, \t, \\ for multiline and formatting)
	replacer := strings.NewReplacer("\\n", "\n", "\\t", "\t", "\\\\", "\\")
	unescapedText := replacer.Replace(text)

	success := appuse_bridge.FillAt(fillX, fillY, unescapedText)
	if !success {
		return fmt.Errorf("failed to fill text at target")
	}
	return nil
}

// Search returns a list of matching applications based on the query.
func Search(query string) ([]AppInfo, error) {
	jsonStr := appuse_bridge.SearchApps()
	var apps []AppInfo
	if err := json.Unmarshal([]byte(jsonStr), &apps); err != nil {
		return nil, fmt.Errorf("failed to parse apps: %v", err)
	}

	if query == "" {
		return apps, nil
	}

	var results []AppInfo
	query = strings.ToLower(norm.NFC.String(query))
	for _, app := range apps {
		appName := strings.ToLower(norm.NFC.String(app.Name))
		fileName := strings.ToLower(norm.NFC.String(app.FileName))
		if strings.Contains(appName, query) || strings.Contains(fileName, query) {
			results = append(results, app)
		}
	}

	return results, nil
}

// Open launches an application by name or path.
func Open(query string) error {
	// 1. If it's a path that exists, open it directly
	absPath, _ := filepath.Abs(query)
	if _, err := os.Stat(absPath); err == nil && strings.HasSuffix(absPath, ".app") {
		if ok := appuse_bridge.OpenApp(absPath); ok {
			return nil
		}
		return fmt.Errorf("failed to open app at path: %s", absPath)
	}

	// 2. Search for the app by name
	apps, err := Search(query)
	if err != nil {
		return err
	}

	if len(apps) == 0 {
		return fmt.Errorf("no application found matching '%s'", query)
	}

	// 3. Find the best match
	target := apps[0]
	queryLower := strings.ToLower(norm.NFC.String(query))
	for _, app := range apps {
		if strings.ToLower(norm.NFC.String(app.FileName)) == queryLower ||
			strings.ToLower(norm.NFC.String(app.Name)) == queryLower {
			target = app
			break
		}
	}

	if ok := appuse_bridge.OpenApp(target.Path); ok {
		fmt.Printf("Opening %s (%s)...\n", target.FileName, target.Name)

		bundleID := appuse_bridge.GetBundleIdentifier(target.Path)
		if bundleID != "" {
			// Wait for window
			fmt.Printf("Waiting for window (timeout 30s)...\n")
			start := time.Now()
			for time.Since(start) < 30*time.Second {
				if appuse_bridge.AppHasWindow(bundleID) {
					fmt.Printf("Window detected.\n")
					// Bring to front just in case
					appuse_bridge.ActivateApp(bundleID)

					// Update cache metadata only to set "sticky" focus
					// We don't TakeSnapshot here as per user request to keep it fast
					partialResp := SnapshotResponse{
						AppName:  target.Name,
						BundleID: bundleID,
					}
					jsonData, _ := json.Marshal(partialResp)
					SaveCache(string(jsonData), nil)

					return nil
				}
				time.Sleep(500 * time.Millisecond)
			}
			return fmt.Errorf("timeout: no window detected for %s after 30s", target.Name)
		}
		return nil
	}
	return fmt.Errorf("failed to open app: %s", target.Name)
}

// Activate resolves the application by name/path and brings it to the front.
func Activate(query string) error {
	apps, err := Search(query)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return fmt.Errorf("no application found matching '%s'", query)
	}

	target := apps[0]
	queryLower := strings.ToLower(norm.NFC.String(query))
	for _, app := range apps {
		if strings.ToLower(norm.NFC.String(app.FileName)) == queryLower ||
			strings.ToLower(norm.NFC.String(app.Name)) == queryLower {
			target = app
			break
		}
	}

	bundleID := appuse_bridge.GetBundleIdentifier(target.Path)
	if bundleID == "" {
		return fmt.Errorf("failed to get bundle identifier for %s", target.Path)
	}

	if ok := appuse_bridge.ActivateApp(bundleID); !ok {
		// App might not be running, try to open it
		fmt.Printf("Application %s is not running. Launching it...\n", target.Name)
		return Open(target.Path)
	}
	return nil
}

// Close resolves the application by name/path and terminates it.
func Close(query string) error {
	apps, err := Search(query)
	if err != nil {
		return err
	}
	if len(apps) == 0 {
		return fmt.Errorf("no application found matching '%s'", query)
	}

	target := apps[0]
	queryLower := strings.ToLower(norm.NFC.String(query))
	for _, app := range apps {
		if strings.ToLower(norm.NFC.String(app.FileName)) == queryLower ||
			strings.ToLower(norm.NFC.String(app.Name)) == queryLower {
			target = app
			break
		}
	}

	bundleID := appuse_bridge.GetBundleIdentifier(target.Path)
	if bundleID == "" {
		return fmt.Errorf("failed to get bundle identifier for %s", target.Path)
	}

	if ok := appuse_bridge.TerminateApp(bundleID); !ok {
		return fmt.Errorf("failed to close app: %s", target.Name)
	}
	fmt.Printf("Closed %s (%s).\n", target.FileName, target.Name)
	return nil
}

// Screenshot takes a screenshot of the specified frame (or frontmost window if empty) and saves it to path.
func Screenshot(path, frame, targetAppName string) error {
	if path == "" {
		path = "screenshot.png"
	}

	// 1. Resolve target app if name is provided
	bundleID := ""
	if targetAppName != "" {
		if err := Activate(targetAppName); err != nil {
			return err
		}
		// Get bundle ID for specific targeting
		apps, _ := Search(targetAppName)
		if len(apps) > 0 {
			bundleID = appuse_bridge.GetBundleIdentifier(apps[0].Path)
		}
	}

	// 2. Determine frame if not provided
	if frame == "" {
		// Get frontmost window frame (of target app or system)
		winFrame := appuse_bridge.GetWindowFrame(bundleID)
		if winFrame == "" {
			return fmt.Errorf("failed to detect frontmost window frame")
		}
		frame = winFrame
		fmt.Printf("Detecting frontmost window frame: %s\n", frame)
	}

	// 3. Take screenshot
	// Ensure frame is in x,y,w,h format for screencapture -R
	frame = strings.ReplaceAll(frame, " ", "")
	if ok := appuse_bridge.SaveAreaScreenshot(path, frame); !ok {
		return fmt.Errorf("failed to take screenshot of frame: %s", frame)
	}

	fmt.Printf("Screenshot saved to %s\n", path)
	return nil
}

// SendKey sends a key (or combination like "cmd+v") to the target app.
func SendKey(key, targetAppName string) error {
	if targetAppName != "" {
		if err := Activate(targetAppName); err != nil {
			return err
		}
		// Give macOS a moment to complete the focus transition
		time.Sleep(200 * time.Millisecond)
	}
	if ok := appuse_bridge.SendKey(key); !ok {
		return fmt.Errorf("failed to send key: %s (ensure key name is valid)", key)
	}
	return nil
}

// Scroll performs a scroll in the specified area and direction.
func Scroll(areaName, direction string, distance int, targetAppName string) error {
	// 1. Activate app
	if targetAppName != "" {
		if err := Activate(targetAppName); err != nil {
			return err
		}
		time.Sleep(200 * time.Millisecond)
	}

	// 2. Resolve target app from cache if needed
	_, bundleID := GetMostRecentApp()
	if targetAppName != "" {
		apps, _ := Search(targetAppName)
		if len(apps) > 0 {
			bundleID = appuse_bridge.GetBundleIdentifier(apps[0].Path)
		}
	}

	// 3. Get snapshot from cache for areas
	data, err := os.ReadFile(GetCachePath())
	if err != nil {
		return fmt.Errorf("no cache found, please run snapshot first")
	}
	var snapshots []SnapshotResponse
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return fmt.Errorf("failed to parse cache")
	}

	var targetSnap *SnapshotResponse
	for _, s := range snapshots {
		if s.BundleID == bundleID {
			targetSnap = &s
			break
		}
	}

	if targetSnap == nil {
		return fmt.Errorf("no snapshot found for app in cache, please run snapshot first")
	}

	// 4. Find target coordinates
	var scrollX, scrollY float64
	var foundTarget bool

	areaQuery := strings.ToLower(areaName)
	for _, a := range targetSnap.Areas {
		lowerName := strings.ToLower(a.Name)
		// Only support 'main' or the alphabetical hint
		if (areaQuery == "main" || areaQuery == "center") && (lowerName == "main area" || lowerName == "center area" || lowerName == "vertical area" || lowerName == "horizontal area") {
			scrollX = a.Frame.X + a.Frame.Width/2
			scrollY = a.Frame.Y + a.Frame.Height/2
			foundTarget = true
			break
		}
		if strings.ToLower(a.Hint) == areaQuery || lowerName == areaQuery || lowerName == areaQuery+" area" {
			scrollX = a.Frame.X + a.Frame.Width/2
			scrollY = a.Frame.Y + a.Frame.Height/2
			foundTarget = true
			break
		}
	}

	if !foundTarget {
		// Fallback for 'main' or 'window' to the whole window center
		if areaQuery == "main" || areaQuery == "Main" {
			if targetSnap.FrontmostWindow != nil && targetSnap.FrontmostWindow.Frame != nil {
				scrollX = targetSnap.FrontmostWindow.Frame.X + targetSnap.FrontmostWindow.Frame.Width/2
				scrollY = targetSnap.FrontmostWindow.Frame.Y + targetSnap.FrontmostWindow.Frame.Height/2
				foundTarget = true
			}
		}

		// Fallback to AX/OCR hints
		if !foundTarget {
			if x, y, found := FindHint(areaName, targetAppName); found {
				scrollX, scrollY = x, y
				foundTarget = true
			}
		}
	}

	if !foundTarget {
		return fmt.Errorf("target hint/area '%s' not found", areaName)
	}

	// 5. Calculate scroll offsets
	dx, dy := 0.0, 0.0
	dist := float64(distance)
	if dist == 0 {
		dist = 200 // Default distance
	}

	switch strings.ToLower(direction) {
	case "up", "u":
		dy = dist
	case "down", "d":
		dy = -dist
	case "left", "l":
		dx = dist
	case "right", "r":
		dx = -dist
	default:
		return fmt.Errorf("invalid direction: %s", direction)
	}

	// 6. Execute scroll at target point
	appuse_bridge.ScrollAt(scrollX, scrollY, dx, dy)

	fmt.Printf("Scrolled %s on target '%s' by %.0f pixels\n", direction, areaName, dist)
	return nil
}

// CheckUpdate checks for the latest version of application-use on NPM.
// Returns the latest version string if an update is available, otherwise returns empty string.
func CheckUpdate() (string, error) {
	fmt.Printf("Checking for updates (current version: %s)...\n", Version)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://registry.npmjs.org/application-use/latest")
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to check for updates (HTTP %d)", resp.StatusCode)
	}

	var data struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", fmt.Errorf("failed to parse update info: %v", err)
	}

	if data.Version != "" && data.Version != Version {
		return data.Version, nil
	}
	return "", nil
}

// PerformUpgrade runs npm install -g application-use@latest to upgrade the tool.
func PerformUpgrade(latestVersion string) error {
	fmt.Printf("Updating to version %s...\n", latestVersion)

	cmd := exec.Command("npm", "install", "-g", "application-use@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to perform upgrade: %v", err)
	}

	fmt.Printf("\nSuccessfully upgraded to version %s!\n", latestVersion)
	return nil
}

// AutoCheckUpdate checks if the cache is older than 24 hours and triggers a background update if so.
func AutoCheckUpdate() {
	cachePath := GetCachePath()
	info, err := os.Stat(cachePath)
	if err != nil {
		return // Cache doesn't exist or is inaccessible
	}

	// Trigger if older than 24 hours
	if time.Since(info.ModTime()) > 24*time.Hour {
		go func() {
			latest, err := CheckUpdate()
			if err == nil && latest != "" {
				spawnBackgroundUpgrade()
			}
		}()
	}
}

// spawnBackgroundUpgrade starts a detached background process to perform the upgrade.
func spawnBackgroundUpgrade() {
	// Use nohup to ensure the process continues after the parent exits.
	// We redirect output to /dev/null as this is a background auto-update.
	cmd := exec.Command("nohup", "npm", "install", "-g", "application-use@latest")

	// Create a new process group to detach
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start without waiting
	_ = cmd.Start()
}
