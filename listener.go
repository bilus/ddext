// Package ddext extends the github.com/DataDog/datadog-go library with useful
// utilities.
package ddext

import (
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/bilus/ddext/internal/atomicext"
	"go.uber.org/atomic"
)

// listener emits metrics related to accepted connections.
type listener struct {
	net.Listener

	gaugeMetricName string // the GAUGE metric name
	countMetricName string // the COUNTER metric name

	client client

	accepted *atomic.Uint32 // accepted connection count
	timedout *atomic.Uint32 // timedout connection count
	errored  *atomic.Uint32 // errored connection count

	// open tracks the current number of open connections
	// and is used to calculate maxPeriodOpen (see below).
	open *atomic.Uint32 // current open connection count

	// maxPeriodOpen tracks the maximum number of open connections between
	// subsequent flushes because using `open` directly would only sample the
	// value once every flush interval which is 10 second by default,
	// effectively ignoring lots of peak values.
	maxPeriodOpen *atomic.Uint32 // max open connections since the last flush
	exit          chan struct{}  // exit signal channel (on Close call)
}

const (
	gaugeMetricFmt       = "%s.open_connections"
	countMetricFmt       = "%s.accept"
	defaultPrefix        = "http"
	defaultFlushInterval = 10 * time.Second
)

// ListenerOptions includes additional listener configuration options.
type ListenerOptions struct {
	// FlushInterval determines how often metrics are sent to Datadog.
	FlushInterval time.Duration // default: 10s
	// MetricPrefix is added to every metric (AFTER conrad.).
	MetricPrefix string // default: http
}

// ErrOptsArgumentError indicates more than 1 opts argument was passed.
var ErrTooManyOpts = errors.New("expected only one ListenerOptions argument")

// NewListener returns a listener emitting Datadog GAUGE metric indicating the
// number of open connections.
func NewListener(ln net.Listener, client client, opts ...ListenerOptions) (net.Listener, error) {
	var opt ListenerOptions
	if len(opts) > 1 {
		return nil, ErrTooManyOpts
	}
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.FlushInterval == 0 {
		opt.FlushInterval = defaultFlushInterval
	}
	if opt.MetricPrefix == "" {
		opt.MetricPrefix = defaultPrefix
	}
	ccl := &listener{
		Listener:        ln,
		client:          client,
		gaugeMetricName: fmt.Sprintf(gaugeMetricFmt, opt.MetricPrefix),
		countMetricName: fmt.Sprintf(countMetricFmt, opt.MetricPrefix),

		accepted:      atomic.NewUint32(0),
		timedout:      atomic.NewUint32(0),
		errored:       atomic.NewUint32(0),
		open:          atomic.NewUint32(0),
		maxPeriodOpen: atomic.NewUint32(0),
		exit:          make(chan struct{}),
	}
	go ccl.run(opt.FlushInterval)
	return ccl, nil
}

func (ln *listener) run(flushInterval time.Duration) {
	tick := time.NewTicker(flushInterval)
	defer tick.Stop()
	defer close(ln.exit)
	for {
		select {
		case <-tick.C:
			ln.flushMetrics()
		case <-ln.exit:
			return
		}
	}
}

func (ln *listener) flushMetrics() {
	v := ln.maxPeriodOpen.Swap(0)
	ln.client.Gauge(ln.gaugeMetricName, float64(v), nil, 1)

	for tag, stat := range map[string]*atomic.Uint32{
		"status:success": ln.accepted,
		"status:timeout": ln.timedout,
		"status:error":   ln.errored,
	} {
		if v := int64(stat.Swap(0)); v > 0 {
			ln.client.Count(ln.countMetricName, v, []string{tag}, 1)
		}
	}
}

// Accept implements net.Listener and keeps count of open connections.
func (ln *listener) Accept() (net.Conn, error) {
	conn, err := ln.Listener.Accept()
	if err != nil {
		if ne, ok := err.(net.Error); ok && ne.Timeout() && !ne.Temporary() {
			ln.timedout.Inc()
		} else {
			ln.errored.Inc()
		}
		return conn, err
	}
	new := ln.accepted.Inc()
	atomicext.Update[uint32](ln.maxPeriodOpen, 100, func(old uint32) uint32 {
		if new > old {
			return new
		}
		return old
	})
	return decOnCloseConn{conn, ln.open}, nil
}

func (ln *listener) updatePeriodOpen(value uint32) {
	// Store the max value, retrying if another thread modified it.
	limit := 100 // Try max times.
	for {
		old := ln.maxPeriodOpen.Load()
		if old < value {
			if ln.maxPeriodOpen.CompareAndSwap(old, value) {
				return
			}
			limit--
			if limit == 0 {
				// TODO(bilus): Should we log the error?
				return
			}
		}
	}
}

// Close implements net.Listener and flushes metrics.
func (ln *listener) Close() error {
	err := ln.Listener.Close()
	ln.flushMetrics()

	ln.exit <- struct{}{}
	<-ln.exit
	return err
}

// decOnCloseConn decreases active connection count when the connection closes.
type decOnCloseConn struct {
	net.Conn

	open *atomic.Uint32 // open connection count
}

// Close implements net.Conn, decreasing the number of open connections.
func (conn decOnCloseConn) Close() error {
	conn.open.Dec()
	return conn.Conn.Close()
}
