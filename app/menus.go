package app

import "github.com/egoist/mygo/ui"

// Context menus and toolbar menus share the theme, spacing and motion.
// Keep MyGo's item highlights and keyboard navigation intact.
func menuPanel(panel ui.Element) {
	panel.Radius(10).Padding(4).Transition(popIn)
}

func menuItem(item ui.Element) {
	item.Padding(5, 10).Radius(6)
}

func contextMenu(e ui.Element, build func(*ui.Menu)) ui.Element {
	return e.PopoverMenu(build, ui.PopoverMenuOptions{Panel: menuPanel, Item: menuItem})
}

func menuButton(e ui.Element, build func(*ui.Menu)) ui.Element {
	return e.PopoverMenuButton(build, ui.PopoverMenuButtonOptions{Panel: menuPanel, Item: menuItem})
}
