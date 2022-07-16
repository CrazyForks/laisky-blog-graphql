package config

import (
	"path/filepath"

	"laisky-blog-graphql/library/log"

	gconfig "github.com/Laisky/go-config"
	"github.com/Laisky/zap"
)

func LoadFromFile(cfgPath string) {
	gconfig.Shared.Set("cfg_dir", filepath.Dir(cfgPath))
	if err := gconfig.Shared.LoadFromFile(cfgPath); err != nil {
		log.Logger.Panic("load configuration",
			zap.Error(err),
			zap.String("config", cfgPath))
	}

	log.Logger.Info("load configuration",
		zap.String("config", cfgPath))
}

func LoadTest() {
	LoadFromFile("/opt/configs/laisky-blog-graphql/settings.yml")
}
