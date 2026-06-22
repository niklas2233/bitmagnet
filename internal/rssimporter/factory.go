package rssimporter

import (
	"context"

	"github.com/bitmagnet-io/bitmagnet/internal/httpserver"
	"github.com/bitmagnet-io/bitmagnet/internal/importer"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/worker"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Params struct {
	fx.In
	Config   Config
	Importer lazy.Lazy[importer.Importer]
	DB       lazy.Lazy[*gorm.DB]
	Logger   *zap.SugaredLogger
}

type Result struct {
	fx.Out
	Worker worker.Worker     `group:"workers"`
	Option httpserver.Option `group:"http_server_options"`
}

func New(p Params) Result {
	var pol *poller
	return Result{
		Worker: worker.NewWorker("rss_importer", fx.Hook{
			OnStart: func(_ context.Context) error {
				imp, err := p.Importer.Get()
				if err != nil {
					return err
				}
				db, err := p.DB.Get()
				if err != nil {
					return err
				}
				pol = &poller{
					config:   p.Config,
					importer: imp,
					db:       db,
					logger:   p.Logger.Named("rss_importer"),
					stop:     make(chan struct{}),
				}
				go pol.start() //nolint:contextcheck
				return nil
			},
			OnStop: func(_ context.Context) error {
				if pol != nil {
					close(pol.stop)
				}
				return nil
			},
		}),
		Option: apiHandler{lazyDB: p.DB},
	}
}
