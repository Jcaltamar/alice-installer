package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcaltamar/alice-installer/internal/theme"
)

// OptionalPackage identifies an opt-in installer package.
type OptionalPackage string

const (
	// OptionalPackageAsterisk enables host-managed Asterisk SIP Audio Server setup.
	OptionalPackageAsterisk OptionalPackage = "asterisk"
)

type optionalPackageItem struct {
	id          OptionalPackage
	label       string
	description string
}

var asteriskOptionalPackage = optionalPackageItem{
	id:          OptionalPackageAsterisk,
	label:       "Asterisk SIP Audio Server",
	description: "Prepare host Asterisk and backend-visible AMI resources.",
}

// OptionalPackagesModel renders the optional package selection screen.
type OptionalPackagesModel struct {
	theme    theme.Theme
	items    []optionalPackageItem
	cursor   int
	selected map[OptionalPackage]bool
}

// NewOptionalPackagesModel constructs an optional package selector.
func NewOptionalPackagesModel(th theme.Theme, asteriskAvailable bool) OptionalPackagesModel {
	items := []optionalPackageItem{}
	if asteriskAvailable {
		items = append(items, asteriskOptionalPackage)
	}
	return OptionalPackagesModel{
		theme:    th,
		items:    items,
		selected: make(map[OptionalPackage]bool),
	}
}

// Init implements tea.Model.
func (o OptionalPackagesModel) Init() tea.Cmd {
	return nil
}

// IsSelected reports whether the package is selected.
func (o OptionalPackagesModel) IsSelected(pkg OptionalPackage) bool {
	return o.selected[pkg]
}

// Update implements tea.Model.
func (o OptionalPackagesModel) Update(msg tea.Msg) (OptionalPackagesModel, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch m.Type {
		case tea.KeyUp:
			if o.cursor > 0 {
				o.cursor--
			}
		case tea.KeyDown:
			if o.cursor < len(o.items)-1 {
				o.cursor++
			}
		case tea.KeySpace:
			if len(o.items) == 0 {
				return o, nil
			}
			id := o.items[o.cursor].id
			o.selected[id] = !o.selected[id]
		case tea.KeyEnter:
			selected := copyOptionalPackageMap(o.selected)
			return o, func() tea.Msg { return OptionalPackagesConfirmedMsg{Selected: selected} }
		}
	}
	return o, nil
}

// View implements tea.Model.
func (o OptionalPackagesModel) View() string {
	title := o.theme.Primary.Bold(true).Render("Optional Packages")
	var lines []string
	lines = append(lines, title, "")

	if len(o.items) == 0 {
		lines = append(lines, o.theme.TextMuted.Render("No optional packages are available on this host."))
		lines = append(lines, "", o.theme.TextMuted.Render("Press Enter to continue."))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, o.theme.TextMuted.Render("Select optional host setup packages:"))
	lines = append(lines, "")
	for i, item := range o.items {
		cursor := " "
		if i == o.cursor {
			cursor = ">"
		}
		check := "[ ]"
		if o.selected[item.id] {
			check = "[x]"
		}
		lines = append(lines, fmt.Sprintf("%s %s %s", cursor, check, item.label))
		lines = append(lines, "    "+o.theme.TextMuted.Render(item.description))
	}
	lines = append(lines, "", o.theme.TextMuted.Render("Space toggles selection. Press Enter to continue."))
	return strings.Join(lines, "\n")
}

func copyOptionalPackageMap(in map[OptionalPackage]bool) map[OptionalPackage]bool {
	out := make(map[OptionalPackage]bool, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
