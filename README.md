# application-use

A native, blazingly fast macOS application automation CLI designed specifically for AI agents. 

Similar to Anthropic's **Computer Use**, `application-use` empowers AI agents to operate desktop applications. However, instead of relying on heavy visual inference and imprecise `(x, y)` coordinate clicks from full-screen screenshots, **application-use** provides a **textual understanding interface built directly on top of underlying macOS native APIs (Accessibility) and Apple Vision Framework**. 

By operating at the OS level, it retrieves a highly structured view of the UI instantly. This approach achieves superior speed, deterministic accuracy, and significantly better effects for LLMs navigating complex desktop interfaces.

## Key Features

- **Native OS APIs**: Uses macOS native Accessibility (`AXUIElement`) for instant, reliable UI tree extraction and precise element interaction.
- **Vision Integration**: Powered by Apple's built-in Vision framework for robust OCR and screen analysis, easily capturing and interacting with elements that standard APIs miss.
- **AI-Optimized Interface**: Converts visual spatial information into structured text representations (snapshots with alphabet hints like `JK`). LLMs can instantly map these hints back to specific actions.
- **Fast & Lightweight**: Built meticulously with Go and Swift (via CGO bridge) for maximum native performance without heavy dependencies.


## Installation

### Global Installation (recommended)

Installs the native execution binary directly from NPM:

```bash
npm install -g application-use@latest
```

### AI Coding Assistants (recommended)

Add the skill to your AI coding assistant for richer context:

```bash
npx skills add qdore/application-use
```

### From Source (macOS)

Dependencies: Go (1.20+) and Xcode Command Line Tools.

```bash
git clone <repository-url>
cd application-use

# Build the Swift static bridge and Go executable
make clean && make
```

## Quick Start

```bash
# Search for installed apps
application-use search "safari"

# Open an application
application-use open --appName "Safari"

# Take a structural snapshot (returns an annotated accessibility tree for AI)
application-use snapshot --appName "Safari"

# Click an element by its hint letters (e.g., 'JK' returned from the snapshot)
application-use click JK --appName "Safari"

# Fill text into an element
application-use fill JK "hello world" --appName "Safari"

# Send keystrokes directly
application-use sendkey cmd+t --appName "Safari"

# Take a screenshot
application-use screenshot result.png --appName "Safari"

# Close the application
application-use close --appName "Safari"
```

## Core Commands

```bash
application-use open --appName <app>                  # Open a specific application
application-use snapshot --appName <app>              # Snapshot the UI tree & overlay alphabet hints on interactive elements
application-use click <hint> --appName <app>          # Click element by hint (e.g. 'JK'). Supports: --right, --double
application-use fill [hint] <text> --appName <app>    # Fill text into an element or current focus (uses clipboard paste)
application-use sendkey <key> --appName <app>         # Send a key combination (e.g., cmd+v, enter, esc)
application-use scroll <area> <dir> [px] --appName <app> # Scroll a specific UI block (up/down/left/right)
application-use screenshot [path] --appName <app>     # Take a visual screenshot of the window
application-use search [query]                        # Search for installed applications
application-use close --appName <app>                 # Gracefully close the application
application-use upgrade                               # Check for updates and automatically upgrade
```

## Agent Workflow (Recommended for AI)

Instead of relying on fragile coordinate clicks (`x,y`), **application-use** implements an LLM-friendly snapshot-and-interact pattern:

1. **Snapshot**: `application-use snapshot` queries the native Accessibility tree and layers Vision OCR data. It assigns a deterministic 2-3 letter hint (like `AA`, `JK`) to every clickable, editable, or readable element.
2. **Understand**: The LLM parses the printed structural tree to understand the UI layout.
   - `(+)` markers indicate pure text elements.
   - `(*)` markers indicate elements discovered primarily via OCR.
3. **Interact**: The LLM issues a command like `application-use click JK` or `application-use fill AB "admin"`. The CLI uses OS-level handles to instantly perform the action.

## Security & Permissions

Because `application-use` interacts natively with OS controls and visual outputs, it requires two macOS permissions on the first run:

1. **Accessibility**: `System Settings > Privacy & Security > Accessibility` (to read the UI tree and inject clicks/keys)
2. **Screen Recording**: `System Settings > Privacy & Security > Screen Recording` (required for Vision OCR and taking snapshots)

If permissions are missing, the CLI will output specialized error prompts instructing the user to enable them.

## Contributing

Currently, `application-use` is designed for macOS. We welcome pull requests to add support for other operating systems (Windows, Linux, etc.).
