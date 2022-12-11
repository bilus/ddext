# ddext

This package extends the
[github.com/DataDog/datadog-go](https://github.com/DataDog/datadog-go) library
with useful utilities:

- Wrapper for `net.Listener`, adding metrics around active connections. Most
  typical use - HTTP/TLS servers.


## Installation

Get the code with:

``` gogo
go get -u github.com/bilus/ddext
```

It's not "batteries included" in the sense that it lets you pick the underlying
statsd implementation. Statsd library is not included, you need to add it to
`go.mod`.

The library has been tested with the `v5` major version. Visit the
[homepage](https://github.com/DataDog/datadog-go) for more details. TL;DR:

``` go
go get -u github.com/DataDog/datadog-go/v5/statsd
```

## Listener

Use it to emit useful metrics about accepted connections by a TCP
server:

- `http.open_connections` is a GAUGE metric containing the number of active
  connections
- `http.accept` is a COUNTER metric tracking accepted connections; it has a
  `status` tag with the following values:
  - `success` - connection accepted,
  - `timeout` - connection not accepted due to a time-out,
  - `error`- another error.

> Because you will typically use it with an HTTP server, the names of the
> metrics sent by the listener start with `"http"` by default. You may change
> the prefix via `Options` passed to `NewListener`. Read on.

### Quick-start

To use the listener, simply wrap the default one. Here is an example (error
handling elided):

``` go
import (
    "fmt"
    "net"

    "github.com/DataDog/datadog-go/v5/statsd"
    "github.com/bilus/ddext"
)

// [...]

statsd, __ := statsd.New("127.0.0.1:8125")
listener, _ := net.Listen("tcp", fmt.Sprintf(":%d", port))
listener, _ = ddext.NewListener(listener, statsd)
_ = _httpServer.Serve(listener)
```


### Customization

By default, metrics are sent once every 10 seconds but you can change the
interval as well as the prefix for metric names:

``` go

opts := ddext.ListenerOptions {
  FlushInterval: 20 * time.Second,
  MetricPrefix: "acme.app.http",
}
listener, __ = ddext.NewListener(listener, statsd, opts)
```

In the above example, the listener sends `acme.app.http.open_connections` and
`acme.app.http.accept` metrics once every 20 seconds.

### TLS server example

Below is an example adapted from
[github.com/mwitkow/go-conntrack](https://github.com/mwitkow/go-conntrack) using
that library to make it easier to wrap a TLS HTTP server listener:

``` go
import "github.com/mwitkow/go-conntrack/connhelpers"

// [...]

listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
listener = ddext.NewListener(listener, statsd)
tlsConfig, _ := connhelpers.TlsConfigForServerCerts(certFilePath, keyFilePath)
tlsConfig, _ = connhelpers.TlsConfigWithHttp2Enabled(tlsConfig)
tlsListener := tls.NewListener(listener, tlsConfig)
httpServer.Serve(listener)
```

> This library does not come with `go-conntrack` so you need to add it to your
> `go.mod` to use it. Visit its
> [homepage](https://github.com/mwitkow/go-conntrack) for more information.

## Does it work only for Datadog?

The library uses a rather straightforward interface defined in
[client.go](client.go) so it should be fairly easy to adapt it to other
observability services.

## Related work

1. This library is largely based on
[github.com/DataDog/datadog-agent/trace][https://github.com/DataDog/datadog-agent/tree/main/pkg/trace],
extending it with more functionality and ways to customize it.

2. For Prometheus I recommend the aforementioned
   [github.com/mwitkow/go-conntrack](https://github.com/mwitkow/go-conntrack).
   It also supports tracing.

## Status

The code is used in production and is a relatively thin wrapper over the
battle-tested Statsd client. Nevertheless, you are advised to measure its
performance impact before exposing to large-scale production load.

## Benchmark

```
go test -test.v -test.run=NONE -test.bench=".*"
goos: darwin
goarch: amd64
pkg: github.com/bilus/ddext
cpu: Intel(R) Core(TM) i7-1068NG7 CPU @ 2.30GHz
BenchmarkListener_Serial
BenchmarkListener_Serial-8     	27500558	        41.01 ns/op
BenchmarkListener_Parallel
BenchmarkListener_Parallel-8   	27395151	        53.08 ns/op
```
