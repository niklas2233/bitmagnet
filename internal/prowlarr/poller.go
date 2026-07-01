package prowlarr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/importer"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
	"github.com/bitmagnet-io/bitmagnet/internal/rssimporter"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const sourceKey = "prowlarr"

type poller struct {
	importer importer.Importer
	db       *gorm.DB
	logger   *zap.SugaredLogger
	stop     chan struct{}
	trigger  chan struct{}
}

func (p *poller) start() {
	p.poll()

	ticker := time.NewTicker(defaultPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.poll()
		case <-p.trigger:
			p.poll()
		case <-p.stop:
			return
		}
	}
}

func (p *poller) poll() {
	ctx := context.Background()

	var cfg model.ProwlarrConfig
	if err := p.db.WithContext(ctx).First(&cfg).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			p.logger.Warnw("prowlarr: failed to read config", "error", err)
		}

		return
	}

	if !cfg.Enabled || cfg.BaseURL == "" {
		return
	}

	// Ensure the prowlarr TorrentSource row exists.
	if err := p.db.WithContext(ctx).FirstOrCreate(&model.TorrentSource{
		Key:  sourceKey,
		Name: "Prowlarr",
	}, model.TorrentSource{Key: sourceKey}).Error; err != nil {
		p.logger.Warnw("prowlarr: failed to upsert torrent source", "error", err)
		return
	}

	indexers, err := listIndexers(ctx, cfg.BaseURL, cfg.APIKey)
	if err != nil {
		p.logger.Warnw("prowlarr: failed to list indexers", "error", err)
		return
	}

	importID := fmt.Sprintf("prowlarr_%s", strconv.FormatInt(time.Now().Unix(), 10))
	ai := p.importer.New(ctx, importer.Info{ID: importID, Priority: 20})

	for _, idx := range indexers {
		if !idx.Enable || !idx.SupportsRss || idx.Protocol != "torrent" || idx.DefinitionName == "bitmagnet" {
			continue
		}

		p.pollIndexer(idx, cfg.BaseURL, cfg.APIKey, ai)
	}

	ai.Drain()

	if err := ai.Close(); err != nil {
		p.logger.Warnw("prowlarr: import close error", "error", err)
	}
}

func (p *poller) pollIndexer(idx Indexer, baseURL, apiKey string, ai importer.ActiveImport) {
	feedURL := fmt.Sprintf("%s/%d/api?apikey=%s&t=search&limit=100", baseURL, idx.ID, apiKey)

	resp, err := http.Get(feedURL) //nolint:noctx
	if err != nil {
		p.logger.Warnw("prowlarr: rss fetch failed", "indexer", idx.Name, "error", err)
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Warnw("prowlarr: rss read failed", "indexer", idx.Name, "error", err)
		return
	}

	items, err := rssimporter.ParseFeed(data)
	if err != nil {
		p.logger.Warnw("prowlarr: rss parse failed", "indexer", idx.Name, "error", err)
		return
	}

	count := 0

	for _, item := range items {
		hashStr, ok := rssimporter.ExtractInfoHash(item)
		if !ok {
			p.logger.Debugw("prowlarr: item missing infohash, skipping", "title", item.Title)
			continue
		}

		infoHash, err := protocol.ParseID(hashStr)
		if err != nil {
			p.logger.Debugw("prowlarr: invalid infohash", "hash", hashStr, "error", err)
			continue
		}

		publishedAt := rssimporter.ParsePubDate(item.PubDate)
		if publishedAt.IsZero() {
			publishedAt = time.Now()
		}

		if err := ai.Import(importer.Item{
			Source:      sourceKey,
			InfoHash:    infoHash,
			Name:        item.Title,
			Size:        rssimporter.ExtractSize(item),
			Seeders:     rssimporter.ExtractSeeders(item),
			Leechers:    rssimporter.ExtractLeechers(item),
			PublishedAt: publishedAt,
		}); err != nil {
			p.logger.Warnw("prowlarr: import failed", "title", item.Title, "error", err)
			continue
		}

		count++
	}

	p.logger.Infow("prowlarr: indexer poll complete", "indexer", idx.Name, "imported", count)
}
