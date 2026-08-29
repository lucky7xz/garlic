package domain

import (
	"fmt"
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
	// Ignore names path segments that never travel to or from a remote,
	// e.g. ["dist", "node_modules"]. .git is always excluded regardless.
	Ignore []string `toml:"ignore"`
}

type BoardOptions struct {
	Path                string
	Name                string
	Extension           string
	Statuses            []string
	ShowEmptyCategories bool
	// WholeFolder marks a semi bulb, where a category is not an area of
	// several projects but one project directory: a .clove.md inside it puts
	// the entire folder in play.
	WholeFolder bool
	Ignore      []string
}

// Remote is another machine garlic can plant to. It carries exactly what ssh
// cannot express in the destination string, and nothing more.
type Remote struct {
	Name         string `toml:"name"`
	Host         string `toml:"host"`
	Port         int    `toml:"port"`
	IdentityFile string `toml:"identity_file"`
	Root         string `toml:"root"`
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
	AsyncApps      []string     `toml:"async_apps"`
	Remotes        []Remote     `toml:"remote"`
}

// FindRemote resolves the name given after "@".
func (c Config) FindRemote(name string) (Remote, error) {
	for _, r := range c.Remotes {
		if r.Name == name {
			return r, nil
		}
	}
	if len(c.Remotes) == 0 {
		return Remote{}, fmt.Errorf("no remotes configured: add a [[remote]] block to your config.toml")
	}
	return Remote{}, fmt.Errorf("no remote named %q", name)
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
			Ignore:              bulb.Ignore,
		})
	}
	for _, bulb := range c.SemiBulbs {
		opts = append(opts, BoardOptions{
			Path:                bulb.Path,
			Name:                filepath.Base(bulb.Path),
			Extension:           ".clove.md",
			Statuses:            bulb.Statuses,
			ShowEmptyCategories: false,
			WholeFolder:         true,
			Ignore:              bulb.Ignore,
		})
	}
	return opts
}

type Theme struct {
	Primary    string `toml:"Primary"`
	Secondary  string `toml:"Secondary"`
	Foreground string `toml:"Foreground"`
	Comment    string `toml:"Comment"`
	Warning    string `toml:"Warning"`
	Accent     string `toml:"Accent"`
}

type Themes map[string]Theme
