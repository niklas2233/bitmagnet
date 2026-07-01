package adapter

import (
	"strconv"

	"github.com/bitmagnet-io/bitmagnet/internal/database/search"
	"github.com/bitmagnet-io/bitmagnet/internal/model"
	"github.com/bitmagnet-io/bitmagnet/internal/torznab"
)

func torrentContentResultToTorznabResult(
	req torznab.SearchRequest,
	res search.TorrentContentResult,
) torznab.SearchResult {
	entries := make([]torznab.SearchResultItem, 0, len(res.Items))
	for _, item := range res.Items {
		entries = append(entries, torrentContentResultItemToTorznabResultItem(item))
	}

	return torznab.SearchResult{
		Channel: torznab.SearchResultChannel{
			Title: req.Profile.Title,
			Response: torznab.SearchResultResponse{
				Offset: req.Offset.Uint,
				// Total:  res.TotalCount,
			},
			Items: entries,
		},
	}
}

func torrentContentResultItemToTorznabResultItem(item search.TorrentContentResultItem) torznab.SearchResultItem {
	category := "Unknown"
	categoryID := torznab.CategoryOther.ID

	if item.ContentType.Valid {
		switch item.ContentType.ContentType {
		case model.ContentTypeMovie:
			categoryID, category = movieCategory(item.VideoResolution)
		case model.ContentTypeTvShow:
			categoryID, category = tvCategory(item.VideoResolution)
		case model.ContentTypeMusic:
			categoryID, category = torznab.CategoryAudio.ID, "music"
		case model.ContentTypeEbook:
			categoryID, category = torznab.CategoryBooksEBook.ID, "ebook"
		case model.ContentTypeComic:
			categoryID, category = torznab.CategoryBooksComics.ID, "comic"
		case model.ContentTypeAudiobook:
			categoryID, category = torznab.CategoryAudioAudiobook.ID, "audiobook"
		case model.ContentTypeSoftware:
			categoryID, category = torznab.CategoryPC.ID, "software"
		case model.ContentTypeGame:
			categoryID, category = torznab.CategoryPCGames.ID, "game"
		case model.ContentTypeXxx:
			categoryID, category = torznab.CategoryXXX.ID, "xxx"
		}
	}

	attrs := []torznab.SearchResultItemTorznabAttr{
		{
			AttrName:  torznab.AttrInfoHash,
			AttrValue: item.Torrent.InfoHash.String(),
		},
		{
			AttrName:  torznab.AttrMagnetURL,
			AttrValue: item.Torrent.MagnetURI(),
		},
		{
			AttrName:  torznab.AttrCategory,
			AttrValue: strconv.Itoa(categoryID),
		},
		{
			AttrName:  torznab.AttrSize,
			AttrValue: strconv.FormatUint(uint64(item.Torrent.Size), 10),
		},
		{
			AttrName:  torznab.AttrPublishDate,
			AttrValue: item.PublishedAt.Format(torznab.RssDateDefaultFormat),
		},
	}
	seeders := item.Torrent.Seeders()
	leechers := item.Torrent.Leechers()

	if seeders.Valid {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrSeeders,
			AttrValue: strconv.Itoa(int(seeders.Uint)),
		})
	}

	if leechers.Valid {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrLeechers,
			AttrValue: strconv.Itoa(int(leechers.Uint)),
		})
	}

	if leechers.Valid && seeders.Valid {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrPeers,
			AttrValue: strconv.Itoa(int(leechers.Uint) + int(seeders.Uint)),
		})
	}

	if len(item.Torrent.Files) > 0 {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrFiles,
			AttrValue: strconv.Itoa(len(item.Torrent.Files)),
		})
	}

	if !item.Content.ReleaseYear.IsNil() {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrYear,
			AttrValue: strconv.Itoa(int(item.Content.ReleaseYear)),
		})
	}

	if len(item.Episodes) > 0 {
		// should we be adding all seasons and episodes here?
		seasons := item.Episodes.SeasonEntries()
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrSeason,
			AttrValue: strconv.Itoa(seasons[0].Season),
		})

		if len(seasons[0].Episodes) > 0 {
			attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
				AttrName:  torznab.AttrEpisode,
				AttrValue: strconv.Itoa(seasons[0].Episodes[0]),
			})
		}
	}

	if item.VideoCodec.Valid {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrVideo,
			AttrValue: item.VideoCodec.VideoCodec.Label(),
		})
	}

	if item.VideoResolution.Valid {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrResolution,
			AttrValue: item.VideoResolution.VideoResolution.Label(),
		})
	}

	if item.ReleaseGroup.Valid {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrTeam,
			AttrValue: item.ReleaseGroup.String,
		})
	}

	if tmdbID, ok := item.Content.Identifier("tmdb"); ok {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrTmdb,
			AttrValue: tmdbID,
		})
	}

	if imdbID, ok := item.Content.Identifier("imdb"); ok {
		attrs = append(attrs, torznab.SearchResultItemTorznabAttr{
			AttrName:  torznab.AttrImdb,
			AttrValue: imdbID[2:],
		})
	}

	return torznab.SearchResultItem{
		Title:    item.Torrent.Name,
		Size:     item.Torrent.Size,
		Category: category,
		GUID:     item.InfoHash.String(),
		PubDate:  torznab.RSSDate(item.PublishedAt),
		Enclosure: torznab.SearchResultItemEnclosure{
			URL:    item.Torrent.MagnetURI(),
			Type:   "application/x-bittorrent;x-scheme-handler/magnet",
			Length: strconv.FormatUint(uint64(item.Torrent.Size), 10),
		},
		TorznabAttrs: attrs,
	}
}

func tvCategory(res model.NullVideoResolution) (int, string) {
	if res.Valid {
		switch res.VideoResolution {
		case model.VideoResolutionV2160p, model.VideoResolutionV4320p:
			return torznab.CategoryTVUHD.ID, "tv_show_uhd"
		case model.VideoResolutionV720p, model.VideoResolutionV1080p, model.VideoResolutionV1440p:
			return torznab.CategoryTVHD.ID, "tv_show_hd"
		case model.VideoResolutionV360p, model.VideoResolutionV480p, model.VideoResolutionV540p, model.VideoResolutionV576p:
			return torznab.CategoryTVSD.ID, "tv_show_sd"
		}
	}
	return torznab.CategoryTV.ID, "tv_show"
}

func movieCategory(res model.NullVideoResolution) (int, string) {
	if res.Valid {
		switch res.VideoResolution {
		case model.VideoResolutionV2160p, model.VideoResolutionV4320p:
			return torznab.CategoryMoviesUHD.ID, "movie_uhd"
		case model.VideoResolutionV720p, model.VideoResolutionV1080p, model.VideoResolutionV1440p:
			return torznab.CategoryMoviesHD.ID, "movie_hd"
		case model.VideoResolutionV360p, model.VideoResolutionV480p, model.VideoResolutionV540p, model.VideoResolutionV576p:
			return torznab.CategoryMoviesSD.ID, "movie_sd"
		}
	}
	return torznab.CategoryMovies.ID, "movie"
}

