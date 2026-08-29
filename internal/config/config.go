package config

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/lucky7xz/garlic/internal/domain"
)

//go:embed bootstrap/*
var bootstrapFS embed.FS

const (
	userConfigDir      = ".config/garlic"
	userConfigFileName = "config.toml"
	userThemesFileName = "themes.toml"
)

func EnsureUserFile(userDir, fileName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	userDirPath := filepath.Join(homeDir, userDir)
	userFilePath := filepath.Join(userDirPath, fileName)

	if _, err := os.Stat(userDirPath); os.IsNotExist(err) {
		if err := os.MkdirAll(userDirPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create user config directory: %w", err)
		}
	}

	if _, err := os.Stat(userFilePath); os.IsNotExist(err) {
		data, err := bootstrapFS.ReadFile("bootstrap/" + fileName)
		if err != nil {
			return "", fmt.Errorf("failed to read embedded bootstrap file %s: %w", fileName, err)
		}
		if err := os.WriteFile(userFilePath, data, 0644); err != nil {
			return "", fmt.Errorf("failed to write bootstrap file to user config: %w", err)
		}
	}

	return userFilePath, nil
}

func LoadConfig() (domain.Config, error) {
	configPath, err := EnsureUserFile(userConfigDir, userConfigFileName)
	if err != nil {
		return domain.Config{}, err
	}

	var config domain.Config
	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		if _, ok := err.(*os.PathError); ok {
			return domain.Config{}, nil
		}
		return domain.Config{}, fmt.Errorf("failed to decode config file: %w", err)
	}

	if config.AltModifier == "" {
		config.AltModifier = "alt"
	}

	// The same home EnsureUserFile used. os.UserHomeDir reads $HOME, which is
	// what a shell means by ~; user.Current reads the passwd database, and the
	// two disagree under sudo and inside containers -- which would mean reading
	// the config from one home and expanding its paths against another.
	home, err := os.UserHomeDir()
	if err != nil {
		return domain.Config{}, fmt.Errorf("failed to get user home directory: %w", err)
	}

	expandPath := func(p string) string {
		if strings.HasPrefix(p, "~/") {
			return filepath.Join(home, p[2:])
		}
		return p
	}

	for i := range config.FullBulbs {
		config.FullBulbs[i].Path = expandPath(config.FullBulbs[i].Path)
	}
	for i := range config.SemiBulbs {
		config.SemiBulbs[i].Path = expandPath(config.SemiBulbs[i].Path)
	}

	// A remote's identity file is ours, so it expands here. Its root belongs to
	// the other machine and must not: ~ there is not ~ here.
	for i := range config.Remotes {
		config.Remotes[i].IdentityFile = expandPath(config.Remotes[i].IdentityFile)
	}

	// Master theme override check
	drakoConfigPath := filepath.Join(home, ".config/drako/config.toml")
	if _, err := os.Stat(drakoConfigPath); err == nil {
		var drakoConfig struct {
			Theme string `toml:"theme"`
		}
		if _, err := toml.DecodeFile(drakoConfigPath, &drakoConfig); err == nil {
			if drakoConfig.Theme != "" {
				config.Theme = drakoConfig.Theme
			}
		}
	}

	return config, nil
}

func LoadThemes() (domain.Themes, error) {
	themesPath, err := EnsureUserFile(userConfigDir, userThemesFileName)
	if err != nil {
		return nil, err
	}

	var themes domain.Themes
	if _, err := toml.DecodeFile(themesPath, &themes); err != nil {
		return nil, fmt.Errorf("failed to decode themes file: %w", err)
	}
	return themes, nil
}
