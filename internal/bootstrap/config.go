package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

// ConfigFormatVersion is the version written to and required from
// config.toml.
const ConfigFormatVersion = 1

// Config is the persisted installation configuration: only the format
// version and the absolute Store and state database paths.
type Config struct {
	Version     int    `toml:"version"`
	StorePath   string `toml:"store_path"`
	StateDBPath string `toml:"state_db_path"`
}

func readConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	return parseConfig(data)
}

func parseConfig(data []byte) (Config, error) {
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	switch {
	case cfg.Version != ConfigFormatVersion:
		return cfg, fmt.Errorf("unsupported config format version %d", cfg.Version)
	case cfg.StorePath == "" || !filepath.IsAbs(cfg.StorePath):
		return cfg, fmt.Errorf("store_path must be an absolute path")
	case cfg.StateDBPath == "" || !filepath.IsAbs(cfg.StateDBPath):
		return cfg, fmt.Errorf("state_db_path must be an absolute path")
	}
	return cfg, nil
}

// writeConfigAtomic writes the configuration through a temporary file and an
// atomic rename, without any backup or version history.
func writeConfigAtomic(path string, cfg Config) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config.toml.*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
