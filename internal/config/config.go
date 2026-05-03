package config

import (
	"embed"
	"fmt"
	"log"
	"os"
	"os/user"
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

	usr, err := user.Current()
	if err != nil {
		log.Fatalf("could not get current user: %v", err)
	}
	
	expandPath := func(p string) string {
		if strings.HasPrefix(p, "~/") {
			return filepath.Join(usr.HomeDir, p[2:])
		}
		return p
	}

	for i := range config.FullBulbs {
		config.FullBulbs[i].Path = expandPath(config.FullBulbs[i].Path)
	}
	for i := range config.SemiBulbs {
		config.SemiBulbs[i].Path = expandPath(config.SemiBulbs[i].Path)
	}

	// Master theme override check
	drakoConfigPath := filepath.Join(usr.HomeDir, ".config/drako/config.toml")
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
