# Application-Use Project Standards

## Directory Structure

```
.
├── cmd/
│   └── application-use/
│       └── main.go                 # Entry point - initializes CLI on main thread
├── internal/
│   ├── appuse/                     # Service Layer - core domain logic
│   │   ├── service.go              # High-level services (Snapshot, Search, Open, Click, Fill)
│   │   ├── models.go               # Data structures (ElementNode, OCRElement, AppInfo)
│   │   └── display.go              # Snapshot visualization and terminal printing
│   ├── cli/                        # Interface Layer - user interaction
│   │   ├── root.go                 # Cobra root command
│   │   ├── commands.go             # Subcommands (snapshot, click, fill, open, search)
│   │   └── daemon.go               # Background daemon orchestration
│   ├── cgo/macos/appuse_bridge/    # Bridge Layer - Go/Swift interface
│   │   ├── appuse_bridge.go        # CGO declarations and wrappers
│   │   └── AppUseSnapshot.swift    # Swift implementation (Accessibility, OCR, Mouse)
│   └── daemon/                     # Background Services
│       └── hotkey.go               # Hotkey handling logic
└── Makefile                        # Build orchestration
```

## Build Instructions

To build the project from scratch:

```bash
make clean && make
```

This command will:
1. Clean all build artifacts (Swift libraries and Go binary)
2. Build the Swift static library (`libappuse_bridge.a`)
3. Build the Go application binary (`application-use`)

The build process is orchestrated by the `Makefile` and handles all cross-language compilation automatically.

## Code Standards

1.  **Layered Separation**: The CLI and Daemon should NEVER call CGO functions directly. All interactions must go through the `internal/appuse` service layer.
2.  **Swift/CGO Integrity**:
    - Swift functions exported to C must be annotated with `@_cdecl`.
    - Always use `defer C.free(unsafe.Pointer(cStr))` for strings returned from Swift/C to prevent memory leaks.
    - Add `#include <stdlib.h>` and `#include <stdbool.h>` in CGO preamble for necessary types.
3.  **macOS Main Thread**: All UI-related calls (including `AXUIElement` and `NSWorkspace`) MUST be reachable from the main thread initialized in `main.go` using `mainthread.Run()`.
4.  **Localization Support**:
    - Always use Unicode normalization (NFC) when comparing user-input strings with system strings (filenames, etc.).
    - Detect system preferred languages and pass them to Vision OCR for better accuracy.
5.  **Clean CLI Interface**:
    - Keep command descriptions concise and action-oriented.
    - Use flags for variations (e.g., `click --right`) instead of separate subcommands.
6.  **Snapshot Lifecycle**: Ensure `ClearSnapshot()` is called when the visual overlay is no longer needed to maintain a clean user screen.
