package mmsg

import (
	"net"
	"net/netip"
	"testing"

	"github.com/go-quicktest/qt"
)

func recvOne(t *testing.T, mc *Conn, batch int) *Message {
	t.Helper()
	ms := make([]Message, batch)
	for i := range ms {
		ms[i].Buffers = [][]byte{make([]byte, 0x1000)}
	}
	n, err := mc.RecvMsgs(ms)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.Equals(n, 1))
	return &ms[0]
}

// Addr names the same sender as AddrPort, on both the batch and the single message path.
func TestAddrMatchesAddrPort(t *testing.T) {
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
		want := s.LocalAddr().(*net.UDPAddr).AddrPort()
		qt.Check(t, qt.Equals(m.AddrPort.Port(), want.Port()))
		qt.Check(t, qt.Equals(m.AddrPort.Addr().Unmap(), want.Addr().Unmap()))
		addr := m.Addr()
		qt.Assert(t, qt.IsNotNil(addr))
		qt.Check(t, qt.Equals(
			addr.(*net.UDPAddr).AddrPort().Addr().Unmap(),
			m.AddrPort.Addr().Unmap(),
		))
		// Asking twice hands back what was already built.
		qt.Check(t, qt.Equals(m.Addr(), addr))
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

// A PacketReader that isn't a UDP socket keeps whatever address it reports.
func TestOtherPacketReaderKeepsItsAddr(t *testing.T) {
	mc := NewConn(&fakePacketReader{addr: fakeAddr("somewhere")})
	m := Message{Buffers: [][]byte{make([]byte, 16)}}
	qt.Assert(t, qt.IsNil(mc.RecvMsg(&m)))
	qt.Check(t, qt.Equals(m.Addr(), net.Addr(fakeAddr("somewhere"))))
	qt.Check(t, qt.IsFalse(m.AddrPort.IsValid()))
}

type fakeAddr string

func (me fakeAddr) Network() string { return "fake" }
func (me fakeAddr) String() string  { return string(me) }

type fakePacketReader struct {
	addr net.Addr
}

func (me *fakePacketReader) ReadFrom(b []byte) (int, net.Addr, error) {
	return copy(b, "hi"), me.addr, nil
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
