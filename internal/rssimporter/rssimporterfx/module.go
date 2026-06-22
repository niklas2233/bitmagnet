package rssimporterfx

import (
	"github.com/bitmagnet-io/bitmagnet/internal/config/configfx"
	"github.com/bitmagnet-io/bitmagnet/internal/rssimporter"
	"go.uber.org/fx"
)

func New() fx.Option {
	return fx.Module(
		"rss_importer",
		configfx.NewConfigModule[rssimporter.Config]("rss_importer", rssimporter.NewDefaultConfig()),
		fx.Provide(rssimporter.New),
	)
}
