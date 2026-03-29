package appuse_bridge

/*
#cgo LDFLAGS: -L${SRCDIR} ${SRCDIR}/libappuse_bridge.a -framework Foundation -framework AppKit -framework ApplicationServices
#include <stdlib.h>
#include <stdbool.h>

char* trigger_appuse_snapshot();
void show_appuse_overlay();
void clear_appuse_snapshot();
bool click_at(double x, double y);
bool double_click_at(double x, double y);
bool right_click_at(double x, double y);
bool fill_at(double x, double y, char* text);
char* search_apps();
bool open_app_at_path(char* path);
char* get_bundle_identifier(char* path);
bool app_has_window(char* bundleID);
bool activate_app(char* bundleID);
bool terminate_app(char* bundleID);
bool save_area_screenshot(char* path, char* frame);
char* get_window_frame(char* bundleID);
bool send_key(char* key);
void scroll_at(double x, double y, double dx, double dy);
bool check_accessibility_permission(int prompt);
bool check_screen_recording_permission();
*/
import "C"

import (
	"unsafe"
)

// TriggerSnapshot extracts the accessibility tree + cursor info. Returns JSON.
// Does NOT show the overlay. Call ShowOverlay() after screenshots are taken.
func TriggerSnapshot() string {
	cStr := C.trigger_appuse_snapshot()
	if cStr == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cStr))
	return C.GoString(cStr)
}

// ShowOverlay displays the hint letter overlay on screen.
// Call this AFTER taking per-element screenshots.
func ShowOverlay() {
	C.show_appuse_overlay()
}

// ClearSnapshot removes the visual overlay.
func ClearSnapshot() {
	C.clear_appuse_snapshot()
}

// ClickAt performs a mouse click at the specified coordinates.
func ClickAt(x, y float64) {
	C.click_at(C.double(x), C.double(y))
}

// DoubleClickAt performs a double mouse click at the specified coordinates.
func DoubleClickAt(x, y float64) {
	C.double_click_at(C.double(x), C.double(y))
}

// RightClickAt performs a right mouse click at the specified coordinates.
func RightClickAt(x, y float64) {
	C.right_click_at(C.double(x), C.double(y))
}

// FillAt sets the value of the element at (x, y), or the focused element if x, y < 0.
func FillAt(x, y float64, text string) bool {
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))
	return bool(C.fill_at(C.double(x), C.double(y), cText))
}

// SearchApps returns a JSON string of all installed applications.
func SearchApps() string {
	cStr := C.search_apps()
	if cStr == nil {
		return "[]"
	}
	defer C.free(unsafe.Pointer(cStr))
	return C.GoString(cStr)
}

// OpenApp launches the application at the specified path.
func OpenApp(path string) bool {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	return bool(C.open_app_at_path(cPath))
}

// GetBundleIdentifier returns the bundle ID of the app at path.
func GetBundleIdentifier(path string) string {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	cStr := C.get_bundle_identifier(cPath)
	if cStr == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cStr))
	return C.GoString(cStr)
}

// AppHasWindow returns true if the app with bundleID has a window.
func AppHasWindow(bundleID string) bool {
	cID := C.CString(bundleID)
	defer C.free(unsafe.Pointer(cID))
	return bool(C.app_has_window(cID))
}

// ActivateApp brings the app with bundleID to the front.
func ActivateApp(bundleID string) bool {
	cID := C.CString(bundleID)
	defer C.free(unsafe.Pointer(cID))
	return bool(C.activate_app(cID))
}

// TerminateApp closes the app with bundleID.
func TerminateApp(bundleID string) bool {
	cID := C.CString(bundleID)
	defer C.free(unsafe.Pointer(cID))
	return bool(C.terminate_app(cID))
}

// SaveAreaScreenshot takes a screenshot of the specified frame and saves it to path.
func SaveAreaScreenshot(path, frame string) bool {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))
	cFrame := C.CString(frame)
	defer C.free(unsafe.Pointer(cFrame))
	return bool(C.save_area_screenshot(cPath, cFrame))
}

// GetWindowFrame returns the frame (x,y,w,h) of the main window for bundleID.
// If bundleID is empty, it uses the frontmost application.
func GetWindowFrame(bundleID string) string {
	cID := C.CString(bundleID)
	defer C.free(unsafe.Pointer(cID))
	cStr := C.get_window_frame(cID)
	if cStr == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(cStr))
	return C.GoString(cStr)
}

// SendKey simulates pressing the specified key string (e.g., "enter", "cmd+c").
func SendKey(key string) bool {
	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))
	return bool(C.send_key(cKey))
}

// ScrollAt performs a scroll at the specified coordinates.
func ScrollAt(x, y, dx, dy float64) {
	C.scroll_at(C.double(x), C.double(y), C.double(dx), C.double(dy))
}

// CheckAccessibilityPermission returns true if the app has accessibility permissions.
// If prompt is true, it will trigger a system prompt if permission is missing.
func CheckAccessibilityPermission(prompt bool) bool {
	p := 0
	if prompt {
		p = 1
	}
	return bool(C.check_accessibility_permission(C.int(p)))
}

// CheckScreenRecordingPermission returns true if the app has screen recording permissions.
func CheckScreenRecordingPermission() bool {
	return bool(C.check_screen_recording_permission())
}
