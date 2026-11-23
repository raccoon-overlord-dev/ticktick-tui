package ui

// density is how much room the UI spends on itself (Settings → Density). The font size is the
// terminal's own setting; what ttui can scale is the space between and inside its panes.
type density struct {
	gap    int // blank lines between blocks: lists sections, task groups, detail blocks
	rowGap int // blank lines between task rows
	pad    int // inner horizontal padding, per side, inside a pane
	gutter int // outer margin and the gutter between panes
	lists  int // sidebar width in the 3-pane layout (0: the default, which widens on a wide terminal)
}

// dens reads the Density setting; anything unknown (including "") is normal.
func dens(name string) density {
	switch name {
	case "compact":
		return density{gap: 0, rowGap: 0, pad: 0, gutter: 0, lists: 20}
	case "roomy":
		return density{gap: 2, rowGap: 1, pad: 2, gutter: 2, lists: 30}
	}
	return density{gap: 1, rowGap: 0, pad: 1, gutter: 1}
}

// blanks is n empty lines, for the gaps between blocks.
func blanks(n int) []string {
	if n <= 0 {
		return nil
	}
	return make([]string, n)
}
