package rssimporter

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/bitmagnet-io/bitmagnet/internal/importer"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type poller struct {
	config       Config
	importer     importer.Importer
	db           *gorm.DB
	logger       *zap.SugaredLogger
	stop         chan struct{}
	torrentCache sync.Map // download URL → infohash hex string
}

func (p *poller) start() {
	p.poll()

	ticker := time.NewTicker(p.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.poll()
		case <-p.stop:
			return
		}
	}
}

func (p *poller) poll() {
	feeds := p.allFeeds()
	for _, feed := range feeds {
		p.pollFeed(feed)
	}
}

func (p *poller) allFeeds() []FeedConfig {
	feeds := append([]FeedConfig{}, p.config.Feeds...)

	if p.db != nil {
		var dbFeeds []model.RssFeed

		if err := p.db.Find(&dbFeeds).Error; err != nil {
			p.logger.Warnw("failed to load rss feeds from db", "error", err)
		} else {
			for _, f := range dbFeeds {
				feeds = append(feeds, FeedConfig{URL: f.URL, Source: f.Source})
			}
		}
	}

	return feeds
}

func (p *poller) pollFeed(feed FeedConfig) {
	resp, err := http.Get(feed.URL) //nolint:noctx
	if err != nil {
		p.logger.Warnw("rss fetch failed", "url", feed.URL, "error", err)
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Warnw("rss read failed", "url", feed.URL, "error", err)
		return
	}

	items, err := parseRSSFeed(data)
	if err != nil {
		p.logger.Warnw("rss parse failed", "url", feed.URL, "error", err)
		return
	}

	source := feed.Source
	if source == "" {
		source = feed.URL
	}

	importID := fmt.Sprintf("rss_%s_%s", source, strconv.FormatInt(time.Now().Unix(), 10))
	ai := p.importer.New(context.Background(), importer.Info{ID: importID, Priority: 20})

	p.logger.Debugw("fetched rss items", "source", source, "count", len(items))

	count := 0
	consecutiveRateLimit := 0

	for _, item := range items {
		hashStr, ok := extractInfoHash(item)
		if !ok {
			// Fall back to downloading the torrent file and extracting the infohash
			downloadURL := item.Enclosure.URL
			if downloadURL == "" {
				downloadURL = item.Link
			}

			if downloadURL == "" {
				p.logger.Debugw("no download url", "title", item.Title)
				continue
			}

			if p.config.DownloadDelay > 0 {
				time.Sleep(p.config.DownloadDelay)
			}

			var rateLimited bool

			hashStr, ok, rateLimited = p.infoHashFromDownload(downloadURL)
			if rateLimited {
				consecutiveRateLimit++
				if consecutiveRateLimit >= 3 {
					p.logger.Warnw("aborting poll: indexer is rate limiting downloads, will retry next cycle", "source", source)
					break
				}
			} else {
				consecutiveRateLimit = 0
			}

			if !ok {
				p.logger.Warnw("could not extract infohash", "title", item.Title)
				continue
			}
		} else {
			consecutiveRateLimit = 0
		}

		infoHash, err := protocol.ParseID(hashStr)
		if err != nil {
			p.logger.Debugw("invalid infohash", "hash", hashStr, "error", err)
			continue
		}

		size := extractSize(item)
		seeders := extractSeeders(item)
		leechers := extractLeechers(item)

		publishedAt := parsePubDate(item.PubDate)
		if publishedAt.IsZero() {
			publishedAt = time.Now()
		}

		if err := ai.Import(importer.Item{
			Source:      source,
			InfoHash:    infoHash,
			Name:        item.Title,
			Size:        size,
			Seeders:     seeders,
			Leechers:    leechers,
			PublishedAt: publishedAt,
		}); err != nil {
			p.logger.Warnw("import failed", "title", item.Title, "error", err)
			continue
		}

		count++
	}

	ai.Drain()

	if err := ai.Close(); err != nil {
		p.logger.Warnw("import close error", "source", source, "error", err)
	} else {
		p.logger.Infow("rss poll complete", "source", source, "imported", count)
	}
}

// noRedirectClient stops at the first redirect so we can inspect Location headers
// (Prowlarr download URLs redirect to magnet: links which http.Client can't follow).
var noRedirectClient = &http.Client{
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func (p *poller) infoHashFromDownload(downloadURL string) (hashStr string, ok bool, rateLimited bool) {
	if cached, hit := p.torrentCache.Load(downloadURL); hit {
		return cached.(string), true, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		p.logger.Warnw("torrent request build failed", "error", err)
		return "", false, false
	}

	resp, err := noRedirectClient.Do(req)
	if err != nil {
		p.logger.Warnw("torrent download failed", "error", err)
		return "", false, false
	}
	defer resp.Body.Close()

	// Handle redirect — Prowlarr redirects download URLs to magnet links
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		if hash, ok := hashFromMagnet(location); ok {
			p.torrentCache.Store(downloadURL, hash)
			return hash, true, false
		}

		p.logger.Warnw("redirect to non-magnet location", "location", location)

		return "", false, false
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		p.logger.Warnw("torrent download rate limited (429)", "url", downloadURL)
		return "", false, true
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		p.logger.Warnw("torrent download failed", "status", resp.StatusCode, "url", downloadURL)
		return "", false, false
	}

	// Direct torrent file response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Warnw("torrent read failed", "error", err)
		return "", false, false
	}

	mi, err := metainfo.Load(bytes.NewReader(body))
	if err != nil {
		p.logger.Warnw("torrent parse failed", "error", err, "preview", string(body[:min(120, len(body))]))
		return "", false, false
	}

	h := mi.HashInfoBytes()
	hs := hex.EncodeToString(h[:])
	p.torrentCache.Store(downloadURL, hs)

	return hs, true, false
}
