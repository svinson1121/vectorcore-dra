package router

import (
	"errors"
	"sync"

	"go.uber.org/zap"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/avp"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/peer"
)

// AppRelayAgent is the Diameter Relay Application-ID (0xFFFFFFFF).
const AppRelayAgent uint32 = 0xFFFFFFFF

// Sentinel errors returned by Route.
var (
	ErrNoRoute      = errors.New("no matching route rule")
	ErrRejected     = errors.New("route action: reject")
	ErrDrop         = errors.New("route action: drop")
	ErrLoopDetected = errors.New("loop detected in Route-Record")
	ErrNoPeer       = errors.New("no open peer available")
)

// Router evaluates routing rules against Diameter messages and selects a peer.
type Router struct {
	mu         sync.RWMutex
	localID    string               // DRA's own Diameter identity (loop detection)
	rules      []Rule               // sorted by priority
	imsiRoutes imsiTable            // sorted by prefix length desc
	peers      map[string]*peer.Peer // keyed by FQDN
	groups     map[string][]string  // group name -> peer FQDNs
	groupLB    map[string]Selector  // group name -> LB selector
	log        *zap.Logger
}

// New creates a new Router.
func New(localID string, log *zap.Logger) *Router {
	return &Router{
		localID: localID,
		peers:   make(map[string]*peer.Peer),
		groups:  make(map[string][]string),
		groupLB: make(map[string]Selector),
		log:     log,
	}
}

// UpdateRules atomically replaces the route rule table.
func (r *Router) UpdateRules(rules []Rule) {
	sorted := Sorted(rules)
	r.mu.Lock()
	r.rules = sorted
	r.mu.Unlock()
	r.log.Info("routing rules updated", zap.Int("count", len(sorted)))
}

// UpdateIMSIRoutes atomically replaces the IMSI prefix table.
func (r *Router) UpdateIMSIRoutes(routes []IMSIRoute) {
	table := newIMSITable(routes)
	r.mu.Lock()
	r.imsiRoutes = table
	r.mu.Unlock()
	r.log.Info("IMSI routes updated", zap.Int("count", len(table)))
}

// SetGroupPolicy sets the load-balancing policy for a peer group.
// Called from peermgr when syncing peer group config.
func (r *Router) SetGroupPolicy(group, policy string) {
	r.mu.Lock()
	r.groupLB[group] = NewSelector(policy)
	r.mu.Unlock()
}

// AddPeer registers a peer in the routing table.
func (r *Router) AddPeer(p *peer.Peer) {
	fqdn := p.Cfg().FQDN
	peerGroup := p.Cfg().PeerGroup

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.peers[fqdn]; ok && existing == p {
		return
	}
	r.peers[fqdn] = p

	if peerGroup != "" {
		// Add to group if not already present
		fqdns := r.groups[peerGroup]
		found := false
		for _, f := range fqdns {
			if f == fqdn {
				found = true
				break
			}
		}
		if !found {
			r.groups[peerGroup] = append(fqdns, fqdn)
		}
		// Ensure group has a selector
		if _, ok := r.groupLB[peerGroup]; !ok {
			r.groupLB[peerGroup] = NewSelector("round_robin")
		}
	}

	r.log.Debug("peer added to router", zap.String("fqdn", fqdn), zap.String("group", peerGroup))
}

// RemovePeer unregisters a peer by FQDN.
func (r *Router) RemovePeer(fqdn string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.peers[fqdn]
	if !ok {
		return
	}
	delete(r.peers, fqdn)

	// Remove from peer group
	peerGroup := p.Cfg().PeerGroup
	if peerGroup != "" {
		fqdns := r.groups[peerGroup]
		filtered := fqdns[:0]
		for _, f := range fqdns {
			if f != fqdn {
				filtered = append(filtered, f)
			}
		}
		if len(filtered) == 0 {
			delete(r.groups, peerGroup)
		} else {
			r.groups[peerGroup] = filtered
		}
	}

	r.log.Debug("peer removed from router", zap.String("fqdn", fqdn))
}

