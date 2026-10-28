package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows/registry"
)

// Settings — то, что лежит в config.json (%APPDATA%\dontsleep\config.json).
type Settings struct {
	Autostart bool   `json:"autostart"`
	Hotkey    string `json:"hotkey"`
}

// Config — настройки в памяти с сохранением на диск.
type Config struct {
	mu   sync.RWMutex
	path string
	data Settings
}

func defaultSettings() Settings {
	return Settings{Autostart: true, Hotkey: "Ctrl+Shift+Space"}
}

func dataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "dontsleep")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

func loadConfig(path string) (*Config, error) {
	cfg := &Config{path: path, data: defaultSettings()}

	raw, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(raw, &cfg.data); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	if cfg.data.Hotkey == "" {
		cfg.data.Hotkey = "Ctrl+Shift+Space"
	}
	return cfg, nil
}

// Get возвращает текущие настройки.
func (c *Config) Get() Settings {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data
}

// Update применяет изменения и пишет файл.
func (c *Config) Update(fn func(*Settings)) error {
	c.mu.Lock()
	fn(&c.data)
	raw, err := json.MarshalIndent(c.data, "", "  ")
	c.mu.Unlock()
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, raw, 0o644)
}

const (
	runKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "dontsleep"
)

// syncAutostart приводит запись в реестре в соответствие с настройкой.
func syncAutostart(enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()

	if enabled {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		return key.SetStringValue(runValueName, `"`+exe+`" -tray`)
	}

	if err := key.DeleteValue(runValueName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}
