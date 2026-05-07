package domain

import (
	"path/filepath"
)

// Project represents a project file.
type Project struct {
	Name     string
	Path     string
	Category string
	Status   string
}

// Board represents a single workspace ("bulb").
type Board struct {
	Name          string
	Grid          map[string]map[string][]Project
	HiddenGrid    map[string]map[string][]Project
	CategoryOrder []string
	Statuses      []string
	Opts          BoardOptions
}

// ActiveGrid cleanly isolates UI fetching from state filtering
func (b *Board) ActiveGrid(showHidden bool) map[string]map[string][]Project {
	if showHidden {
		return b.HiddenGrid
	}
	return b.Grid
}

type BulbConfig struct {
	Path     string   `toml:"path"`
	Statuses []string `toml:"statuses"`
}

type BoardOptions struct {
	Path                string
	Name                string
	Extension           string
	Statuses            []string
	ShowEmptyCategories bool
}

type Config struct {
	Theme          string       `toml:"theme"`
	FullBulbs      []BulbConfig `toml:"full-bulb"`
	SemiBulbs      []BulbConfig `toml:"semi-bulb"`
	Editor         string       `toml:"editor"`
	FileManager    string       `toml:"file_manager"`
	AltModifier    string       `toml:"alt_modifier"`
	AltEditor      string       `toml:"alt_editor"`
	AltFileManager string       `toml:"alt_file_manager"`
}

func (c Config) GetBoardOptions() []BoardOptions {
	var opts []BoardOptions
	for _, bulb := range c.FullBulbs {
		opts = append(opts, BoardOptions{
			Path:                bulb.Path,
			Name:                filepath.Base(bulb.Path),
			Extension:           ".md",
			Statuses:            bulb.Statuses,
			ShowEmptyCategories: true,
		})
	}
	for _, bulb := range c.SemiBulbs {
		opts = append(opts, BoardOptions{
			Path:                bulb.Path,
			Name:                filepath.Base(bulb.Path),
			Extension:           ".clove.md",
			Statuses:            bulb.Statuses,
			ShowEmptyCategories: false,
		})
	}
	return opts
}

type Theme struct {
	Primary    string `toml:"Primary"`
	Secondary  string `toml:"Secondary"`
	Background string `toml:"Background"`
	Foreground string `toml:"Foreground"`
	Comment    string `toml:"Comment"`
	Success    string `toml:"Success"`
	Warning    string `toml:"Warning"`
	Error      string `toml:"Error"`
	Info       string `toml:"Info"`
	Accent     string `toml:"Accent"`
}

type Themes map[string]Theme
