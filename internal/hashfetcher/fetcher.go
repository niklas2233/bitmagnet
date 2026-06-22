package hashfetcher

import (
	"context"
	"errors"
	"net/netip"
	"sort"
	"sync"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/blocking"
	"github.com/bitmagnet-io/bitmagnet/internal/database/dao"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/processor"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
	dhtclient "github.com/bitmagnet-io/bitmagnet/internal/protocol/dht/client"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol/dht/ktable"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol/metainfo"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol/metainfo/banning"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol/metainfo/metainforequester"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"
	"gorm.io/gorm/clause"
)

const alpha = 5 // Kademlia parallelism factor for iterative get_peers

type fetcher struct {
	config          Config
	client          dhtclient.Client
	kTable          ktable.Table
	requester       metainforequester.Requester
	banningChecker  banning.Checker
	blockingManager blocking.Manager
	dao             *dao.Query
	logger          *zap.SugaredLogger
	stop            chan struct{}
}

func (f *fetcher) start() {
	f.run(context.Background())
}

func (f *fetcher) run(ctx context.Context) {
	f.processBatch(ctx)

	ticker := time.NewTicker(f.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-f.stop:
			return
		case <-ticker.C:
			f.processBatch(ctx)
		}
	}
}

func (f *fetcher) processBatch(ctx context.Context) {
	type row struct {
		InfoHash protocol.ID
	}

	var rows []row
	if err := f.dao.Torrent.WithContext(ctx).
		UnderlyingDB().
		Select("info_hash").
		Where("files_status = ?", string(model.FilesStatusNoInfo)).
		Limit(f.config.BatchSize).
		Find(&rows).Error; err != nil {
		f.logger.Warnw("query failed", "error", err)
		return
	}

	if len(rows) == 0 {
		return
	}

	f.logger.Infow("starting targeted DHT fetch", "count", len(rows))

	sem := semaphore.NewWeighted(int64(f.config.Concurrency))

	var wg sync.WaitGroup

	resolved := 0

	var mu sync.Mutex

	for _, r := range rows {
		if err := sem.Acquire(ctx, 1); err != nil {
			return
		}

		wg.Add(1)

		go func(hash protocol.ID) {
			defer sem.Release(1)
			defer wg.Done()

			if err := f.processHash(ctx, hash); err == nil {
				mu.Lock()
				resolved++
				mu.Unlock()
			}
		}(r.InfoHash)
	}

	wg.Wait()

	f.logger.Infow("targeted DHT fetch complete", "found", len(rows), "resolved", resolved)
}

func (f *fetcher) processHash(ctx context.Context, hash protocol.ID) error {
	peers, err := f.iterativeGetPeers(ctx, hash)
	if err != nil || len(peers) == 0 {
		return errors.New("no peers found")
	}

	info, err := f.fetchMetaInfo(ctx, hash, peers)
	if err != nil {
		return err
	}

	return f.persist(ctx, hash, info)
}

// iterativeGetPeers performs a Kademlia-style iterative get_peers lookup.
// It starts with the closest nodes from the shared routing table and follows
// closer nodes hop by hop until peers are found or hops are exhausted.
func (f *fetcher) iterativeGetPeers(
	ctx context.Context, hash protocol.ID,
) ([]netip.AddrPort, error) {
	if len(f.kTable.GetClosestNodes(hash)) == 0 {
		return nil, errors.New("routing table empty")
	}

	seen := make(map[string]bool)
	frontier := make([]nodeCandidate, 0, 20)

	for _, n := range f.kTable.GetClosestNodes(hash) {
		addr := n.Addr()

		key := addr.String()
		if !seen[key] {
			seen[key] = true

			frontier = append(frontier, nodeCandidate{addr: addr, id: n.ID()})
		}
	}

	for range f.config.GetPeersHops {
		if len(frontier) == 0 {
			break
		}

		size := alpha
		if len(frontier) < size {
			size = len(frontier)
		}

		batch := frontier[:size]
		frontier = frontier[size:]

		var mu sync.Mutex

		var foundPeers []netip.AddrPort

		var newCandidates []nodeCandidate

		var wg sync.WaitGroup

		for _, c := range batch {
			wg.Add(1)

			go func(addr netip.AddrPort) {
				defer wg.Done()

				hopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				res, err := f.client.GetPeers(hopCtx, addr, hash)
				if err != nil {
					return
				}

				mu.Lock()
				defer mu.Unlock()

				foundPeers = append(foundPeers, res.Values...)

				for _, n := range res.Nodes {
					key := n.Addr.String()

					if !seen[key] {
						seen[key] = true

						newCandidates = append(
							newCandidates,
							nodeCandidate{addr: n.Addr, id: n.ID},
						)
					}
				}
			}(c.addr)
		}

		wg.Wait()

		if len(foundPeers) > 0 {
			return foundPeers, nil
		}

		// Sort new candidates by XOR distance to hash (closer first) and prepend to frontier
		sort.Slice(newCandidates, func(i, j int) bool {
			return xorLess(newCandidates[i].id, newCandidates[j].id, hash)
		})

		frontier = append(newCandidates, frontier...)
	}

	return nil, errors.New("no peers found after iterative lookup")
}

