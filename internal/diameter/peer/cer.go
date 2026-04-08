package peer

import (
	"fmt"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/avp"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
)

// originStateID is set once at process start and used in all CER/CEA messages.
// Peers use it to detect if we have restarted (RFC 6733 8.16).
var originStateID uint32

func init() {
	atomic.StoreUint32(&originStateID, uint32(time.Now().Unix()))
}

// sendCER builds and sends a Capabilities-Exchange-Request to the peer.
func sendCER(p *Peer) error {
	cer := buildCER(p)
	encoded, err := cer.Encode()
	if err != nil {
		return fmt.Errorf("cer: encoding CER: %w", err)
	}

	p.mu.RLock()
	conn := p.conn
	p.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("cer: no connection")
	}

	_, err = conn.Write(encoded)
	return err
}

// buildCER creates the CER message for outbound capability exchange.
func buildCER(p *Peer) *message.Message {
	b := message.NewRequest(message.CmdCapabilitiesExchange, message.AppDiameterCommon)
	b.Add(
		avp.NewString(avp.CodeOriginHost, 0, avp.FlagMandatory, p.cfg.LocalFQDN),
		avp.NewString(avp.CodeOriginRealm, 0, avp.FlagMandatory, p.cfg.LocalRealm),
		avp.NewUint32(avp.CodeOriginStateID, 0, avp.FlagMandatory, atomic.LoadUint32(&originStateID)),
		avp.NewAddress(avp.CodeHostIPAddress, 0, avp.FlagMandatory, p.cfg.LocalIP),
		avp.NewUint32(avp.CodeVendorID, 0, avp.FlagMandatory, 0),
		avp.NewString(avp.CodeProductName, 0, 0, "VectorCore DRA"),
		avp.NewUint32(avp.CodeFirmwareRevision, 0, 0, 1),
		avp.NewUint32(avp.CodeInbandSecurityID, 0, avp.FlagMandatory, avp.InbandSecurityNoSec),
		avp.NewUint32(avp.CodeAuthApplicationID, 0, avp.FlagMandatory, message.AppRelayAgent),
		avp.NewUint32(avp.CodeSupportedVendorID, 0, avp.FlagMandatory, message.Vendor3GPP),
	)
	b.NonProxiable()
	return b.Build()
}

// buildCEA creates the CEA message responding to an inbound CER.
func buildCEA(p *Peer, req *message.Message) *message.Message {
	b := message.NewAnswer(req)
	b.Add(
		avp.NewUint32(avp.CodeResultCode, 0, avp.FlagMandatory, message.DiameterSuccess),
		avp.NewString(avp.CodeOriginHost, 0, avp.FlagMandatory, p.cfg.LocalFQDN),
		avp.NewString(avp.CodeOriginRealm, 0, avp.FlagMandatory, p.cfg.LocalRealm),
		avp.NewUint32(avp.CodeOriginStateID, 0, avp.FlagMandatory, atomic.LoadUint32(&originStateID)),
		avp.NewAddress(avp.CodeHostIPAddress, 0, avp.FlagMandatory, p.cfg.LocalIP),
		avp.NewUint32(avp.CodeVendorID, 0, avp.FlagMandatory, 0),
		avp.NewString(avp.CodeProductName, 0, 0, "VectorCore DRA"),
		avp.NewUint32(avp.CodeFirmwareRevision, 0, 0, 1),
		avp.NewUint32(avp.CodeAuthApplicationID, 0, avp.FlagMandatory, message.AppRelayAgent),
		avp.NewUint32(avp.CodeInbandSecurityID, 0, avp.FlagMandatory, avp.InbandSecurityNoSec),
		avp.NewUint32(avp.CodeSupportedVendorID, 0, avp.FlagMandatory, message.Vendor3GPP),
	)
	return b.Build()
}

// handleCER processes an incoming CER (inbound peer).
// It sends a CEA and signals the session that capability exchange is complete.
func handleCER(p *Peer, req *message.Message) {
	// Extract peer capabilities
	extractPeerCapabilities(p, req)

	// Build and send CEA
	cea := buildCEA(p, req)
	encoded, err := cea.Encode()
	if err != nil {
		p.log.Error("failed to encode CEA", zap.Error(err))
		p.ceaCh <- fmt.Errorf("encoding CEA: %w", err)
		return
	}

	p.mu.RLock()
	conn := p.conn
	p.mu.RUnlock()

	if conn == nil {
		p.ceaCh <- fmt.Errorf("no connection when sending CEA")
		return
	}

	if _, err := conn.Write(encoded); err != nil {
		p.log.Error("failed to send CEA", zap.Error(err))
		p.ceaCh <- fmt.Errorf("sending CEA: %w", err)
		return
	}

	p.log.Info("sent CEA", zap.String("peerFQDN", p.PeerFQDN))
	p.ceaCh <- nil
}

// handleCEA processes an incoming CEA (outbound peer, response to our CER).
func handleCEA(p *Peer, cea *message.Message) {
	// Check result code
	rcAVP := cea.FindAVP(avp.CodeResultCode, 0)
	if rcAVP == nil {
		p.ceaCh <- fmt.Errorf("CEA missing Result-Code AVP")
		return
	}
	rc, err := rcAVP.Uint32()
	if err != nil {
		p.ceaCh <- fmt.Errorf("decoding Result-Code: %w", err)
		return
	}
	if rc != message.DiameterSuccess {
		p.ceaCh <- fmt.Errorf("CEA returned error result code: %d", rc)
		return
	}

	// Extract peer capabilities
	extractPeerCapabilities(p, cea)

	p.log.Info("CEA accepted", zap.String("peerFQDN", p.PeerFQDN), zap.String("peerRealm", p.PeerRealm))
	p.ceaCh <- nil
}

// extractPeerCapabilities parses the peer's identity and application IDs from a CER or CEA.
func extractPeerCapabilities(p *Peer, msg *message.Message) {
	if a := msg.FindAVP(avp.CodeOriginHost, 0); a != nil {
		if s, err := a.String(); err == nil {
			p.PeerFQDN = s
		}
	}
	if a := msg.FindAVP(avp.CodeOriginRealm, 0); a != nil {
		if s, err := a.String(); err == nil {
			p.PeerRealm = s
		}
	}

	// Collect Auth-Application-Id AVPs
	var appIDs []uint32
	for _, a := range msg.FindAVPs(avp.CodeAuthApplicationID, 0) {
		if id, err := a.Uint32(); err == nil {
			appIDs = append(appIDs, id)
		}
	}
	// Also collect from Vendor-Specific-Application-Id grouped AVPs
	for _, a := range msg.FindAVPs(avp.CodeVendorSpecificApplicationID, 0) {
		children, err := avp.DecodeGrouped(a)
		if err != nil {
			continue
		}
		for _, child := range children {
			if child.Code == avp.CodeAuthApplicationID || child.Code == avp.CodeAcctApplicationID {
				if id, err := child.Uint32(); err == nil {
					appIDs = append(appIDs, id)
				}
			}
		}
	}
	// Deduplicate app IDs — peers often advertise the same app ID both as a
	// plain Auth-Application-Id and inside a Vendor-Specific-Application-Id group.
	seen := make(map[uint32]struct{}, len(appIDs))
	deduped := appIDs[:0]
	for _, id := range appIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			deduped = append(deduped, id)
		}
	}
	p.PeerAppIDs = deduped
}
