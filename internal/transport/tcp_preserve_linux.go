//go:build linux

package transport

import (
	"io"
	"net"
	"sync"
	"syscall"
)

type preserveTCPConn struct {
	*net.TCPConn
	raw syscall.RawConn
	mu  sync.RWMutex
	tos int
	ok  bool
}

// WrapPreserveTCP wraps TCP connections so Read captures inbound IP_TOS /
// IPV6_TCLASS control messages for QoS preserve mode.
func WrapPreserveTCP(conn net.Conn) net.Conn {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return conn
	}
	_ = EnableReceiveTOS(tcpConn)
	raw, err := tcpConn.SyscallConn()
	if err != nil {
		return conn
	}
	return &preserveTCPConn{TCPConn: tcpConn, raw: raw}
}

func (c *preserveTCPConn) LastReceivedTOS() (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tos, c.ok
}

func (c *preserveTCPConn) Read(b []byte) (int, error) {
	if c.raw == nil {
		return c.TCPConn.Read(b)
	}
	oob := make([]byte, 512)
	var (
		n    int
		oobn int
		err  error
	)
	readErr := c.raw.Read(func(fd uintptr) bool {
		n, oobn, _, _, err = syscall.Recvmsg(int(fd), b, oob, 0)
		return err != syscall.EAGAIN && err != syscall.EWOULDBLOCK
	})
	if readErr != nil {
		return n, readErr
	}
	if err != nil {
		return n, err
	}
	if n == 0 && oobn == 0 {
		return 0, io.EOF
	}
	if oobn > 0 {
		c.captureTOS(oob[:oobn])
	}
	return n, nil
}

func (c *preserveTCPConn) captureTOS(oob []byte) {
	msgs, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return
	}
	for _, msg := range msgs {
		tos, ok := parseTOSControl(msg)
		if !ok {
			continue
		}
		c.mu.Lock()
		c.tos = tos
		c.ok = true
		c.mu.Unlock()
		return
	}
}
