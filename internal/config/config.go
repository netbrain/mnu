package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/netbrain/mnu/internal/util"
	"github.com/spf13/viper"
)

type Config struct {
	ClipboardTimeout time.Duration `mapstructure:"clipboard_timeout"`
	ApiMode          bool          `mapstructure:"api_mode"`
}

func Load() (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	home, err := os.UserHomeDir()
	if err == nil {
		v.AddConfigPath(filepath.Join(home, ".config", "mnu"))
	}
	v.AddConfigPath(".")

	v.SetDefault("clipboard_timeout", 15*time.Second)
	v.SetDefault("api_mode", true)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Create a default config file in ~/.config/mnu/config.yaml
			cfgDir, derr := util.GetConfigDir()
			if derr == nil {
				cfgPath := filepath.Join(cfgDir, "config.yaml")
				defaultContents := fmt.Sprintf("clipboard_timeout: %s\napi_mode: true\n", (15 * time.Second).String())
				_ = os.WriteFile(cfgPath, []byte(defaultContents), 0600)
				v.SetConfigFile(cfgPath)
				_ = v.ReadInConfig()
			}
		} else {
			return nil, err
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
