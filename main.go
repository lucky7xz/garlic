package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lucky7xz/garlic/internal/config"
	"github.com/lucky7xz/garlic/internal/ui"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		initDemo()
		return
	}

	for {
		cfg, err := config.LoadConfig()
		if err != nil {
			log.Fatalf("Error loading configuration: %v", err)
		}

		themes, err := config.LoadThemes()
		if err != nil {
			log.Fatalf("Error loading themes: %v", err)
		}

		themeName := cfg.Theme
		if themeName == "" {
			themeName = "dracula"
		}

		theme, ok := themes[themeName]
		if !ok {
			log.Printf("Theme '%s' not found, defaulting to 'dracula'", themeName)
			theme = themes["dracula"]
		}

		m := ui.InitialModel(cfg)
		ui.ApplyTheme(theme, &m)

		p := tea.NewProgram(m)
		finalModel, err := p.Run()
		if err != nil {
			log.Fatalf("Error running program: %v", err)
		}

		fModel, ok := finalModel.(ui.Model)
		if !ok || (fModel.SelectedPath == "" && fModel.ResourcePath == "") {
			break
		}

		if fModel.ResourcePath != "" {
			fm := os.Getenv("FILEMANAGER")
			if fm == "" {
				fm = cfg.FileManager
			}
			if fm == "" {
				fm = "xdg-open"
			}

			cmd := exec.Command(fm, fModel.ResourcePath)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Printf("Failed to orchestrate file manager: %v\n", err)
			}
			continue
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = cfg.Editor
		}
		if editor == "" {
			log.Printf("No editor configured. Set EDITOR environment variable or configure 'editor' in config file.")
			continue
		}

		cmd := exec.Command(editor, fModel.SelectedPath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Fatal(err)
		}
	}
}

func initDemo() {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, "shara")

	if _, err := os.Stat(base); err == nil {
		fmt.Printf("Demo directory %s already exists.\n", base)
		return
	}

	files := map[string]string{
		"epics/fitness/running.md":       "#statustag-inProgress\n",
		"epics/learning/golang.md":       "#statustag-toDo\n",
		"scripts/garlic/release.clove.md":"#statustag-onHold\n",
		"scripts/garlic/revise.clove.md": "#statustag-inProgress\n",
		"scripts/drako/revise.clove.md":  "#statustag-onHold\n",
		"decks/ggml_deck/llamacpp.clove.md":    "#statustag-onHold\n",
	}

	for path, content := range files {
		fullPath := filepath.Join(base, path)
		os.MkdirAll(filepath.Dir(fullPath), 0755)
		os.WriteFile(fullPath, []byte(content), 0644)
	}

	// Create an empty resource directory for the demo
	os.MkdirAll(filepath.Join(base, "epics/fitness/running"), 0755)

	fmt.Printf("Demo instantiated at %s\n", base)
}
