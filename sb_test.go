package sockbench

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"testing"
)

func BenchmarkSocket(b *testing.B) {
	benchmarks := map[string]struct {
		network string // network name -- this is passed to net.Listen
		address string // address, this is also passed to net.Listen
		setup   func() // does any setup required by this socket type.  in this case, only used by unix type sockets
		cleanup func() // does any setup required by this socket type.  in this case, only used by unix type sockets
	}{
		"TCP Socket": {
			network: "tcp",
			address: ":8889",
			setup:   func() {},
			cleanup: func() {},
		},
		"Unix Socket": {
			network: "unix",
			address: "/tmp/test-sock",
			setup:   func() { os.Remove("/tmp/test-sock") }, // attempt to remove the socket file before running benchmark in case there was a lingering file
			cleanup: func() { os.Remove("/tmp/test-sock") }, // remove the socket file after the benchmark to prevent a lingering file from causing an error
		},
	}

	// exp is the exponent of the packet size.  we're iterating from 2^1 to 2^30
	// size packets so illustrate how the size of the packet impacts performance.
	// my guess is that as the packet sizes increase, the performance gap between
	// the two methods will shrink beause protocol overhead will be less of a
	// factor.
	for exp := 1; exp < 30; exp++ {
		// size has to be an int64 since we compare it with the result of io.Copy()
		// later.
		size := int64(1) << exp

		// make a buffer of random bytes
		buf := make([]byte, size)
		rand.Read(buf)

		br := bytes.NewReader(buf)
		for name, bench := range benchmarks {
			b.Run(fmt.Sprintf("%d", size), func(b *testing.B) {
				bench.setup()
				defer bench.cleanup()

				// listen on our chosen network type at our specified address
				l, err := net.Listen(bench.network, bench.address)
				if err != nil {
					b.Fatal(err)
				}
				defer l.Close()

				// a simple network server that accepts one connection and copies all
				// bytes read from it to /dev/null essentially.
				//
				// cancelled by the passed in context.
				startListener := func(b *testing.B, ctx context.Context, l net.Listener) {
					b.Helper()

					c, err := l.Accept()
					if err != nil {
						b.Log(err)
						return
					}
					for {
						select {
						case <-ctx.Done():
							return
						default:
							io.Copy(ioutil.Discard, c)
						}
					}
				}

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				go startListener(b, ctx, l)

				s, err := net.Dial(bench.network, bench.address)
				if err != nil {
					b.Fatal(err)
				}
				defer s.Close()

				b.Run(name, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						br.Seek(0, 0)
						nbytes, err := io.Copy(s, br)

						if err != nil {
							b.Fatal(err)
						}
						if nbytes != size {
							b.Fatalf("copied %d of expected %d bytes", nbytes, size)
						}
					}
				})
			})
		}
	}
}
