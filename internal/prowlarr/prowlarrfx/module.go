package prowlarrfx

import (
	"github.com/bitmagnet-io/bitmagnet/internal/prowlarr"
	"go.uber.org/fx"
)

func New() fx.Option {
	return fx.Module(
		"prowlarr",
		fx.Provide(prowlarr.New),
	)
}
