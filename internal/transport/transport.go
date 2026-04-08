package transport

import (
	"context"
	"net"
)

// Transport abstracts the network transport layer for Diameter peers.
// All transports present a net.Conn-compatible interface so the peer FSM
// is transport-agnostic.
type Transport interface {
	// Dial establishes an outbound connection to addr.
	Dial(ctx context.Context, addr string) (net.Conn, error)
	// Listen creates a listener on addr.
	Listen(addr string) (net.Listener, error)
	// Protocol returns the protocol name: "tcp", "tcp+tls", "sctp", "sctp+tls".
	Protocol() string
}
