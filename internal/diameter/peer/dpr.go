package peer

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/avp"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
)

// sendDPR sends a Disconnect-Peer-Request with Disconnect-Cause = REBOOTING.
func sendDPR(p *Peer) {
	dpr := buildDPR(p)
	encoded, err := dpr.Encode()
	if err != nil {
		p.log.Error("failed to encode DPR", zap.Error(err))
		return
	}

	p.mu.RLock()
	conn := p.conn
	p.mu.RUnlock()

	if conn == nil {
		return
	}

	if _, err := conn.Write(encoded); err != nil {
		p.log.Warn("failed to send DPR", zap.Error(err))
	} else {
		p.log.Info("sent DPR")
	}
}

// buildDPR constructs the DPR message.
func buildDPR(p *Peer) *message.Message {
	b := message.NewRequest(message.CmdDisconnectPeer, message.AppDiameterCommon)
	b.Add(
		avp.NewString(avp.CodeOriginHost, 0, avp.FlagMandatory, p.cfg.LocalFQDN),
		avp.NewString(avp.CodeOriginRealm, 0, avp.FlagMandatory, p.cfg.LocalRealm),
		avp.NewUint32(avp.CodeDisconnectCause, 0, avp.FlagMandatory, avp.DisconnectCauseRebooting),
	)
	b.NonProxiable()
	return b.Build()
}

// buildDPA constructs a DPA in response to a DPR.
func buildDPA(p *Peer, req *message.Message) *message.Message {
	b := message.NewAnswer(req)
	b.Add(
		avp.NewUint32(avp.CodeResultCode, 0, avp.FlagMandatory, message.DiameterSuccess),
		avp.NewString(avp.CodeOriginHost, 0, avp.FlagMandatory, p.cfg.LocalFQDN),
		avp.NewString(avp.CodeOriginRealm, 0, avp.FlagMandatory, p.cfg.LocalRealm),
	)
	return b.Build()
}

// handleDPR processes an incoming DPR: sends DPA, then signals session to close.
func handleDPR(p *Peer, req *message.Message) {
	// Extract and log disconnect cause
	if causeAVP := req.FindAVP(avp.CodeDisconnectCause, 0); causeAVP != nil {
		if cause, err := causeAVP.Uint32(); err == nil {
			p.log.Info("received DPR", zap.Uint32("disconnect_cause", cause))
		}
	}

	// Send DPA
	dpa := buildDPA(p, req)
	encoded, err := dpa.Encode()
	if err != nil {
		p.log.Error("failed to encode DPA", zap.Error(err))
	} else {
		p.mu.RLock()
		conn := p.conn
		p.mu.RUnlock()

		if conn != nil {
			if _, err := conn.Write(encoded); err != nil {
				p.log.Warn("failed to send DPA", zap.Error(err))
			} else {
				p.log.Info("sent DPA")
			}
		}
	}

	// Signal the session loop to close (non-blocking)
	select {
	case p.dprDoneCh <- struct{}{}:
	default:
	}

	// Close the connection
	p.mu.RLock()
	conn := p.conn
	p.mu.RUnlock()
	if conn != nil {
		conn.Close()
	}
}

// handleDPA processes an incoming DPA (response to our DPR).
func handleDPA(p *Peer, dpa *message.Message) {
	if rcAVP := dpa.FindAVP(avp.CodeResultCode, 0); rcAVP != nil {
		if rc, err := rcAVP.Uint32(); err == nil {
			p.log.Info("received DPA", zap.Uint32("result_code", rc))
		}
	} else {
		p.log.Info("received DPA (no result code)")
	}

	// Signal that DPA was received
	select {
	case p.dprDoneCh <- struct{}{}:
	default:
	}
}

// sendErrorAnswer sends an error answer for an unrecognized or unroutable request.
func sendErrorAnswer(p *Peer, req *message.Message, resultCode uint32, errMsg string) error {
	ansMsg := message.NewAnswer(req).Add(
		avp.NewUint32(avp.CodeResultCode, 0, avp.FlagMandatory, resultCode),
		avp.NewString(avp.CodeOriginHost, 0, avp.FlagMandatory, p.cfg.LocalFQDN),
		avp.NewString(avp.CodeOriginRealm, 0, avp.FlagMandatory, p.cfg.LocalRealm),
	)
	if errMsg != "" {
		ansMsg.Add(avp.NewString(avp.CodeErrorMessage, 0, 0, errMsg))
	}
	built := ansMsg.Build()
	built.Header.Flags |= message.FlagError

	encoded, err := built.Encode()
	if err != nil {
		return fmt.Errorf("encoding error answer: %w", err)
	}

	p.mu.RLock()
	conn := p.conn
	p.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("no connection")
	}

	_, err = conn.Write(encoded)
	return err
}
