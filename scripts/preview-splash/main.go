// Dump the splash View() to stdout so devs can visually verify the logo
// render without launching the full TUI:
//
//	go run ./scripts/preview-splash
package main

import (
	"fmt"

	"github.com/jcaltamar/alice-installer/internal/theme"
	"github.com/jcaltamar/alice-installer/internal/tui"
)

func main() {
	s := tui.NewSplashModel(theme.Default())
	fmt.Println(s.View())
}
