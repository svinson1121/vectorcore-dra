//go:build linux

package transport

import (
	"io"
	"net"
	"sync"
	"syscall"

	"github.com/ishidawataru/sctp"
	"golang.org/x/sys/unix"
)

type preserveSCTPConn struct {
	*sctp.SCTPConn
	fd  int
	mu  sync.RWMutex
	tos int
	ok  bool
}

// WrapPreserveSCTP wraps accepted SCTP connections so Read captures inbound
// IP_TOS/IPV6_TCLASS control messages for QoS preserve mode.
func WrapPreserveSCTP(conn net.Conn) net.Conn {
	sctpConn, ok := conn.(*sctp.SCTPConn)
	if !ok {
		return conn
	}
	_ = EnableReceiveTOS(sctpConn)
	return &preserveSCTPConn{SCTPConn: sctpConn, fd: sctpFD(sctpConn)}
}

func (c *preserveSCTPConn) LastReceivedTOS() (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tos, c.ok
}

func (c *preserveSCTPConn) Read(b []byte) (int, error) {
	if c.fd < 0 {
		return c.SCTPConn.Read(b)
	}
	oob := make([]byte, 512)
	n, oobn, _, _, err := syscall.Recvmsg(c.fd, b, oob, 0)
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

func sctpFD(conn *sctp.SCTPConn) int {
	fd := -1
	if raw, err := conn.SyscallConn(); err == nil {
		_ = raw.Control(func(rawfd uintptr) {
			fd = int(rawfd)
		})
	}
	return fd
}

func (c *preserveSCTPConn) captureTOS(oob []byte) {
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

func parseTOSControl(msg syscall.SocketControlMessage) (int, bool) {
	switch {
	case msg.Header.Level == unix.IPPROTO_IP && msg.Header.Type == unix.IP_TOS && len(msg.Data) > 0:
		return int(msg.Data[0]), true
	case msg.Header.Level == unix.IPPROTO_IPV6 && msg.Header.Type == unix.IPV6_TCLASS && len(msg.Data) >= 4:
		return int(nativeEndian.Uint32(msg.Data[:4])), true
	default:
		return 0, false
	}
}
