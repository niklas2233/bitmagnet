package hashfetcherfx

import (
	"github.com/bitmagnet-io/bitmagnet/internal/config/configfx"
	"github.com/bitmagnet-io/bitmagnet/internal/hashfetcher"
	"go.uber.org/fx"
)

func New() fx.Option {
	return fx.Module(
		"hash_fetcher",
		configfx.NewConfigModule[hashfetcher.Config]("hash_fetcher", hashfetcher.NewDefaultConfig()),
		fx.Provide(hashfetcher.New),
	)
}
