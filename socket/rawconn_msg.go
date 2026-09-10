// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris || windows || zos

package socket

import (
	"net"
	"os"
)

func (c *Conn) recvMsg(m *Message, flags int) error {
	m.raceWrite()
	var (
		operr     error
		n         int
		oobn      int
		recvflags int
		from      net.Addr
	)
	fn := func(s uintptr) bool {
		n, oobn, recvflags, from, operr = recvmsg(s, m.Buffers, m.OOB, flags, c.network)
		return ioComplete(flags, operr)
	}
	if err := c.c.Read(fn); err != nil {
		return err
	}
	if operr != nil {
		return os.NewSyscallError("recvmsg", operr)
	}
	// Local change: report the peer the same way the recvmmsg path does. This one reaches the
	// address through unix.RecvmsgBuffers, which allocates a sockaddr before we see it, so
	// there's nothing to save here beyond keeping the two paths consistent.
	m.AddrPort = NetAddrToAddrPort(from)
	m.Addr = nil
	m.N = n
	m.NN = oobn
	m.Flags = recvflags
	return nil
}

func (c *Conn) sendMsg(m *Message, flags int) error {
	m.raceRead()
	var (
		operr error
		n     int
	)
	fn := func(s uintptr) bool {
		n, operr = sendmsg(s, m.Buffers, m.OOB, m.Addr, flags)
		return ioComplete(flags, operr)
	}
	if err := c.c.Write(fn); err != nil {
		return err
	}
	if operr != nil {
		return os.NewSyscallError("sendmsg", operr)
	}
	m.N = n
	m.NN = len(m.OOB)
	return nil
}
