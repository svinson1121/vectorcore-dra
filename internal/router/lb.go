package router

import (
	"hash/fnv"
	"sync/atomic"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/avp"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/peer"
)

// Selector picks a peer from a list of candidates for a given message.
type Selector interface {
	Select(peers []*peer.Peer, msg *message.Message) *peer.Peer
}

// NewSelector returns a Selector for the named policy.
// Unknown policies fall back to round_robin.
func NewSelector(policy string) Selector {
	switch policy {
	case "least_conn":
		return &leastConnSelector{}
	default:
		return &roundRobinSelector{}
	}
}

// openPeers returns only the peers that are currently in the OPEN state.
func openPeers(peers []*peer.Peer) []*peer.Peer {
	var out []*peer.Peer
	for _, p := range peers {
		if p.State() == peer.StateOpen {
			out = append(out, p)
		}
	}
	return out
}

// sessionAffinityPeer checks msg for a Session-Id AVP (code 263) and, if present,
// uses an FNV hash of the Session-Id to pick a peer from open peers deterministically.
// Returns nil if no Session-Id AVP is found or if open is empty.
func sessionAffinityPeer(open []*peer.Peer, msg *message.Message) *peer.Peer {
	if len(open) == 0 {
		return nil
	}
	sidAVP := msg.FindAVP(avp.CodeSessionID, 0)
	if sidAVP == nil {
		return nil
	}
	sid, err := sidAVP.String()
	if err != nil || sid == "" {
		return nil
	}
	h := fnv.New32a()
	h.Write([]byte(sid))
	idx := int(h.Sum32()) % len(open)
	return open[idx]
}

// --- Round Robin ---

type roundRobinSelector struct {
	counter uint64
}

func (s *roundRobinSelector) Select(peers []*peer.Peer, msg *message.Message) *peer.Peer {
	open := openPeers(peers)
	if len(open) == 0 {
		return nil
	}
	// Session affinity overrides round-robin
	if p := sessionAffinityPeer(open, msg); p != nil {
		return p
	}
	idx := atomic.AddUint64(&s.counter, 1) % uint64(len(open))
	return open[idx]
}

// --- Least Connections ---

type leastConnSelector struct{}

func (s *leastConnSelector) Select(peers []*peer.Peer, msg *message.Message) *peer.Peer {
	open := openPeers(peers)
	if len(open) == 0 {
		return nil
	}
	// Session affinity overrides least-conn selection
	if p := sessionAffinityPeer(open, msg); p != nil {
		return p
	}

	best := open[0]
	bestInFlight := best.InFlight()
	for _, p := range open[1:] {
		if inf := p.InFlight(); inf < bestInFlight {
			best = p
			bestInFlight = inf
		}
	}
	return best
}
