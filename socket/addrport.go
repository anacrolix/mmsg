// This file is a local addition to the copy of golang.org/x/net/internal/socket, and has no
// upstream counterpart. Keep it separate so that re-syncing the rest stays a straight copy.

package socket

import (
	"encoding/binary"
	"net"
	"net/netip"
	"runtime"
)

// For the paths that only have a net.Addr to work from. A 4-in-6 address stays mapped, so that
// AddrPort and Addr always name the peer the same way the kernel did.
func NetAddrToAddrPort(a net.Addr) netip.AddrPort {
	switch a := a.(type) {
	case *net.UDPAddr:
		return a.AddrPort()
	case *net.TCPAddr:
		return a.AddrPort()
	}
	return netip.AddrPort{}
}

// Reads a raw sockaddr as a netip.AddrPort. This is parseInetAddr without the allocations: a
// netip.Addr keeps its bytes inline, where a net.IP is a slice and a net.UDPAddr is reached
// through a pointer. Reports false for a family that has no address and port.
func parseInetAddrPort(b []byte) (netip.AddrPort, bool) {
	if len(b) < 2 {
		return netip.AddrPort{}, false
	}
	var af int
	switch runtime.GOOS {
	case "android", "illumos", "linux", "solaris", "windows":
		af = int(NativeEndian.Uint16(b[:2]))
	default:
		af = int(b[1])
	}
	switch af {
	case sysAF_INET:
		if len(b) < sizeofSockaddrInet4 {
			return netip.AddrPort{}, false
		}
		return netip.AddrPortFrom(
			netip.AddrFrom4([4]byte(b[4:8])),
			binary.BigEndian.Uint16(b[2:4]),
		), true
	case sysAF_INET6:
		if len(b) < sizeofSockaddrInet6 {
			return netip.AddrPort{}, false
		}
		addr := netip.AddrFrom16([16]byte(b[8:24]))
		// Only a link-local address carries one, and the interning behind WithZone means the
		// allocation it can cost happens once per zone rather than once per packet.
		if id := int(NativeEndian.Uint32(b[24:28])); id > 0 {
			addr = addr.WithZone(zoneCache.name(id))
		}
		return netip.AddrPortFrom(addr, binary.BigEndian.Uint16(b[2:4])), true
	}
	return netip.AddrPort{}, false
}

// Fills in a received message's peer address. Only AddrPort is set: turning a sockaddr into a
// net.Addr allocates, and the caller can do it from AddrPort if it turns out to want one.
func setMessageAddr(m *Message, name []byte) {
	m.AddrPort, _ = parseInetAddrPort(name)
	m.Addr = nil
}
