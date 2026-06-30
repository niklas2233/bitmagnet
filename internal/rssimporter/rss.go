package rssimporter

import (
	"encoding/xml"
	"fmt"
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
	Items []RSSItem `xml:"item"`
}

// TorznabAttr is a torznab:attr element from a Torznab RSS feed.
type TorznabAttr struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value,attr"`
}

// RSSEnclosure is the <enclosure> element of an RSS item.
type RSSEnclosure struct {
	URL    string `xml:"url,attr"`
	Length uint   `xml:"length,attr"`
}

// RSSItem is a single <item> from a Torznab/RSS feed.
type RSSItem struct {
	Title        string        `xml:"title"`
	Link         string        `xml:"link"`
	PubDate      string        `xml:"pubDate"`
	TorznabAttrs []TorznabAttr `xml:"http://torznab.com/schemas/2015/feed attr"`
	Enclosure    RSSEnclosure  `xml:"enclosure"`
}

func (item RSSItem) torznabAttr(name string) string {
	for _, a := range item.TorznabAttrs {
		if a.Name == name {
			return a.Value
		}
	}

	return ""
}

// torznabErrorResp matches Prowlarr/Torznab error responses like:
// <?xml version="1.0"?><error code="429" description="Indexer is disabled..."/>
type torznabErrorResp struct {
	XMLName     xml.Name `xml:"error"`
	Code        string   `xml:"code,attr"`
	Description string   `xml:"description,attr"`
}

// ParseFeed parses raw RSS/Torznab XML bytes into a slice of RSSItems.
func ParseFeed(data []byte) ([]RSSItem, error) {
	// Detect Prowlarr/Torznab error envelope before attempting RSS parse.
	var errResp torznabErrorResp
	if xml.Unmarshal(data, &errResp) == nil && errResp.Code != "" {
		return nil, fmt.Errorf("feed error (code %s): %s", errResp.Code, errResp.Description)
	}

	var feed rssFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, err
	}

	return feed.Channel.Items, nil
}

// ExtractInfoHash returns the info hash from a Torznab attribute or magnet link.
func ExtractInfoHash(item RSSItem) (string, bool) {
	// Torznab feeds expose infohash directly as an attribute
	if h := item.torznabAttr("infohash"); h != "" {
		return h, true
	}
	// Fall back to magnet link in <link> or <enclosure url>
	for _, s := range []string{item.Link, item.Enclosure.URL} {
		if hash, ok := HashFromMagnet(s); ok {
			return hash, true
		}
	}

	return "", false
}

// ExtractSize returns the torrent size in bytes from torznab attributes or enclosure length.
func ExtractSize(item RSSItem) uint {
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

// ExtractSeeders returns the seeder count from torznab attributes.
func ExtractSeeders(item RSSItem) model.NullUint {
	if s := item.torznabAttr("seeders"); s != "" {
		if v, err := strconv.ParseUint(s, 10, 64); err == nil {
			return model.NewNullUint(uint(v))
		}
	}

	return model.NullUint{}
}

// ExtractLeechers returns the leecher count derived from torznab peers - seeders.
func ExtractLeechers(item RSSItem) model.NullUint {
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

// HashFromMagnet extracts the btih info hash from a magnet URI.
func HashFromMagnet(s string) (string, bool) {
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

// ParsePubDate parses an RSS pubDate string into a time.Time, trying common formats.
func ParsePubDate(s string) time.Time {
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
