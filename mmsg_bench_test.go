package mmsg

import (
	"net"
	"testing"
)

func udpSocketB(b *testing.B) *net.UDPConn {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { pc.Close() })
	return pc
}

// One datagram in flight at a time, so every call receives exactly one message and the reported
// allocations are what one message costs. The loop avoids anything that allocates itself, since
// what's being measured is a handful of allocations per message.
func benchmarkRecv(b *testing.B, skipNetAddr bool, batch int) {
	s := udpSocketB(b)
	r := udpSocketB(b)
	mc := NewConn(r)
	mc.SkipNetAddrs(skipNetAddr)
	ms := make([]Message, batch)
	for i := range ms {
		ms[i].Buffers = [][]byte{make([]byte, 0x10000)}
	}
	payload := make([]byte, 512)
	raddr := r.LocalAddr().(*net.UDPAddr).AddrPort()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := s.WriteToUDPAddrPort(payload, raddr); err != nil {
			b.Fatal(err)
		}
		n, err := mc.RecvMsgs(ms)
		if err != nil {
			b.Fatal(err)
		}
		if n != 1 {
			b.Fatalf("received %d messages, want 1", n)
		}
	}
}

func BenchmarkRecvMsgsBatch16(b *testing.B)            { benchmarkRecv(b, false, 16) }
func BenchmarkRecvMsgsBatch16SkipNetAddr(b *testing.B) { benchmarkRecv(b, true, 16) }

// len(ms) == 1 takes the single message path rather than recvmmsg.
func BenchmarkRecvMsgsSingle(b *testing.B)            { benchmarkRecv(b, false, 1) }
func BenchmarkRecvMsgsSingleSkipNetAddr(b *testing.B) { benchmarkRecv(b, true, 1) }
