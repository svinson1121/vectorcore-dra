package peer

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/avp"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
	"github.com/svinson1121/vectorcore-dra/internal/metrics"
)

// watchdogLoop sends DWR at the configured interval and tracks failures.
// It exits when done is closed.
func (p *Peer) watchdogLoop(ctx interface{ Done() <-chan struct{} }, done <-chan struct{}) {
	ticker := time.NewTicker(p.cfg.WatchdogInterval)
	defer ticker.Stop()

	pendingReply := false
	failures := 0

	for {
		select {
		case <-done:
			return
		case <-p.wdReplyCh:
			// DWA received - clear pending and reset failure count
			pendingReply = false
			failures = 0
		case <-ticker.C:
			if pendingReply {
				failures++
				p.log.Warn("watchdog: no DWA received",
					zap.Int("consecutive_failures", failures),
					zap.Int("max_failures", p.cfg.WatchdogMaxFail),
				)
				metrics.WatchdogTimeoutTotal.WithLabelValues(p.cfg.FQDN).Inc()
				if failures >= p.cfg.WatchdogMaxFail {
					p.log.Error("watchdog: max failures exceeded, declaring peer down")
					p.mu.RLock()
					conn := p.conn
					p.mu.RUnlock()
					if conn != nil {
						conn.Close()
					}
					return
				}
			}
			if err := sendDWR(p); err != nil {
				p.log.Warn("watchdog: failed to send DWR", zap.Error(err))
			} else {
				pendingReply = true
				metrics.WatchdogSentTotal.WithLabelValues(p.cfg.FQDN).Inc()
			}
		}
	}
}

// onDWAReceived signals the watchdog goroutine that a DWA was received.
func (p *Peer) onDWAReceived() {
	select {
	case p.wdReplyCh <- struct{}{}:
	default:
	}
}

// sendDWR sends a Device-Watchdog-Request to the peer.
func sendDWR(p *Peer) error {
	dwr := buildDWR(p)
	encoded, err := dwr.Encode()
	if err != nil {
		return fmt.Errorf("dwr: encoding DWR: %w", err)
	}

	p.mu.RLock()
	conn := p.conn
	p.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("dwr: no connection")
	}

	_, err = conn.Write(encoded)
	return err
}

// buildDWR constructs the DWR message.
func buildDWR(p *Peer) *message.Message {
	b := message.NewRequest(message.CmdDeviceWatchdog, message.AppDiameterCommon)
	b.Add(
		avp.NewString(avp.CodeOriginHost, 0, avp.FlagMandatory, p.cfg.LocalFQDN),
		avp.NewString(avp.CodeOriginRealm, 0, avp.FlagMandatory, p.cfg.LocalRealm),
	)
	b.NonProxiable()
	return b.Build()
}

// buildDWA constructs a DWA in response to a DWR.
func buildDWA(p *Peer, req *message.Message) *message.Message {
	b := message.NewAnswer(req)
	b.Add(
		avp.NewUint32(avp.CodeResultCode, 0, avp.FlagMandatory, message.DiameterSuccess),
		avp.NewString(avp.CodeOriginHost, 0, avp.FlagMandatory, p.cfg.LocalFQDN),
		avp.NewString(avp.CodeOriginRealm, 0, avp.FlagMandatory, p.cfg.LocalRealm),
	)
	return b.Build()
}

// handleDWR processes an incoming DWR and sends DWA.
func handleDWR(p *Peer, req *message.Message) {
	dwa := buildDWA(p, req)
	encoded, err := dwa.Encode()
	if err != nil {
		p.log.Error("failed to encode DWA", zap.Error(err))
		return
	}

	p.mu.RLock()
	conn := p.conn
	p.mu.RUnlock()

	if conn == nil {
		return
	}

	if _, err := conn.Write(encoded); err != nil {
		p.log.Warn("failed to send DWA", zap.Error(err))
	}
}

// handleDWA processes an incoming DWA.
func handleDWA(p *Peer, dwa *message.Message) {
	// Verify result code if present
	if rcAVP := dwa.FindAVP(avp.CodeResultCode, 0); rcAVP != nil {
		if rc, err := rcAVP.Uint32(); err == nil && rc != message.DiameterSuccess {
			p.log.Warn("DWA non-success result code", zap.Uint32("result_code", rc))
		}
	}
	p.onDWAReceived()
}
