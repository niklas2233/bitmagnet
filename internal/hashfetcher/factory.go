package hashfetcher

import (
	"context"

	"github.com/bitmagnet-io/bitmagnet/internal/blocking"
	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	dhtclient "github.com/bitmagnet-io/bitmagnet/internal/protocol/dht/client"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol/dht/ktable"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol/metainfo/banning"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol/metainfo/metainforequester"
	"github.com/bitmagnet-io/bitmagnet/internal/worker"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Params struct {
	fx.In
	Config          Config
	Client          lazy.Lazy[dhtclient.Client]
	KTable          ktable.Table
	Requester       metainforequester.Requester
	BanningChecker  banning.Checker `name:"metainfo_banning_checker"`
	BlockingManager lazy.Lazy[blocking.Manager]
	Dao             lazy.Lazy[*dao.Query]
	Logger          *zap.SugaredLogger
}

type Result struct {
	fx.Out
	Worker worker.Worker `group:"workers"`
}

func New(p Params) Result {
	stop := make(chan struct{})

	return Result{
		Worker: worker.NewWorker("hash_fetcher", fx.Hook{
			OnStart: func(_ context.Context) error {
				cl, err := p.Client.Get()
				if err != nil {
					return err
				}
				q, err := p.Dao.Get()
				if err != nil {
					return err
				}
				bm, err := p.BlockingManager.Get()
				if err != nil {
					return err
				}
				f := &fetcher{
					config:          p.Config,
					client:          cl,
					kTable:          p.KTable,
					requester:       p.Requester,
					banningChecker:  p.BanningChecker,
					blockingManager: bm,
					dao:             q,
					logger:          p.Logger.Named("hash_fetcher"),
					stop:            stop,
				}
				go f.start() //nolint:contextcheck
				return nil
			},
			OnStop: func(_ context.Context) error {
				close(stop)
				return nil
			},
		}),
	}
}
