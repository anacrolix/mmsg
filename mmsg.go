package mmsg

import (
	"errors"
	"net"
	"net/netip"
	"strings"

	"github.com/anacrolix/mmsg/socket"
)

// Considered MSG_DONTWAIT, but I think Go puts the socket into non-blocking
// mode in its runtime and it seems to do the right thing.
const flags = 0

// Conn receives datagrams, in batches where the platform can. Its receive methods reuse state
// held here, so they aren't safe to call concurrently with each other on one Conn.
type Conn struct {
	// If this is not nil, attempts to use batch APIs will be skipped automatically.
	err error
	s   *socket.Conn
	pr  PacketReader
	// Set when pr has it, to read the sender without allocating a net.Addr.
	apr         packetReaderAddrPort
	skipNetAddr bool
	// Reused between calls, so a batch receive doesn't allocate one per call.
	sms []socket.Message
}

type PacketReader interface {
	ReadFrom([]byte) (int, net.Addr, error)
}

// Implemented by *net.UDPConn. ReadFrom has to box the sender into a net.Addr, allocating that
// and the net.IP inside it for every datagram; this reports it as a value instead.
type packetReaderAddrPort interface {
	ReadFromUDPAddrPort([]byte) (int, netip.AddrPort, error)
}

// pr must implement net.Conn for mmsg to be enabled.
func NewConn(pr PacketReader) *Conn {
	ret := Conn{
		pr: pr,
	}
	ret.apr, _ = pr.(packetReaderAddrPort)
	nc, ok := pr.(net.Conn)
	if ok {
		ret.s, ret.err = socket.NewConn(nc)
	} else {
		ret.err = errors.New("mmsg.NewConn: not a net.Conn")
	}
	return &ret
}

// SkipNetAddrs stops receives filling Message.Addr, leaving the sender in Message.AddrPort alone.
// Building the net.Addr is the last thing a receive allocates, so a caller that can work with a
// netip.AddrPort should set this. Set it before the first receive.
func (me *Conn) SkipNetAddrs(skip bool) {
	me.skipNetAddr = skip
	if me.s != nil {
		me.s.SkipNetAddr = skip
	}
}

func (me *Conn) recvMsgAsMsgs(ms []Message) (int, error) {
	err := me.RecvMsg(&ms[0])
	if err != nil {
		return 0, err
	}
	return 1, err
}

func (me *Conn) RecvMsgs(ms []Message) (n int, err error) {
	if me.err != nil || len(ms) == 1 {
		return me.recvMsgAsMsgs(ms)
	}
	sms := me.socketMsgs(ms)
	n, err = me.s.RecvMsgs(sms, flags)
	if err != nil && strings.Contains(err.Error(), "not implemented") {
		if me.err != nil {
			panic(me.err)
		}
		me.err = err
		if n <= 0 {
			return me.recvMsgAsMsgs(ms)
		}
		err = nil
	}
	for i := 0; i < n; i++ {
		ms[i].Addr = sms[i].Addr
		ms[i].AddrPort = sms[i].AddrPort
		ms[i].N = sms[i].N
	}
	return n, err
}

// The socket.Messages to receive into, grown as needed and reused between calls.
func (me *Conn) socketMsgs(ms []Message) []socket.Message {
	if cap(me.sms) < len(ms) {
		me.sms = make([]socket.Message, len(ms))
	}
	me.sms = me.sms[:len(ms)]
	for i := range ms {
		me.sms[i].Buffers = ms[i].Buffers
	}
	return me.sms
}

func (me *Conn) RecvMsg(m *Message) error {
	if len(m.Buffers) == 1 { // What about 0?
		var err error
		if me.apr != nil {
			m.N, m.AddrPort, err = me.apr.ReadFromUDPAddrPort(m.Buffers[0])
			m.Addr = nil
			if err == nil && !me.skipNetAddr {
				m.Addr = net.UDPAddrFromAddrPort(m.AddrPort)
			}
			return err
		}
		m.N, m.Addr, err = me.pr.ReadFrom(m.Buffers[0])
		m.AddrPort = socket.NetAddrToAddrPort(m.Addr)
		if me.skipNetAddr {
			m.Addr = nil
		}
		return err
	}
	sm := socket.Message{
		Buffers: m.Buffers,
	}
	err := me.s.RecvMsg(&sm, flags)
	m.Addr = sm.Addr
	m.AddrPort = sm.AddrPort
	m.N = sm.N
	return err
}

type Message struct {
	Buffers [][]byte
	N       int
	// The sender. Nil if the Conn was told to skip it with [Conn.SkipNetAddrs].
	Addr net.Addr
	// The sender, for an IP socket. Always filled in, and unlike Addr it costs no allocation. A
	// 4-in-6 address stays mapped, exactly as the kernel reported it.
	AddrPort netip.AddrPort
}

func (me *Message) Payload() (p []byte) {
	n := me.N
	for _, b := range me.Buffers {
		if len(b) >= n {
			p = append(p, b[:n]...)
			return
		}
		p = append(p, b...)
		n -= len(b)
	}
	panic(n)
}

// Returns not nil if message batching is not working.
func (me *Conn) Err() error {
	return me.err
}
