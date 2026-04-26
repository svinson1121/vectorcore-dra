//go:build !linux

package transport

import "net"

// WrapPreserveSCTP is a no-op on platforms without SCTP recvmsg support here.
func WrapPreserveSCTP(conn net.Conn) net.Conn {
	return conn
}
