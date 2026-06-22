package metafetcher

import (
	"context"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/dhtcrawler"
	"github.com/bitmagnet-io/bitmagnet/internal/lazy"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
	"github.com/bitmagnet-io/bitmagnet/internal/worker"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type Params struct {
	fx.In
	Config       Config
	Dao          lazy.Lazy[*dao.Query]
	InfoHashSeed dhtcrawler.SeedInfoHash `name:"dht_infohash_seed"`
	Logger       *zap.SugaredLogger
}

type Result struct {
	fx.Out
	Worker worker.Worker `group:"workers"`
}

func New(p Params) Result {
	stop := make(chan struct{})
	return Result{
		Worker: worker.NewWorker("metafetcher", fx.Hook{
			OnStart: func(_ context.Context) error {
				go run(p, stop) //nolint:contextcheck
				return nil
			},
			OnStop: func(_ context.Context) error {
				close(stop)
				return nil
			},
		}),
	}
}

func run(p Params, stop chan struct{}) {
	logger := p.Logger.Named("metafetcher")

	fetch(p, logger)

	ticker := time.NewTicker(p.Config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			fetch(p, logger)
		}
	}
}

func fetch(p Params, logger *zap.SugaredLogger) {
	q, err := p.Dao.Get()
	if err != nil {
		logger.Warnw("dao unavailable", "error", err)
		return
	}

	type row struct {
		InfoHash protocol.ID
	}

	var rows []row
	if err := q.Torrent.WithContext(context.Background()).
		UnderlyingDB().
		Select("info_hash").
		Where("files_status = ?", string(model.FilesStatusNoInfo)).
		Limit(p.Config.BatchSize).
		Find(&rows).Error; err != nil {
		logger.Warnw("query failed", "error", err)
		return
	}

	seeded := 0

	for _, r := range rows {
		if p.InfoHashSeed.Seed(r.InfoHash) {
			seeded++
		}
	}

	if len(rows) > 0 {
		logger.Infow("seeded infohashes for metainfo fetch", "found", len(rows), "seeded", seeded)
	}
}
