package peer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
)

// readLoop reads messages from conn and dispatches them to the appropriate handler.
// It signals readErrCh when the loop exits.
func (p *Peer) readLoop(ctx context.Context, conn net.Conn, readErrCh chan<- error) {
	for {
		// Check for stop signal
		select {
		case <-ctx.Done():
			readErrCh <- ctx.Err()
			return
		case <-p.stopCh:
			return
		default:
		}

		msg, err := message.Decode(conn)
		if err != nil {
			if isClosedConnError(err) || errors.Is(err, io.EOF) {
				readErrCh <- fmt.Errorf("connection closed: %w", err)
			} else {
				p.log.Warn("read error", zap.Error(err))
				readErrCh <- err
			}
			return
		}

		p.dispatch(msg)
	}
}

// writeLoop reads messages from writeCh and writes them to conn.
// sessionDone is closed when the owning session ends; the loop exits immediately
// so it does not outlive the session and race with the next session's write loop.
// It signals writeErrCh when the loop exits due to a write error.
func (p *Peer) writeLoop(ctx context.Context, conn net.Conn, writeErrCh chan<- error, sessionDone <-chan struct{}) {
	for {
		select {
		case <-ctx.Done():
			writeErrCh <- ctx.Err()
			return
		case <-p.stopCh:
			return
		case <-sessionDone:
			return
		case msg, ok := <-p.writeCh:
			if !ok {
				return
			}
			encoded, err := msg.Encode()
			if err != nil {
				p.log.Error("failed to encode message for write", zap.Error(err),
					zap.Uint32("cmd", msg.Header.CommandCode))
				continue
			}
			if _, err := conn.Write(encoded); err != nil {
				if isClosedConnError(err) {
					writeErrCh <- fmt.Errorf("connection closed during write: %w", err)
				} else {
					p.log.Warn("write error", zap.Error(err))
					writeErrCh <- err
				}
				return
			}
		}
	}
}

// dispatch routes an incoming message to the correct handler.
func (p *Peer) dispatch(msg *message.Message) {
	cmd := msg.Header.CommandCode
	isReq := msg.IsRequest()

	p.log.Debug("received message",
		zap.Uint32("cmd", cmd),
		zap.Bool("request", isReq),
		zap.Uint32("app_id", msg.Header.AppID),
		zap.Uint32("hop_by_hop", msg.Header.HopByHop),
	)

	switch cmd {
	case message.CmdCapabilitiesExchange:
		if isReq {
			// Inbound CER
			handleCER(p, msg)
		} else {
			// CEA response to our CER
			handleCEA(p, msg)
		}

	case message.CmdDeviceWatchdog:
		if isReq {
			handleDWR(p, msg)
		} else {
			handleDWA(p, msg)
		}

	case message.CmdDisconnectPeer:
		if isReq {
			handleDPR(p, msg)
		} else {
			handleDPA(p, msg)
		}

	default:
		// All other messages go to the application callback
		state := p.State()
		if state != StateOpen {
			p.log.Warn("received non-FSM message in non-OPEN state",
				zap.Uint32("cmd", cmd),
				zap.String("state", state.String()))
			if isReq {
				// Return UNABLE_TO_DELIVER
				if err := sendErrorAnswer(p, msg, message.DiameterUnableToDeliver, "peer not ready"); err != nil {
					p.log.Warn("failed to send error answer", zap.Error(err))
				}
			}
			return
		}

		if p.OnMessage != nil {
			p.OnMessage(p, msg)
		} else {
			p.log.Debug("no OnMessage handler set, dropping message",
				zap.Uint32("cmd", cmd))
		}
	}
}

// isClosedConnError returns true if the error is due to a closed connection.
func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}
	// Check for net.ErrClosed or common "use of closed network connection" string
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}
	return errors.Is(err, net.ErrClosed)
}
