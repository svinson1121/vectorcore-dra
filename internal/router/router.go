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

// SetGroupPolicy sets the load-balancing policy for an lb group.
// Called from peermgr when syncing lb group config.
func (r *Router) SetGroupPolicy(lbGroup, policy string) {
	r.mu.Lock()
	r.groupLB[lbGroup] = NewSelector(policy)
	r.mu.Unlock()
}

// AddPeer registers a peer in the routing table.
func (r *Router) AddPeer(p *peer.Peer) {
	fqdn := p.Cfg().FQDN
	lbGroup := p.Cfg().LBGroup

	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, ok := r.peers[fqdn]; ok && existing == p {
		return
	}
	r.peers[fqdn] = p

	if lbGroup != "" {
		// Add to group if not already present
		fqdns := r.groups[lbGroup]
		found := false
		for _, f := range fqdns {
			if f == fqdn {
				found = true
				break
			}
		}
		if !found {
			r.groups[lbGroup] = append(fqdns, fqdn)
		}
		// Ensure group has a selector
		if _, ok := r.groupLB[lbGroup]; !ok {
			r.groupLB[lbGroup] = NewSelector("round_robin")
		}
	}

	r.log.Debug("peer added to router", zap.String("fqdn", fqdn), zap.String("lb_group", lbGroup))
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

	// Remove from lb group
	lbGroup := p.Cfg().LBGroup
	if lbGroup != "" {
		fqdns := r.groups[lbGroup]
		filtered := fqdns[:0]
		for _, f := range fqdns {
			if f != fqdn {
				filtered = append(filtered, f)
			}
		}
		if len(filtered) == 0 {
			delete(r.groups, lbGroup)
		} else {
			r.groups[lbGroup] = filtered
		}
	}

	r.log.Debug("peer removed from router", zap.String("fqdn", fqdn))
}

