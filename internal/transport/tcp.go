package transport

import (
	"context"
	"net"
	"syscall"
)

// TCP is a plain TCP transport.
type TCP struct {
	qos QoSPolicy
}

// NewTCP creates a new plain TCP transport.
func NewTCP(qos ...QoSPolicy) *TCP {
	policy := DefaultQoS()
	if len(qos) > 0 {
		policy = qos[0]
	}
	return &TCP{qos: policy}
}

// Dial establishes a TCP connection to addr.
func (t *TCP) Dial(ctx context.Context, addr string) (net.Conn, error) {
	d := &net.Dialer{Control: t.control}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if err := t.qos.Apply(conn); err != nil {
		conn.Close()
		return nil, err
	}
	if t.qos.Preserve() {
		conn = WrapPreserveTCP(conn)
	}
	return conn, nil
}

// Listen creates a TCP listener on addr.
func (t *TCP) Listen(addr string) (net.Listener, error) {
	lc := net.ListenConfig{Control: t.control}
	return lc.Listen(context.Background(), "tcp", addr)
}

func (t *TCP) control(network, address string, c syscall.RawConn) error {
	if err := t.qos.Control(network, address, c); err != nil {
		return err
	}
	if t.qos.Preserve() {
		return EnableReceiveTOSControl(c)
	}
	return nil
}

// Protocol returns "tcp".
func (t *TCP) Protocol() string {
	return "tcp"
}
