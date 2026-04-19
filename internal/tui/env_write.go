package tui

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcaltamar/alice-installer/internal/envgen"
	"github.com/jcaltamar/alice-installer/internal/theme"
)

// EnvWriteModel renders the .env file generation + write screen.
//
// Behaviour:
//   - Init() runs Render + WriteEnv inside a tea.Cmd; emits EnvWrittenMsg on success.
//   - Also writes docker-compose.yml and docker-compose.gpu.yml from embedded assets.
//   - On render or write error → emits InstallFailureMsg{Stage: "env-write"}.
//   - View: spinner + "Writing .env…" while in-flight; checkmark on done.
type EnvWriteModel struct {
	theme        theme.Theme
	templater    *envgen.Templater
	writer       envgen.FileWriter
	assets       TemplateAssets
	spinner      spinner.Model
	targetPath   string
	workspaceDir string // directory for compose files (same dir as targetPath)
	input        envgen.Input
	err          error
	done         bool
	writtenPath  string
}

// NewEnvWriteModel constructs an EnvWriteModel.
func NewEnvWriteModel(
	th theme.Theme,
	templater *envgen.Templater,
	writer envgen.FileWriter,
	assets TemplateAssets,
	targetPath string,
	input envgen.Input,
) EnvWriteModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(string(theme.ColorPrimary)))
	return EnvWriteModel{
		theme:        th,
		templater:    templater,
		writer:       writer,
		assets:       assets,
		spinner:      sp,
		targetPath:   targetPath,
		workspaceDir: filepath.Dir(targetPath),
		input:        input,
	}
}

// Init implements tea.Model.
// Returns a Cmd that renders the .env template, writes it to targetPath,
// and also writes docker-compose.yml and docker-compose.gpu.yml into workspaceDir.
func (e EnvWriteModel) Init() tea.Cmd {
	return func() tea.Msg {
		// Ensure the workspace directory exists.
		if err := os.MkdirAll(e.workspaceDir, 0o700); err != nil {
			return InstallFailureMsg{Err: err, Stage: "env-write"}
		}

		// Render and write the .env file.
		rendered, err := e.templater.Render(e.assets.EnvExample, e.input)
		if err != nil {
			return InstallFailureMsg{Err: err, Stage: "env-write"}
		}
		if err := e.writer.WriteEnv(e.targetPath, rendered); err != nil {
			return InstallFailureMsg{Err: err, Stage: "env-write"}
		}

		// Write compose files from embedded assets.
		composePath := filepath.Join(e.workspaceDir, "docker-compose.yml")
		if err := os.WriteFile(composePath, e.assets.BaselineYAML, 0o644); err != nil {
			return InstallFailureMsg{Err: err, Stage: "env-write"}
		}
		composeGPUPath := filepath.Join(e.workspaceDir, "docker-compose.gpu.yml")
		if err := os.WriteFile(composeGPUPath, e.assets.OverlayYAML, 0o644); err != nil {
			return InstallFailureMsg{Err: err, Stage: "env-write"}
		}

		return EnvWrittenMsg{Path: e.targetPath}
	}
}

// Update implements tea.Model.
func (e EnvWriteModel) Update(msg tea.Msg) (EnvWriteModel, tea.Cmd) {
	switch m := msg.(type) {
	case EnvWrittenMsg:
		e.done = true
		e.writtenPath = m.Path
		return e, nil

	case InstallFailureMsg:
		e.err = m.Err
		return e, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		e.spinner, cmd = e.spinner.Update(m)
		return e, cmd
	}
	return e, nil
}

// View implements tea.Model.
func (e EnvWriteModel) View() string {
	title := e.theme.Primary.Bold(true).Render("Environment Setup")

	if e.err != nil {
		return title + "\n\n" + e.theme.Danger.Render("✗  Failed: "+e.err.Error()) + "\n"
	}

	if e.done {
		return title + "\n\n" +
			e.theme.Success.Render(fmt.Sprintf("✓  Written .env and compose files to %s", e.workspaceDir)) + "\n"
	}

	return title + "\n\n" +
		e.spinner.View() + " " +
		e.theme.TextMuted.Render(fmt.Sprintf("Writing .env and compose files to %s…", e.workspaceDir)) + "\n"
}
