package appuse

type ElementNode struct {
	Role        string        `json:"role"`
	Title       string        `json:"title"`
	Label       string        `json:"label"`
	Identifier  string        `json:"identifier"`
	Description string        `json:"description"`
	Help        string        `json:"help"`
	Hint        string        `json:"hint"`
	Frame       struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"frame"`
	Children []ElementNode `json:"children"`
}

type BreadcrumbNode struct {
	Role        string  `json:"role"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Identifier  string  `json:"identifier"`
	Frame       *struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"frame"`
}

type OCRElement struct {
	Name  string `json:"name"`
	Hint  string `json:"hint"`
	Frame struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"frame"`
}

type CaretRect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type SnapshotResponse struct {
	AppName          string          `json:"appName"`
	BundleID         string          `json:"bundleID"`
	Elements         []ElementNode   `json:"elements"`
	FrontmostWindow  *BreadcrumbNode `json:"frontmostWindow"`
	OCRElements      []OCRElement    `json:"ocrElements,omitempty"`
	Areas            []AreaNode      `json:"areas,omitempty"`
	Caret            *CaretRect      `json:"caret,omitempty"`
}

type AreaNode struct {
	Name  string `json:"name"`
	Hint  string `json:"hint"`
	Frame struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	} `json:"frame"`
}

type AppInfo struct {
	Name     string `json:"name"`
	FileName string `json:"fileName"`
	Path     string `json:"path"`
}
