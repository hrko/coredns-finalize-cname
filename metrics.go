package finalize

import (
	"sync"

	"github.com/coredns/coredns/plugin"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// requestCount exports a prometheus metric that is incremented every time a query is seen by the example plugin.
var requestCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: pluginName,
	Name:      "request_count_total",
	Help:      "Counter of requests processed.",
}, []string{"server"})

var circularReferenceCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: pluginName,
	Name:      "circular_reference_count_total",
	Help:      "Counter of detected circular references.",
}, []string{"server"})

var danglingCNameCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: pluginName,
	Name:      "dangling_cname_count_total",
	Help:      "Counter of CNAMES that couldn't be resolved.",
}, []string{"server"})

var maxLookupReachedCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: pluginName,
	Name:      "max_lookup_reached_count_total",
	Help:      "Counter of incidents when the maximum lookup depth was reached while trying to resolve a CNAME.",
}, []string{"server"})

var upstreamErrorCount = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: plugin.Namespace,
	Subsystem: pluginName,
	Name:      "upstream_error_count_total",
	Help:      "Counter of upstream errors received.",
}, []string{"server"})

var requestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Namespace: plugin.Namespace,
	Subsystem: pluginName,
	Name:      "request_duration_seconds",
	Buckets:   plugin.TimeBuckets,
	Help:      "Histogram of the time each request took.",
}, []string{"server"})

var _ sync.Once
