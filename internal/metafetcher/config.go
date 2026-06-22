package metafetcher

import "time"

type Config struct {
	Interval  time.Duration
	BatchSize int
}

func NewDefaultConfig() Config {
	return Config{
		Interval:  2 * time.Minute,
		BatchSize: 100,
	}
}
