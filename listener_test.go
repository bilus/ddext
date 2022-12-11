package ddext_test

import (
	"io"
	"net"
	"testing"
	"time"

	"github.com/bilus/ddext"
	"github.com/bilus/ddext/internal/atomicext"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

type noopListener struct{}

func (noopListener) Accept() (net.Conn, error) {
	return noopConnection{}, nil
}

func (noopListener) Close() error {
	return nil
}

func (noopListener) Addr() net.Addr {
	return noopAddr{}
}

type noopConnection struct{}

func (noopConnection) Close() error {
	return nil
}

func (noopConnection) LocalAddr() net.Addr {
	return noopAddr{}
}

func (noopConnection) Read(b []byte) (n int, err error) {
	return 0, io.EOF
}

func (noopConnection) Write(b []byte) (n int, err error) {
	return len(b), nil
}

func (noopConnection) RemoteAddr() net.Addr {
	return noopAddr{}
}

func (noopConnection) SetDeadline(t time.Time) error {
	return nil
}

func (noopConnection) SetReadDeadline(t time.Time) error {
	return nil
}

func (noopConnection) SetWriteDeadline(t time.Time) error {
	return nil
}

type noopAddr struct{}

func (noopAddr) Network() string {
	return "noopAddr"
}

func (noopAddr) String() string {
	return "noopAddr"
}

type mockClient struct {
	countSum *atomic.Int64
	gaugeMax *atomic.Float64
	t        *testing.T
}

func newMockClient(t *testing.T) mockClient {
	return mockClient{
		countSum: atomic.NewInt64(0),
		gaugeMax: atomic.NewFloat64(0),
		t:        t,
	}
}

func (c mockClient) Count(name string, value int64, tags []string, rate float64) error {
	if name != "http.accept" {
		c.t.Fatalf("unexpected metric name: %q", name)
	}
	c.countSum.Store(value)
	return nil
}

func (c mockClient) Gauge(name string, value float64, tags []string, rate float64) error {
	if name != "http.open_connections" {
		c.t.Fatalf("unexpected metric name: %q", name)
	}

	err := atomicext.Update[float64](c.gaugeMax, 100, func(old float64) float64 {
		if value > old {
			return value
		}
		return old
	})
	if err != nil {
		c.t.Fatalf("Error in mockClient.Gauge: %v", err)
	}
	return nil
}

func TestListener(t *testing.T) {
	require := require.New(t)

	c := newMockClient(t)
	listener, _ := ddext.NewListener(noopListener{}, c,
		// Only listener.Close flushes so countSum can accummulate.
		ddext.ListenerOptions{FlushInterval: 10 * time.Millisecond})

	conns := make([]net.Conn, 10)
	for j := 0; j < 10; j++ {
		conn, _ := listener.Accept()
		conns[j] = conn
	}

	time.Sleep(60 * time.Millisecond) // Ensure flush
	require.Equal(int64(10), c.countSum.Load())
	require.Equal(10.0, c.gaugeMax.Load())

	for j := 0; j < 2; j++ {
		conns[j].Close()
	}

	listener.Close()

	require.Equal(int64(10), c.countSum.Load())
	require.Equal(10.0, c.gaugeMax.Load())
}

type noopClient struct{}

func (noopClient) Count(name string, value int64, tags []string, rate float64) error {
	return nil
}

func (noopClient) Gauge(name string, value float64, tags []string, rate float64) error {
	return nil
}

func BenchmarkListener_Serial(b *testing.B) {
	listener, _ := ddext.NewListener(noopListener{}, noopClient{})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		listener.Accept()
	}
}

func BenchmarkListener_Parallel(b *testing.B) {
	listener, _ := ddext.NewListener(noopListener{}, noopClient{})

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			listener.Accept()
		}
	})
}
