package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestOptionalPackagesModelShowsAsteriskUncheckedByDefault(t *testing.T) {
	t.Parallel()

	model := NewOptionalPackagesModel(themeDefaultForTest(), true)

	view := model.View()
	if !strings.Contains(view, "Optional Packages") {
		t.Fatalf("optional packages view missing title:\n%s", view)
	}
	if !strings.Contains(view, "Asterisk SIP Audio Server") {
		t.Fatalf("optional packages view missing Asterisk option:\n%s", view)
	}
	if !strings.Contains(view, "[ ] Asterisk SIP Audio Server") {
		t.Fatalf("Asterisk option should be unchecked by default:\n%s", view)
	}
	if model.IsSelected(OptionalPackageAsterisk) {
		t.Fatal("Asterisk option should not be selected by default")
	}
}

func TestOptionalPackagesModelTogglesAsteriskSelection(t *testing.T) {
	t.Parallel()

	model := NewOptionalPackagesModel(themeDefaultForTest(), true)

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if !updated.IsSelected(OptionalPackageAsterisk) {
		t.Fatal("space should select Asterisk option")
	}
	if !strings.Contains(updated.View(), "[x] Asterisk SIP Audio Server") {
		t.Fatalf("selected Asterisk option should render checked:\n%s", updated.View())
	}

	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeySpace})
	if updated.IsSelected(OptionalPackageAsterisk) {
		t.Fatal("second space should clear Asterisk option")
	}
}

func TestOptionalPackagesModelEnterEmitsSelection(t *testing.T) {
	t.Parallel()

	model := NewOptionalPackagesModel(themeDefaultForTest(), true)
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeySpace})

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should emit OptionalPackagesConfirmedMsg")
	}
	msg, ok := cmd().(OptionalPackagesConfirmedMsg)
	if !ok {
		t.Fatalf("enter emitted %T, want OptionalPackagesConfirmedMsg", cmd())
	}
	if !msg.Selected[OptionalPackageAsterisk] {
		t.Fatalf("selection message should include Asterisk=true, got %#v", msg.Selected)
	}
}

func TestOptionalPackagesModelHidesAsteriskWhenUnsupported(t *testing.T) {
	t.Parallel()

	model := NewOptionalPackagesModel(themeDefaultForTest(), false)

	view := model.View()
	if strings.Contains(view, "Asterisk SIP Audio Server") {
		t.Fatalf("unsupported hosts should not show selectable Asterisk option:\n%s", view)
	}
}
