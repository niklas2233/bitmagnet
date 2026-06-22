package hashfetcher

import "time"

type Config struct {
	Interval     time.Duration
	BatchSize    int
	Concurrency  int
	GetPeersHops int
}

func NewDefaultConfig() Config {
	return Config{
		Interval:     5 * time.Minute,
		BatchSize:    50,
		Concurrency:  20,
		GetPeersHops: 8,
	}
}
