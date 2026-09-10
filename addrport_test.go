package mmsg

import (
	"net"
	"net/netip"
	"testing"

	"github.com/go-quicktest/qt"
)

func recvOne(t *testing.T, mc *Conn, batch int) Message {
	t.Helper()
	ms := make([]Message, batch)
	for i := range ms {
		ms[i].Buffers = [][]byte{make([]byte, 0x1000)}
	}
	n, err := mc.RecvMsgs(ms)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(n, 1))
	return ms[0]
}

// AddrPort names the same sender as Addr, on both the batch and the single message path.
func TestAddrPortMatchesAddr(t *testing.T) {
	for _, batch := range []int{1, 16} {
		s := udpSocket(t)
		r := udpSocket(t)
		defer s.Close()
		defer r.Close()
		mc := NewConn(r)
		_, err := s.WriteTo([]byte("hi"), r.LocalAddr())
		qt.Assert(t, qt.IsNil(err))
		m := recvOne(t, mc, batch)
		qt.Check(t, qt.Equals(string(m.Payload()), "hi"))
		qt.Assert(t, qt.IsNotNil(m.Addr))
		want := s.LocalAddr().(*net.UDPAddr).AddrPort()
		qt.Check(t, qt.Equals(m.AddrPort.Port(), want.Port()))
		qt.Check(t, qt.Equals(m.AddrPort.Addr().Unmap(), want.Addr().Unmap()))
		// Whatever the kernel reported, the two fields have to agree.
		qt.Check(t, qt.Equals(
			m.Addr.(*net.UDPAddr).AddrPort().Addr().Unmap(),
			m.AddrPort.Addr().Unmap(),
		))
	}
}

// SkipNetAddrs leaves Addr nil and still reports the sender.
func TestSkipNetAddrs(t *testing.T) {
	for _, batch := range []int{1, 16} {
		s := udpSocket(t)
		r := udpSocket(t)
		defer s.Close()
		defer r.Close()
		mc := NewConn(r)
		mc.SkipNetAddrs(true)
		_, err := s.WriteTo([]byte("hi"), r.LocalAddr())
		qt.Assert(t, qt.IsNil(err))
		m := recvOne(t, mc, batch)
		qt.Check(t, qt.Equals(string(m.Payload()), "hi"))
		qt.Check(t, qt.IsNil(m.Addr))
		qt.Check(t, qt.IsTrue(m.AddrPort.IsValid()))
		want := s.LocalAddr().(*net.UDPAddr).AddrPort()
		qt.Check(t, qt.Equals(m.AddrPort.Port(), want.Port()))
	}
}

func TestAddrPortIPv6(t *testing.T) {
	listen := func() *net.UDPConn {
		pc, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6loopback, Port: 0})
		if err != nil {
			t.Skipf("no IPv6 here: %v", err)
		}
		t.Cleanup(func() { pc.Close() })
		return pc
	}
	for _, batch := range []int{1, 16} {
		s := listen()
		r := listen()
		mc := NewConn(r)
		_, err := s.WriteTo([]byte("hi"), r.LocalAddr())
		qt.Assert(t, qt.IsNil(err))
		m := recvOne(t, mc, batch)
		qt.Check(t, qt.Equals(string(m.Payload()), "hi"))
		qt.Check(t, qt.IsTrue(m.AddrPort.Addr().Is6()))
		qt.Check(t, qt.Equals(m.AddrPort.Addr().Unmap(), netip.IPv6Loopback()))
		qt.Check(t, qt.Equals(m.AddrPort.Port(), s.LocalAddr().(*net.UDPAddr).AddrPort().Port()))
	}
}

// The reused socket.Message slice must not leak a previous batch's results.
func TestReusedBatchSlice(t *testing.T) {
	s := udpSocket(t)
	r := udpSocket(t)
	defer s.Close()
	defer r.Close()
	mc := NewConn(r)
	for i := range 3 {
		_, err := s.WriteTo([]byte{byte(i)}, r.LocalAddr())
		qt.Assert(t, qt.IsNil(err))
		m := recvOne(t, mc, 16)
		qt.Check(t, qt.DeepEquals(m.Payload(), []byte{byte(i)}))
		qt.Check(t, qt.Equals(m.AddrPort.Port(), s.LocalAddr().(*net.UDPAddr).AddrPort().Port()))
	}
}