// Route evaluates routing rules against msg and returns the selected peer.
// fromFQDN is the config FQDN of the peer that originated the request; it is
// excluded from all candidate sets so the DRA never routes a request back to
// the peer it came from.
// Routing tiers (first match wins):
//  1. Destination-Host AVP in message - route directly, no rule evaluation
//  2. Explicit route_rules - match on dest_realm + app_id, route to dest_host peer or lb_group
//  3. IMSI prefix - derive Destination-Realm from IMSI, route to peer group
//  4. Implicit realm - Destination-Realm matches any OPEN peer's configured realm
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

	// Determine the source peer's group so implicit routing excludes the entire
	// group, not just the individual peer. This prevents S6a requests from one
	// MME being routed back to another MME in the same group.
	fromGroup := ""
	if src, ok := r.peers[fromFQDN]; ok {
		fromGroup = src.Cfg().LBGroup
	}

	// --- Tier 1: Destination-Host in the message ---
	// If the message carries a Destination-Host AVP, route directly to that peer.
	// No rule evaluation, no load balancing - the originator made an explicit
	// routing decision and overriding it would break mid-session flows (RAR, IDR,
	// ASR, CCR-U, etc.). This is evaluated before everything else.
	if destHost != "" {
		if p, ok := r.peers[destHost]; ok && destHost != fromFQDN {
			if p.State() == peer.StateOpen {
				r.log.Debug("dest-host route",
					zap.String("dest_host", destHost),
					zap.String("peer", p.Cfg().Name),
				)
				return p, nil
			}
			r.log.Warn("dest-host peer not open",
				zap.String("dest_host", destHost),
			)
			return nil, ErrNoPeer
		}
	}

	// --- Tier 2: explicit route_rules ---
	// Static rules evaluated in ascending priority order; first match wins.
	// Match conditions: dest_realm and app_id.
	// Routing target (in order of precedence):
	//   dest_host set   -> route to that specific peer
	//   lb_group set    -> load-balance across the group
	//   neither set     -> route to any OPEN peer whose realm matches
	for _, rule := range r.rules {
		if !rule.Enabled {
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
			if rule.DestHost != "" {
				// dest_host in the rule is the routing target - the peer FQDN to send to.
				p, ok := r.peers[rule.DestHost]
				if !ok || p.State() != peer.StateOpen {
					r.log.Warn("rule dest_host peer not open, trying next rule",
						zap.String("dest_host", rule.DestHost),
						zap.Int("priority", rule.Priority),
					)
					continue
				}
				r.log.Debug("rule routed to dest_host peer",
					zap.String("dest_host", rule.DestHost),
					zap.Int("priority", rule.Priority),
				)
				return p, nil
			}
			if rule.LBGroup != "" {
				if p := r.selectFromGroup(rule.LBGroup, appID, destRealm, fromFQDN, msg, false); p != nil {
					r.log.Debug("rule routed to lb group",
						zap.String("lb_group", rule.LBGroup),
						zap.Int("priority", rule.Priority),
					)
					return p, nil
				}
				r.log.Warn("no open peers in lb group, trying next rule",
					zap.String("lb_group", rule.LBGroup),
					zap.Int("priority", rule.Priority),
				)
				continue
			}
			candidates := r.allCandidates(appID, destRealm, fromFQDN, fromGroup)
			if len(candidates) == 0 {
				continue
			}
			if p := r.defaultSelector().Select(candidates, msg); p != nil {
				return p, nil
			}
			continue
		}
	}

	// --- Tier 2: IMSI prefix routing ---
	// Only reached when no explicit route_rule matched. Extracts IMSI from the
	// message, derives the Destination-Realm, and routes to the configured lb group.
	if imsi := extractIMSI(msg); imsi != "" {
		if route, ok := r.imsiRoutes.match(imsi); ok {
			r.log.Debug("IMSI route matched",
				zap.String("imsi_prefix", route.Prefix),
				zap.String("dest_realm", route.DestRealm),
				zap.String("lb_group", route.LBGroup),
			)
			imsiRealm := route.DestRealm
			if p := r.selectFromGroup(route.LBGroup, appID, imsiRealm, fromFQDN, msg, true); p != nil {
				return p, nil
			}
			r.log.Warn("IMSI route matched but no open peer in lb group",
				zap.String("prefix", route.Prefix),
				zap.String("lb_group", route.LBGroup),
			)
			// Fall through to implicit routing with the IMSI-derived realm.
			destRealm = imsiRealm
		}
	}

	// --- Tier 4: implicit realm routing ---
	// Route Destination-Realm to any OPEN peer whose configured realm matches.
	// fromGroup is excluded so requests are never routed back to the source peer group.
	if destRealm != "" {
		candidates := r.allCandidates(appID, destRealm, fromFQDN, fromGroup)
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

	r.log.Warn("no route found",
		zap.String("dest_host", destHost),
		zap.String("dest_realm", destRealm),
		zap.Uint32("app_id", appID),
		zap.String("from", fromFQDN),
	)
	return nil, ErrNoRoute
}

// selectFromGroup picks an open peer from the named group that supports appID.
// excludeFQDN is skipped (the peer that originated the request).
// filterByRealm controls whether p.PeerRealm must match destRealm. Pass true for
// IMSI-derived routing (where the realm was just computed and must match exactly),
// false for explicit rule-driven routing (operator chose the group; realm filter
// could silently exclude all peers if CER-advertised realm differs from Destination-Realm).
func (r *Router) selectFromGroup(group string, appID uint32, destRealm string, excludeFQDN string, msg *message.Message, filterByRealm bool) *peer.Peer {
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
		if filterByRealm && destRealm != "" && p.PeerRealm != destRealm {
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
// excluding excludeFQDN (the originating peer) and all peers in excludeGroup
// (the originating peer's group). Excluding the whole group prevents requests
// from being routed back to a peer of the same type (e.g. MME -> MME).
func (r *Router) allCandidates(appID uint32, destRealm string, excludeFQDN string, excludeGroup string) []*peer.Peer {
	var out []*peer.Peer
	for fqdn, p := range r.peers {
		if fqdn == excludeFQDN {
			continue
		}
		if excludeGroup != "" && p.Cfg().LBGroup == excludeGroup {
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

// peerSupportsApp returns true if p advertises appID or the relay application,
// or if the peer has no advertised app IDs at all (relay-only peer or CER not
// yet captured - assume capable rather than silently blackholing traffic).
func peerSupportsApp(p *peer.Peer, appID uint32) bool {
	if len(p.PeerAppIDs) == 0 {
		return true
	}
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
