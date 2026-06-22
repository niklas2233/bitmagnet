package dhtcrawler

import (
	"context"

	"github.com/bitmagnet-io/bitmagnet/internal/protocol"
)

// runSeedInfoHashes reads infohashes from the infoHashSeed channel and injects them
// directly into the getPeers pipeline with multiple closest-node attempts, so the
// crawler fetches their metainfo from the DHT network even without normal discovery.
func (c *crawler) runSeedInfoHashes(ctx context.Context) {
	const maxNodes = 8 // try up to this many closest DHT nodes per hash

	for {
		select {
		case <-ctx.Done():
			return
		case hash, ok := <-c.infoHashSeed:
			if !ok {
				return
			}

			nodes := c.kTable.GetClosestNodes(hash)
			if len(nodes) == 0 {
				continue // ktable not ready yet; the metafetcher will retry on next poll
			}

			limit := maxNodes
			if len(nodes) < limit {
				limit = len(nodes)
			}

			for _, node := range nodes[:limit] {
				req := nodeHasPeersForHash{
					infoHash: hash,
					node:     node.Addr(),
				}
				select {
				case <-ctx.Done():
					return
				case c.getPeers.In() <- req:
				}
			}
		}
	}
}

// SeedInfoHash is the public interface for external workers to request metainfo fetches.
type SeedInfoHash interface {
	Seed(hash protocol.ID) bool // returns false if the channel is full
}

type infoHashSeedChan chan protocol.ID

func (ch infoHashSeedChan) Seed(hash protocol.ID) bool {
	select {
	case ch <- hash:
		return true
	default:
		return false
	}
}
