package transport

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/ishidawataru/sctp"
)

// SCTP is a plain SCTP transport using the ishidawataru/sctp library.
type SCTP struct{}

// NewSCTP creates a new plain SCTP transport.
func NewSCTP() *SCTP {
	return &SCTP{}
}

// Dial establishes an SCTP connection to addr ("host:port").
func (t *SCTP) Dial(ctx context.Context, addr string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("sctp: invalid addr %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("sctp: invalid port %q: %w", portStr, err)
	}

	ips, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("sctp: resolving %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("sctp: no addresses for %q", host)
	}

	addrs := make([]net.IPAddr, 0, len(ips))
	for _, ip := range ips {
		parsed := net.ParseIP(ip)
		if parsed != nil {
			addrs = append(addrs, net.IPAddr{IP: parsed})
		}
	}

	sctpAddr := &sctp.SCTPAddr{IPAddrs: addrs, Port: port}

	// Dial with context deadline if set
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := sctp.DialSCTP("sctp", nil, sctpAddr)
		ch <- result{conn, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		return r.conn, r.err
	}
}

// Listen creates an SCTP listener on addr ("host:port").
func (t *SCTP) Listen(addr string) (net.Listener, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("sctp: invalid listen addr %q: %w", addr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("sctp: invalid port %q: %w", portStr, err)
	}

	var addrs []net.IPAddr
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		// Wildcard - pass empty IPAddrs list; library binds to all interfaces (IPv4+IPv6)
		addrs = nil
	} else {
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("sctp: listen address must be an IP, got %q", host)
		}
		addrs = []net.IPAddr{{IP: ip}}
	}

	sctpAddr := &sctp.SCTPAddr{IPAddrs: addrs, Port: port}
	ln, err := sctp.ListenSCTP("sctp", sctpAddr)
	if err != nil {
		return nil, fmt.Errorf("sctp: listen on %s: %w", addr, err)
	}
	return ln, nil
}

// Protocol returns "sctp".
func (t *SCTP) Protocol() string {
	return "sctp"
}
