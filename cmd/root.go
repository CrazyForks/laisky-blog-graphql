package cmd

import (
	"context"
	"fmt"
	"time"

	blog "laisky-blog-graphql/internal/web/blog/controller"
	general "laisky-blog-graphql/internal/web/general/controller"
	telegram "laisky-blog-graphql/internal/web/telegram/controller"
	twitter "laisky-blog-graphql/internal/web/twitter/controller"
	"laisky-blog-graphql/library/auth"
	"laisky-blog-graphql/library/config"
	"laisky-blog-graphql/library/jwt"
	"laisky-blog-graphql/library/log"

	gconfig "github.com/Laisky/go-config"
	gutils "github.com/Laisky/go-utils/v2"
	gcmd "github.com/Laisky/go-utils/v2/cmd"
	glog "github.com/Laisky/go-utils/v2/log"
	"github.com/Laisky/zap"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

var rootCMD = &cobra.Command{
	Use:   "laisky-blog-graphql",
	Short: "laisky-blog-graphql",
	Long:  `graphql API service for laisky`,
	Args:  gcmd.NoExtraArgs,
}

func initialize(ctx context.Context, cmd *cobra.Command) error {
	if err := gconfig.Shared.BindPFlags(cmd.Flags()); err != nil {
		return errors.Wrap(err, "bind pflags")
	}

	setupSettings(ctx)
	setupLogger(ctx)
	setupLibrary(ctx)
	setupModules(ctx)

	return nil
}

func setupModules(ctx context.Context) {
	blog.Initialize(ctx)
	twitter.Initialize(ctx)
	telegram.Initialize(ctx)
	general.Initialize(ctx)
}

func setupLibrary(ctx context.Context) {
	if err := auth.Initialize([]byte(gconfig.Shared.GetString("settings.secret"))); err != nil {
		log.Logger.Panic("init jwt", zap.Error(err))
	}

	if err := jwt.Initialize([]byte(gconfig.Shared.GetString("settings.secret"))); err != nil {
		log.Logger.Panic("setup jwt", zap.Error(err))
	}

}

func setupSettings(ctx context.Context) {
	// mode
	if gconfig.Shared.GetBool("debug") {
		fmt.Println("run in debug mode")
		gconfig.Shared.Set("log-level", "debug")
	} else { // prod mode
		fmt.Println("run in prod mode")
	}

	// clock
	gutils.SetInternalClock(100 * time.Millisecond)

	// load configuration
	cfgPath := gconfig.Shared.GetString("config")
	config.LoadFromFile(cfgPath)
}

func setupLogger(ctx context.Context) {
	// log
	// alertPusher, err := gutils.NewAlertPusherWithAlertType(
	// 	ctx,
	// 	gconfig.Shared.GetString("settings.logger.push_api"),
	// 	gconfig.Shared.GetString("settings.logger.alert_type"),
	// 	gconfig.Shared.GetString("settings.logger.push_token"),
	// )
	// if err != nil {
	// 	log.Logger.Panic("create AlertPusher", zap.Error(err))
	// }
	//
	// library.Logger = log.Logger.WithOptions(
	// 	zap.HooksWithFields(alertPusher.GetZapHook()),
	// ).Named("laisky-graphql")

	lvl := gconfig.Shared.GetString("log-level")
	if err := log.Logger.ChangeLevel(glog.Level(lvl)); err != nil {
		log.Logger.Panic("change log level", zap.Error(err), zap.String("level", lvl))
	}
}

func init() {
	rootCMD.PersistentFlags().Bool("debug", false, "run in debug mode")
	rootCMD.PersistentFlags().Bool("dry", false, "run in dry mode")
	rootCMD.PersistentFlags().String("listen", "localhost:8080", "like `localhost:8080`")
	rootCMD.PersistentFlags().StringP("config", "c", "/etc/laisky-blog-graphql/settings.yml", "config file path")
	rootCMD.PersistentFlags().String("log-level", "info", "`debug/info/error`")
	rootCMD.PersistentFlags().StringSliceP("tasks", "t", []string{},
		"which tasks want to runnning, like\n ./main -t t1,t2,heartbeat")
	rootCMD.PersistentFlags().Int("heartbeat", 60, "heartbeat seconds")
}

func Execute() {
	if err := rootCMD.Execute(); err != nil {
		glog.Shared.Panic("start", zap.Error(err))
	}
}
