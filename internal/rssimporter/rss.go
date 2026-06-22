package rssimporter

import (
	"encoding/xml"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bitmagnet-io/bitmagnet/internal/model"
)

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type torznabAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Length uint   `xml:"length,attr"`
}

type rssItem struct {
	Title        string        `xml:"title"`
	Link         string        `xml:"link"`
	PubDate      string        `xml:"pubDate"`
	TorznabAttrs []torznabAttr `xml:"http://torznab.com/schemas/2015/feed attr"`
	Enclosure    rssEnclosure  `xml:"enclosure"`
}

func (item rssItem) torznabAttr(name string) string {
	for _, a := range item.TorznabAttrs {
		if a.Name == name {
			return a.Value
		}
	}
	return ""
}

func parseRSSFeed(data []byte) ([]rssItem, error) {
	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, err
	}
	return feed.Channel.Items, nil
}

func extractInfoHash(item rssItem) (string, bool) {
	// Torznab feeds expose infohash directly as an attribute
	if h := item.torznabAttr("infohash"); h != "" {
		return h, true
	}
	// Fall back to magnet link in <link> or <enclosure url>
	for _, s := range []string{item.Link, item.Enclosure.URL} {
		if hash, ok := hashFromMagnet(s); ok {
			return hash, true
		}
	}
	return "", false
}

func extractSize(item rssItem) uint {
	if s := item.torznabAttr("size"); s != "" {
		if v, err := strconv.ParseUint(s, 10, 64); err == nil {
			return uint(v)
		}
	}

	if item.Enclosure.Length > 0 {
		return item.Enclosure.Length
	}
	return 0
}

func extractSeeders(item rssItem) model.NullUint {
	if s := item.torznabAttr("seeders"); s != "" {
		if v, err := strconv.ParseUint(s, 10, 64); err == nil {
			return model.NewNullUint(uint(v))
		}
	}
	return model.NullUint{}
}

func extractLeechers(item rssItem) model.NullUint {
	// Torznab uses "peers" to mean total peers (seeders + leechers).
	// Leechers = peers - seeders.
	if p := item.torznabAttr("peers"); p != "" {
		if peers, err := strconv.ParseUint(p, 10, 64); err == nil {
			seeders := uint64(0)

			if s := item.torznabAttr("seeders"); s != "" {
				if sv, err2 := strconv.ParseUint(s, 10, 64); err2 == nil {
					seeders = sv
				}
			}

			leechers := uint64(0)

			if peers > seeders {
				leechers = peers - seeders
			}
			return model.NewNullUint(uint(leechers))
		}
	}
	return model.NullUint{}
}

func hashFromMagnet(s string) (string, bool) {
	if !strings.HasPrefix(s, "magnet:") {
		return "", false
	}

	u, err := url.Parse(s)
	if err != nil {
		return "", false
	}

	for _, xt := range u.Query()["xt"] {
		if strings.HasPrefix(xt, "urn:btih:") {
			return strings.TrimPrefix(xt, "urn:btih:"), true
		}
	}
	return "", false
}

func parsePubDate(s string) time.Time {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		"Mon, 02 Jan 2006 15:04:05 -0700",
		"Mon, 02 Jan 2006 15:04:05 MST",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
