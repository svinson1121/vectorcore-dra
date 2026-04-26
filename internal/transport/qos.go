package transport

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// QoSPolicy describes how DSCP/TOS should be applied to transport sockets.
type QoSPolicy struct {
	Mode string
	DSCP int
}

// NewQoSPolicy validates and normalizes a QoS policy.
func NewQoSPolicy(mode string, dscp int) (QoSPolicy, error) {
	if mode == "" {
		mode = "clear"
	}
	mode = strings.ToLower(mode)
	switch mode {
	case "preserve", "set", "clear":
	default:
		return QoSPolicy{}, fmt.Errorf("unknown qos mode %q", mode)
	}
	if dscp < 0 || dscp > 63 {
		return QoSPolicy{}, fmt.Errorf("qos dscp must be between 0 and 63, got %d", dscp)
	}
	if mode != "set" {
		dscp = 0
	}
	return QoSPolicy{Mode: mode, DSCP: dscp}, nil
}

// DefaultQoS returns the default best-effort policy.
func DefaultQoS() QoSPolicy {
	return QoSPolicy{Mode: "clear"}
}

// Preserve reports whether this policy copies traffic class from source sockets.
func (q QoSPolicy) Preserve() bool {
	return q.Mode == "preserve"
}

// LastReceivedTOSConn is implemented by connections that can expose inbound
// packet traffic class captured from recvmsg control messages.
type LastReceivedTOSConn interface {
	LastReceivedTOS() (int, bool)
}

func (q QoSPolicy) configuredTOS() (int, bool) {
	switch q.Mode {
	case "set":
		return q.DSCP << 2, true
	case "clear", "":
		return 0, true
	default:
		return 0, false
	}
}

// Control applies configured set/clear QoS during socket creation.
func (q QoSPolicy) Control(network, address string, c syscall.RawConn) error {
	tos, ok := q.configuredTOS()
	if !ok {
		return nil
	}
	return control(c, func(fd int) error {
		return setTOSFD(fd, tos)
	})
}

// Apply applies configured set/clear QoS to an established socket.
func (q QoSPolicy) Apply(sock any) error {
	tos, ok := q.configuredTOS()
	if !ok {
		return nil
	}
	return ApplyTOS(sock, tos)
}

// ApplyTOS sets the IP TOS / IPv6 traffic class byte on conn.
func ApplyTOS(sock any, tos int) error {
	rc, err := syscallConn(sock)
	if err != nil {
		return err
	}
	return control(rc, func(fd int) error {
		return setTOSFD(fd, tos)
	})
}

// GetTOS returns the current IP TOS / IPv6 traffic class byte from conn.
func GetTOS(conn net.Conn) (int, error) {
	if tracked, ok := conn.(LastReceivedTOSConn); ok {
		if tos, ok := tracked.LastReceivedTOS(); ok {
			return tos, nil
		}
	}
	rc, err := syscallConn(conn)
	if err != nil {
		return 0, err
	}
	var tos int
	err = control(rc, func(fd int) error {
		var getErr error
		tos, getErr = unix.GetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TOS)
		if getErr == nil {
			return nil
		}
		tos, getErr = unix.GetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_TCLASS)
		return getErr
	})
	if err != nil {
		return 0, err
	}
	return tos, nil
}

// EnableReceiveTOS enables delivery of inbound TOS / traffic-class control data.
func EnableReceiveTOS(sock any) error {
	rc, err := syscallConn(sock)
	if err != nil {
		return err
	}
	return EnableReceiveTOSControl(rc)
}

// EnableReceiveTOSControl enables inbound TOS / traffic-class control messages
// on a socket during creation.
func EnableReceiveTOSControl(c syscall.RawConn) error {
	return control(c, func(fd int) error {
		err4 := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_RECVTOS, 1)
		err6 := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_RECVTCLASS, 1)
		if err4 == nil || err6 == nil {
			return nil
		}
		if errors.Is(err4, unix.ENOPROTOOPT) || errors.Is(err4, unix.EAFNOSUPPORT) {
			return err6
		}
		return err4
	})
}

func syscallConn(sock any) (syscall.RawConn, error) {
	if tlsConn, ok := sock.(*tls.Conn); ok {
		sock = tlsConn.NetConn()
	}
	sc, ok := sock.(syscall.Conn)
	if !ok {
		return nil, fmt.Errorf("qos: %T does not expose syscall conn", sock)
	}
	return sc.SyscallConn()
}

func control(c syscall.RawConn, fn func(fd int) error) error {
	var err error
	if controlErr := c.Control(func(fd uintptr) {
		err = fn(int(fd))
	}); controlErr != nil {
		return controlErr
	}
	return err
}

func setTOSFD(fd, tos int) error {
	err4 := unix.SetsockoptInt(fd, unix.IPPROTO_IP, unix.IP_TOS, tos)
	err6 := unix.SetsockoptInt(fd, unix.IPPROTO_IPV6, unix.IPV6_TCLASS, tos)
	if err4 == nil || err6 == nil {
		return nil
	}
	return err4
}
