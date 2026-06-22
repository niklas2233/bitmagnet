---
title: Import
description: Importing torrents into bitmagnet
parent: Guides
layout: default
nav_order: 6
redirect_from:
  - /tutorials/importing.html
  - /tutorials/import.html
---

# Import

{: .warning-title }

> Important
>
> Before continuing with this tutorial, please [obtain and configure a personal TMDB API key, or disable the TMDB API integration]({% link setup/configuration.md %}#obtaining-a-tmdb-api-key).

**bitmagnet** includes an import endpoint at `/import`; this can be used for importing Torrent files from any source.

{: .note }

> A proper schema is needed for this endpoint, along with improved input validation. There isn't currently a way to import a torrent along with information about the files it contains (which is optional in **bitmagnet**). If an imported torrent is later discovered by the DHT crawler then its associated file info would be saved at that point.

## Adding RSS / Torznab sources

The **Sources** page (Dashboard → Sources) lets you add RSS or Torznab feeds that bitmagnet will poll automatically.

### Fields

| Field | Description |
|---|---|
| **URL** | Full `http://` or `https://` URL of the feed |
| **Source name** | A short identifier for the source — lowercase letters, numbers, hyphens and underscores only (e.g. `my_indexer`). This key is what appears in the **Torrent Source** sidebar filter on the Torrents page. |

### How it works

1. Enter the feed URL and a source name, then click **Add**.
2. The source immediately appears in the Torrent Source sidebar filter, even before any torrents have been imported.
3. bitmagnet polls all configured feeds every 5 minutes. Each new torrent found is queued for processing and classification at high priority.
4. To remove a feed, click the delete (trash) icon next to it in the feeds table. This removes the feed from polling but does not delete any already-imported torrents.

### Torznab feeds (Prowlarr etc.)

Torznab-compatible indexers work the same way — paste the full Torznab API URL (including the `apikey` parameter) as the URL and give it a source name. bitmagnet will poll the feed's recent results on the standard 5-minute cycle.

### API

The same operations are available via REST if you prefer to manage feeds programmatically:

```sh
# List feeds
curl http://localhost:3333/api/rss-feeds

# Add a feed
curl -X POST http://localhost:3333/api/rss-feeds \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com/rss", "source": "my_indexer"}'

# Remove a feed by source name
curl -X DELETE http://localhost:3333/api/rss-feeds/my_indexer
```

## Example: Importing from a JSON source

The `/import` endpoint accepts newline-delimited JSON. Each line should be a JSON object with the following fields:

- `infoHash` (required): the torrent info hash as a hex string
- `name` (required): the torrent name
- `size` (optional): total size in bytes
- `source` (required): a source key string (e.g. `"my_source"`)
- `contentType` (optional): one of `movie`, `tv_show`, `music`, `ebook`, `software`, `xxx`
- `publishedAt` (optional): ISO 8601 date string

You can pipe any JSON data source through [jq](https://jqlang.github.io/jq/) to shape it into this format, then send it to the endpoint:

```sh
cat my-data.json \
  | jq -r --indent 0 '.[] | { infoHash: .hash, name: .title, size: .size, source: "my_source" } | del(..|nulls)' \
  | curl --verbose -H "Content-Type: application/json" -H "Connection: close" --data-binary @- http://localhost:3333/import
```

Once the import starts you should immediately start seeing items appear in the web UI. Each imported item will also be sent to the classification queue to further enrich its metadata.
