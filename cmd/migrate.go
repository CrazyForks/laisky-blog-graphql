// Package cmd command line
package cmd

import (
	"context"

	"github.com/Laisky/laisky-blog-graphql/library/log"

	gcmd "github.com/Laisky/go-utils/v5/cmd"
	"github.com/Laisky/zap"
	"github.com/spf13/cobra"
)

var migrateCMD = &cobra.Command{
	Use:   "migrate",
	Short: "migrate",
	Long:  `migrate db`,
	Args:  gcmd.NoExtraArgs,
	PreRun: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		if err := initialize(ctx, cmd); err != nil {
			log.Logger.Panic("init", zap.Error(err))
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		// if err := model.SearchDB.AutoMigrate(
		// 	model.SearchTweet{},
		// ); err != nil {
		// 	log.Logger.Panic("migrate", zap.Error(err))
		// }
	},
}

func init() {
	rootCMD.AddCommand(migrateCMD)
}
