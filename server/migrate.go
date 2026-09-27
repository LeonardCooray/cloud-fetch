package server

import (
	"os"
	"path/filepath"
)

// DefaultConfigName is the config file used when --config-path isn't given.
const DefaultConfigName = "cloud-fetch.json"

const legacyConfigName = "cloud-torrent.json"

// migrateLegacyConfig renames a pre-rename cloud-torrent.json sitting next to
// the default config, so existing installs keep their settings. A custom
// --config-path, or an existing new file, is left untouched.
func migrateLegacyConfig(configPath string) (bool, error) {
	if filepath.Base(configPath) != DefaultConfigName {
		return false, nil
	}
	if _, err := os.Stat(configPath); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}
	legacy := filepath.Join(filepath.Dir(configPath), legacyConfigName)
	if _, err := os.Stat(legacy); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err := os.Rename(legacy, configPath); err != nil {
		return false, err
	}
	return true, nil
}
