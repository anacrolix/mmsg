package mmsg

import (
	"math/rand"
	"net"
	"testing"

	"github.com/go-quicktest/qt"
)

func udpSocket(t *testing.T) interface {
	net.PacketConn
	net.Conn
} {
	pc, err := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 0,
	})
	qt.Assert(t, qt.IsNil(err))
	return pc
}

func payload(t *testing.T) string {
	n := rand.Intn(512) + 1
	b := make([]byte, n)
	nn, err := rand.Read(b)
	qt.Assert(t, qt.IsNil(err))
	qt.Check(t, qt.Equals(nn, n))
	return string(b)
}

func TestReceiveBatch(t *testing.T) {
	s := udpSocket(t)
	r := udpSocket(t)
	mc := NewConn(r)
	b1 := payload(t)
	b2 := payload(t)
	s.WriteTo([]byte(b1), r.LocalAddr())
	s.WriteTo([]byte(b2), r.LocalAddr())
	var ms []Message
	for range 3 {
		ms = append(ms, Message{
			Buffers: [][]byte{make([]byte, 0x1000)},
		})
	}
	n, err := mc.RecvMsgs(ms)
	qt.Check(t, qt.IsNil(err))
	t.Log(n)
	if mc.Err() == nil {
		qt.Check(t, qt.Equals(n, 2))
		qt.Check(t, qt.Equals(string(ms[0].Payload()), b1))
		qt.Check(t, qt.Equals(string(ms[1].Payload()), b2))
	} else {
		t.Logf("error using multi: %s", mc.Err())
		qt.Check(t, qt.Equals(n, 1))
		qt.Check(t, qt.Equals(string(ms[0].Payload()), b1))
	}
}
