//go:build !linux

package transport

import "net"

// WrapPreserveTCP is a no-op on platforms without recvmsg TOS support here.
func WrapPreserveTCP(conn net.Conn) net.Conn {
	return conn
}
