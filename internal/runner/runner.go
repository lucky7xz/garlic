package runner

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lucky7xz/garlic/internal/config"
	"github.com/lucky7xz/garlic/internal/ui"
)

func isAsync(cmd string, asyncList []string) bool {
	base := filepath.Base(cmd)
	for _, app := range asyncList {
		if app == base {
			return true
		}
	}
	return false
}

func resolveCmd(cfgCmd, envVar string) (string, []string) {
	cmdStr := os.Getenv(envVar)
	if cmdStr == "" {
		cmdStr = cfgCmd
	}

	if cmdStr == "" {
		if runtime.GOOS == "darwin" {
			return "open", nil
		}
		return "xdg-open", nil
	}

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return "xdg-open", nil
	}

	binary := parts[0]
	args := parts[1:]

	if _, err := exec.LookPath(binary); err != nil {
		log.Printf("Warning: command '%s' not found, falling back to system default", binary)
		if runtime.GOOS == "darwin" {
			return "open", nil
		}
		return "xdg-open", nil
	}

	return binary, args
}

func Run() {
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

		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			log.Fatalf("Error running program: %v", err)
		}

		fModel, ok := finalModel.(ui.Model)
		if !ok || (fModel.SelectedPath == "" && fModel.ResourcePath == "") {
			break
		}

		if fModel.ResourcePath != "" {
			var binary string
			var args []string

			if fModel.UseAlt {
				binary, args = resolveCmd(cfg.AltFileManager, "")
			} else {
				binary, args = resolveCmd(cfg.FileManager, "FILEMANAGER")
			}

			args = append(args, fModel.ResourcePath)
			cmd := exec.Command(binary, args...)

			if isAsync(binary, cfg.AsyncApps) {
				if err := cmd.Start(); err != nil {
					log.Printf("Failed to start async file manager: %v\n", err)
				}
			} else {
				cmd.Stdin = os.Stdin
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					log.Printf("Failed to orchestrate file manager: %v\n", err)
				}
			}
			continue
		}

		var binary string
		var args []string

		if fModel.UseAlt {
			binary, args = resolveCmd(cfg.AltEditor, "")
		} else {
			binary, args = resolveCmd(cfg.Editor, "EDITOR")
		}

		args = append(args, fModel.SelectedPath)
		cmd := exec.Command(binary, args...)

		if isAsync(binary, cfg.AsyncApps) {
			if err := cmd.Start(); err != nil {
				log.Printf("Editor (async) exited with error: %v\n", err)
			}
		} else {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				log.Printf("Editor exited with error: %v\n", err)
			}
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
		"epics/fitness/running.md":          "#statustag-inProgress\n",
		"epics/learning/golang.md":          "#statustag-toDo\n",
		"scripts/garlic/release.clove.md":   "#statustag-onHold\n",
		"scripts/garlic/revise.clove.md":    "#statustag-inProgress\n",
		"scripts/drako/revise.clove.md":     "#statustag-onHold\n",
		"decks/ggml_deck/llamacpp.clove.md": "#statustag-onHold\n",
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
