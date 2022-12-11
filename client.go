package ddext

// client is compatible with Datadog's statsd client.
type client interface {
	// Count tracks how the number of occurrences per second.
	Count(name string, value int64, tags []string, rate float64) error
	// Gauge tracks a value at a particular points in time.
	Gauge(name string, value float64, tags []string, rate float64) error
}
