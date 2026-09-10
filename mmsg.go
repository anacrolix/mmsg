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
	apr packetReaderAddrPort
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
		ms[i].setSender(sms[i].AddrPort, nil)
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
		if me.apr != nil {
			n, ap, err := me.apr.ReadFromUDPAddrPort(m.Buffers[0])
			m.N = n
			m.setSender(ap, nil)
			return err
		}
		n, addr, err := me.pr.ReadFrom(m.Buffers[0])
		m.N = n
		// A PacketReader of some other kind can have an address netip can't hold, so keep it.
		m.setSender(socket.NetAddrToAddrPort(addr), addr)
		return err
	}
	sm := socket.Message{
		Buffers: m.Buffers,
	}
	err := me.s.RecvMsg(&sm, flags)
	m.setSender(sm.AddrPort, nil)
	m.N = sm.N
	return err
}

type Message struct {
	Buffers [][]byte
	N       int
	// AddrPort is the sender, and costs nothing to report: a netip.Addr holds its bytes inline.
	// A 4-in-6 address is left mapped, exactly as the kernel reported it. Prefer this to Addr.
	AddrPort netip.AddrPort

	// Kept only when the sender isn't an address and port, which a PacketReader that isn't a
	// UDP socket can produce.
	otherAddr net.Addr
	// What Addr has already built, if anything.
	netAddr net.Addr
}

func (me *Message) setSender(ap netip.AddrPort, other net.Addr) {
	me.AddrPort = ap
	me.otherAddr = other
	me.netAddr = other
}

// Addr is the sender as a net.Addr, built from AddrPort the first time it's asked for and kept
// for later calls. Building it allocates the address and the net.IP inside it, which is why
// receiving no longer does: use AddrPort where it will do.
func (me *Message) Addr() net.Addr {
	if me.netAddr == nil && me.AddrPort.IsValid() {
		me.netAddr = net.UDPAddrFromAddrPort(me.AddrPort)
	}
	return me.netAddr
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