// Route evaluates routing rules against msg and returns the selected peer.
// fromFQDN is the config FQDN of the peer that originated the request; it is
// excluded from all candidate sets so the DRA never routes a request back to
// the peer it came from.
// Routing tiers (first match wins):
//  1. IMSI prefix - extract IMSI, match prefix table -> set Destination-Realm, route to group
//  2. Explicit host - Destination-Host matches a peer FQDN exactly
//  3. Realm + AppID - Destination-Realm + AppID match a route rule
//  4. Realm only - Destination-Realm matches, any AppID
//  5. Default - catch-all (empty dest_realm)
func (r *Router) Route(msg *message.Message, fromFQDN string) (*peer.Peer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// --- Loop detection ---
	for _, a := range msg.FindAVPs(avp.CodeRouteRecord, 0) {
		if ident, err := a.String(); err == nil && ident == r.localID {
			r.log.Warn("loop detected", zap.String("local_id", r.localID))
			return nil, ErrLoopDetected
		}
	}

	// --- Extract routing keys ---
	destHost := avpString(msg, avp.CodeDestinationHost, 0)
	destRealm := avpString(msg, avp.CodeDestinationRealm, 0)
	appID := msg.Header.AppID

	// --- Tier 1: IMSI prefix routing ---
	if imsi := extractIMSI(msg); imsi != "" {
		if route, ok := r.imsiRoutes.match(imsi); ok {
			r.log.Debug("IMSI route matched",
				zap.String("imsi_prefix", route.Prefix),
				zap.String("dest_realm", route.DestRealm),
				zap.String("peer_group", route.PeerGroup),
			)
			// Override dest_realm for subsequent rule evaluation
			destRealm = route.DestRealm
			// Route directly to the peer group
			if p := r.selectFromGroup(route.PeerGroup, appID, destRealm, fromFQDN, msg); p != nil {
				return p, nil
			}
			r.log.Warn("IMSI route matched but no open peer in group",
				zap.String("prefix", route.Prefix),
				zap.String("group", route.PeerGroup),
			)
			// Fall through to rule-based routing with the overridden destRealm
		}
	}

	// --- Tiers 2-4: explicit route_rules (optional) ---
	// route_rules are only needed for: explicit rejects, routing a realm to a
	// specific peer group, or a catch-all default action. Normal realm routing
	// works without any rules via the implicit tier below.
	for _, rule := range r.rules {
		if !rule.Enabled {
			continue
		}
		if rule.DestHost != "" && rule.DestHost != destHost {
			continue
		}
		if rule.DestRealm != "" && rule.DestRealm != destRealm {
			continue
		}
		if rule.AppID != 0 && rule.AppID != appID {
			continue
		}

		switch rule.Action {
		case "reject":
			r.log.Debug("route rejected by rule", zap.Int("priority", rule.Priority))
			return nil, ErrRejected
		case "drop":
			r.log.Debug("route dropped by rule", zap.Int("priority", rule.Priority))
			return nil, ErrDrop
		case "route":
			if rule.Peer != "" {
				p, ok := r.peers[rule.Peer]
				if !ok || p.State() != peer.StateOpen {
					r.log.Warn("static route peer not open", zap.String("peer", rule.Peer))
					return nil, ErrNoPeer
				}
				return p, nil
			}
			if rule.PeerGroup != "" {
				if p := r.selectFromGroup(rule.PeerGroup, appID, destRealm, fromFQDN, msg); p != nil {
					r.log.Debug("rule route selected",
						zap.String("group", rule.PeerGroup),
						zap.Int("priority", rule.Priority),
					)
					return p, nil
				}
				r.log.Warn("no open peers in group, trying next rule",
					zap.String("group", rule.PeerGroup),
					zap.Int("priority", rule.Priority),
				)
				continue
			}
			candidates := r.allCandidates(appID, destRealm, fromFQDN)
			if len(candidates) == 0 {
				continue
			}
			if p := r.defaultSelector().Select(candidates, msg); p != nil {
				return p, nil
			}
			continue
		}
	}

	// --- Tier 4b: implicit Destination-Host routing ---
	// If Destination-Host is set and matches a configured peer FQDN exactly,
	// route directly to that peer. This handles server-initiated requests (e.g.
	// RAR from HSS targeting a specific MME) that carry no route rules but do
	// carry an explicit Destination-Host. Mirrors freeDiameter routing_dispatch.c
	// behaviour: explicit host match is always tried before realm-only fallback.
	if destHost != "" {
		if p, ok := r.peers[destHost]; ok && destHost != fromFQDN {
			if p.State() == peer.StateOpen {
				r.log.Debug("implicit dest-host route",
					zap.String("dest_host", destHost),
					zap.String("peer", p.Cfg().Name),
				)
				return p, nil
			}
			r.log.Warn("implicit dest-host peer not open",
				zap.String("dest_host", destHost),
			)
			return nil, ErrNoPeer
		}
	}

	// --- Tier 5: implicit realm routing (no route_rules required) ---
	// Route Destination-Realm to any OPEN peer whose configured realm matches.
	// This mirrors freeDiameter's default behaviour - peers declare their realm
	// in config and CER, and the DRA routes to them automatically.
	if destRealm != "" {
		candidates := r.allCandidates(appID, destRealm, fromFQDN)
		if len(candidates) > 0 {
			if p := r.defaultSelector().Select(candidates, msg); p != nil {
				r.log.Debug("implicit realm route",
					zap.String("dest_realm", destRealm),
					zap.String("peer", p.Cfg().Name),
				)
				return p, nil
			}
		}
	}

	return nil, ErrNoRoute
}