type nodeCandidate struct {
	addr netip.AddrPort
	id   protocol.ID
}

// xorLess reports whether a is closer to target than b in XOR metric.
func xorLess(a, b, target protocol.ID) bool {
	for i := range a {
		ai := a[i] ^ target[i]
		bi := b[i] ^ target[i]

		if ai != bi {
			return ai < bi
		}
	}

	return false
}

func (f *fetcher) fetchMetaInfo(
	ctx context.Context, hash protocol.ID, peers []netip.AddrPort,
) (metainfo.Info, error) {
	var errs []error

	for _, p := range peers {
		res, err := f.requester.Request(ctx, hash, p)
		if err != nil {
			errs = append(errs, err)

			continue
		}

		if banErr := f.banningChecker.Check(res.Info); banErr != nil {
			_ = f.blockingManager.Block(ctx, []protocol.ID{hash}, false)

			return metainfo.Info{}, banErr
		}

		return res.Info, nil
	}

	return metainfo.Info{}, errors.Join(errs...)
}

func (f *fetcher) persist(ctx context.Context, hash protocol.ID, info metainfo.Info) error {
	name := info.BestName()

	filesStatus := model.FilesStatusSingle

	var filesCount model.NullUint

	var files []model.TorrentFile

	const saveFilesThreshold = 100

	if len(info.Files) > 0 {
		filesStatus = model.FilesStatusMulti
		filesCount = model.NewNullUint(uint(len(info.Files)))

		for i, file := range info.Files {
			if i >= saveFilesThreshold {
				filesStatus = model.FilesStatusOverThreshold

				break
			}

			files = append(files, model.TorrentFile{
				InfoHash: hash,
				Index:    uint(i),
				Path:     file.DisplayPath(&info),
				Size:     uint(file.Length),
			})
		}
	}

	private := false
	if info.Private != nil {
		private = *info.Private
	}

	torrent := model.Torrent{
		InfoHash:    hash,
		Name:        name,
		Size:        uint(info.TotalLength()),
		Private:     private,
		FilesStatus: filesStatus,
		FilesCount:  filesCount,
	}

	source := model.TorrentsTorrentSource{
		Source:   "dht",
		InfoHash: hash,
	}

	job, jobErr := processor.NewQueueJob(
		processor.MessageParams{InfoHashes: []protocol.ID{hash}},
		model.QueueJobDelayBy(time.Minute),
		model.QueueJobPriority(10),
	)
	if jobErr != nil {
		return jobErr
	}

	return f.dao.Transaction(func(tx *dao.Query) error {
		if err := tx.WithContext(ctx).Torrent.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: string(tx.Torrent.InfoHash.ColumnName())}},
			DoUpdates: clause.AssignmentColumns([]string{
				string(tx.Torrent.Name.ColumnName()),
				string(tx.Torrent.FilesStatus.ColumnName()),
				string(tx.Torrent.FilesCount.ColumnName()),
				string(tx.Torrent.Size.ColumnName()),
				string(tx.Torrent.UpdatedAt.ColumnName()),
			}),
		}).Create(&torrent); err != nil {
			return err
		}

		if len(files) > 0 {
			filePtrs := make([]*model.TorrentFile, len(files))
			for i := range files {
				filePtrs[i] = &files[i]
			}

			if err := tx.WithContext(ctx).TorrentFile.Clauses(clause.OnConflict{
				DoNothing: true,
			}).CreateInBatches(filePtrs, 100); err != nil {
				return err
			}
		}

		if err := tx.WithContext(ctx).TorrentsTorrentSource.Clauses(clause.OnConflict{
			DoNothing: true,
		}).Create(&source); err != nil {
			return err
		}

		return tx.WithContext(ctx).QueueJob.Clauses(clause.OnConflict{
			DoNothing: true,
		}).Create(&job)
	})
}
