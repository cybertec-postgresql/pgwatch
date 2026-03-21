package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Style for the header
var headerStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#21CBF3")).
	Bold(true).
	MarginLeft(2)

type model struct {
	cursor  int
	choices []string
}

func initialModel() model {
	return model{
		choices: []string{"Analyze Slow Queries", "Check Resource Spikes", "Ask Custom Question"},
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			fmt.Printf("\nSelected: %s (Backend logic coming soon!)\n", m.choices[m.cursor])
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string {
	s := headerStyle.Render("🤖 pgwatch3 AI Copilot (TUI Mode)") + "\n\n"

	for i, choice := range m.choices {
		cursor := "  "
		if m.cursor == i {
			cursor = "> "
		}
		s += fmt.Sprintf("%s%s\n", cursor, choice)
	}

	s += "\n(press q to quit or enter to select)\n"
	return s
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running TUI: %v", err)
		os.Exit(1)
	}
}