// selectFromGroup picks an open peer from the named group that supports appID.
// excludeFQDN is skipped (the peer that originated the request).
func (r *Router) selectFromGroup(group string, appID uint32, destRealm string, excludeFQDN string, msg *message.Message) *peer.Peer {
	fqdns := r.groups[group]
	if len(fqdns) == 0 {
		return nil
	}
	candidates := make([]*peer.Peer, 0, len(fqdns))
	for _, fqdn := range fqdns {
		if fqdn == excludeFQDN {
			continue
		}
		p, ok := r.peers[fqdn]
		if !ok || p.State() != peer.StateOpen {
			continue
		}
		if destRealm != "" && p.PeerRealm != destRealm {
			continue
		}
		if appID != 0 && appID != AppRelayAgent && !peerSupportsApp(p, appID) {
			continue
		}
		candidates = append(candidates, p)
	}
	if len(candidates) == 0 {
		return nil
	}
	sel := r.groupLB[group]
	if sel == nil {
		sel = r.defaultSelector()
	}
	return sel.Select(candidates, msg)
}

// allCandidates returns all OPEN peers eligible for appID and destRealm,
// excluding excludeFQDN (the peer that originated the request).
func (r *Router) allCandidates(appID uint32, destRealm string, excludeFQDN string) []*peer.Peer {
	var out []*peer.Peer
	for fqdn, p := range r.peers {
		if fqdn == excludeFQDN {
			continue
		}
		if p.State() != peer.StateOpen {
			continue
		}
		if destRealm != "" && p.PeerRealm != destRealm {
			continue
		}
		if appID != 0 && appID != AppRelayAgent && !peerSupportsApp(p, appID) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (r *Router) defaultSelector() Selector {
	return &roundRobinSelector{}
}

// peerSupportsApp returns true if p advertises appID or the relay application.
func peerSupportsApp(p *peer.Peer, appID uint32) bool {
	for _, id := range p.PeerAppIDs {
		if id == appID || id == AppRelayAgent {
			return true
		}
	}
	return false
}

// avpString is a helper to extract a string AVP value from a message.
func avpString(msg *message.Message, code uint32, vendorID uint32) string {
	a := msg.FindAVP(code, vendorID)
	if a == nil {
		return ""
	}
	s, err := a.String()
	if err != nil {
		return ""
	}
	return s
}
