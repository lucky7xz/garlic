package runner

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lucky7xz/garlic/internal/config"
	"github.com/lucky7xz/garlic/internal/remote"
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

var exitWarnings []string

func fallback() (string, []string) {
	if runtime.GOOS == "darwin" {
		return "open", nil
	}
	return "xdg-open", nil
}

func resolveCmd(cfgCmd, envVar string) (string, []string) {
	cmdStr := cfgCmd
	source := "config"

	// If config is empty, try the environment variable
	if cmdStr == "" && envVar != "" {
		cmdStr = os.Getenv(envVar)
		source = "environment"
	}

	// If both are empty, use fallback
	if cmdStr == "" {
		return fallback()
	}

	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return fallback()
	}

	binary := parts[0]
	var args []string
	if len(parts) > 1 {
		// Keep it simple: allow only one flag
		args = []string{parts[1]}
	}

	if _, err := exec.LookPath(binary); err != nil {
		exitWarnings = append(exitWarnings, fmt.Sprintf("%s command '%s' not found. Falling back to default.", source, binary))
		return fallback()
	}

	return binary, args
}

func executeCmd(binary string, args []string, asyncApps []string) {
	cmd := exec.Command(binary, args...)
	if isAsync(binary, asyncApps) {
		// TOTAL DETACHMENT: New session, new group, no controlling terminal.
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setsid: true,
		}

		// BLACK HOLE: Explicitly route all output to /dev/null to stop bleeding.
		devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
		if err == nil {
			cmd.Stdout = devNull
			cmd.Stderr = devNull
		}

		if err := cmd.Start(); err != nil {
			log.Printf("Failed to start async command: %v\n", err)
		} else {
			// REAPING: Prevent zombie processes.
			go func() {
				_ = cmd.Wait()
				if devNull != nil {
					devNull.Close()
				}
			}()
		}
	} else {
		// ATTACHMENT: Standard blocking execution for TUIs (vim, nano, etc.)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			log.Printf("Command exited with error: %v\n", err)
		}
	}
}

func Run() {
	if len(os.Args) > 1 {
		switch {
		case os.Args[1] == "init":
			initDemo()
			return
		case os.Args[1] == "__complete":
			// Completion reads only this machine, which is also the only thing
			// that can be planted -- no ssh runs until you press enter. A broken
			// config offers nothing rather than shouting into the prompt.
			cfg, err := config.LoadConfig()
			if err != nil {
				return
			}
			for _, candidate := range remote.Complete(cfg, os.Args[2:]) {
				fmt.Println(candidate)
			}
			return
		case os.Args[1] == "completion":
			shell := ""
			if len(os.Args) > 2 {
				shell = os.Args[2]
			}
			script, err := remote.CompletionScript(shell)
			if err != nil {
				fmt.Fprintf(os.Stderr, "garlic: %v\n", err)
				os.Exit(1)
			}
			fmt.Print(script)
			return
		case remote.IsCommand(os.Args[1]):
			cfg, err := config.LoadConfig()
			if err != nil {
				log.Fatalf("Error loading configuration: %v", err)
			}
			if err := remote.Run(cfg, os.Args[1:]); err != nil {
				fmt.Fprintf(os.Stderr, "garlic: %v\n", err)
				os.Exit(1)
			}
			return
		}
	}

	// Each lap rebuilds the model from scratch, so where the cursor was has to be
	// handed forward explicitly. The zero value restores nothing, which is what
	// the first lap wants.
	var session ui.Session

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
		m.Restore(session)

		p := tea.NewProgram(m, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			log.Fatalf("Error running program: %v", err)
		}

		fModel, ok := finalModel.(ui.Model)
		if ok {
			// Both of these have to happen before the branches below: the resource
			// branch leaves via continue, and the next lap builds its own watcher
			// whether or not this one was cleaned up.
			session = fModel.Session()
			fModel.StopWatcher()
		}
		if !ok || (fModel.SelectedPath == "" && fModel.ResourcePath == "") {
			if len(exitWarnings) > 0 {
				fmt.Println("\n--- Garlic Post-Run Warnings ---")
				for _, w := range exitWarnings {
					fmt.Printf("⚠️  %s\n", w)
				}
			}
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
			executeCmd(binary, args, cfg.AsyncApps)
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
		executeCmd(binary, args, cfg.AsyncApps)
	}
}

func initDemo() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "garlic: %v\n", err)
		return
	}
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
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "garlic: %v\n", err)
			return
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "garlic: %v\n", err)
			return
		}
	}

	// Create an empty resource directory for the demo
	if err := os.MkdirAll(filepath.Join(base, "epics/fitness/running"), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "garlic: %v\n", err)
		return
	}

	fmt.Printf("Demo instantiated at %s\n", base)
}
