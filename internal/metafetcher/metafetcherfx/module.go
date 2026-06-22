package metafetcherfx

import (
	"github.com/bitmagnet-io/bitmagnet/internal/config/configfx"
	"github.com/bitmagnet-io/bitmagnet/internal/metafetcher"
	"go.uber.org/fx"
)

func New() fx.Option {
	return fx.Module(
		"metafetcher",
		configfx.NewConfigModule[metafetcher.Config]("metafetcher", metafetcher.NewDefaultConfig()),
		fx.Provide(metafetcher.New),
	)
}
