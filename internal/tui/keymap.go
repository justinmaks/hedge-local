package tui

type KeyMap struct {
	TabNext     string
	TabPrev     string
	Refresh     string
	DateFilter  string
	Help        string
	Quit        string
	ScrollUp    string
	ScrollDown  string
	ScrollLeft  string
	ScrollRight string
	Enter       string
	Escape      string
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		TabNext:     "tab",
		TabPrev:     "shift+tab",
		Refresh:     "r",
		DateFilter:  "e",
		Help:        "?",
		Quit:        "q",
		ScrollUp:    "up",
		ScrollDown:  "down",
		ScrollLeft:  "left",
		ScrollRight: "right",
		Enter:       "enter",
		Escape:      "esc",
	}
}

var tabNames = []string{"Today", "Cost", "Tools", "Models", "Projects", "Live", "Export"}
