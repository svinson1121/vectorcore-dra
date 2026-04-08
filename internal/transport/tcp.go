package transport

import (
	"context"
	"net"
)

// TCP is a plain TCP transport.
type TCP struct{}

// NewTCP creates a new plain TCP transport.
func NewTCP() *TCP {
	return &TCP{}
}

// Dial establishes a TCP connection to addr.
func (t *TCP) Dial(ctx context.Context, addr string) (net.Conn, error) {
	d := &net.Dialer{}
	return d.DialContext(ctx, "tcp", addr)
}

// Listen creates a TCP listener on addr.
func (t *TCP) Listen(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

// Protocol returns "tcp".
func (t *TCP) Protocol() string {
	return "tcp"
}
