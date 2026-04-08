package router

import "sort"

// Rule is a compiled routing rule evaluated against incoming Diameter messages.
type Rule struct {
	Priority  int
	DestHost  string // "" = wildcard
	DestRealm string // "" = wildcard
	AppID     uint32 // 0 = wildcard
	PeerGroup string // peer group name; "" = any open peer
	Peer      string // specific peer FQDN for static routing; "" = auto-select
	Action    string // "route", "reject", "drop"
	Enabled   bool
}

// Sorted returns a copy of rules sorted by priority ascending (lower = evaluated first).
func Sorted(rules []Rule) []Rule {
	out := make([]Rule, len(rules))
	copy(out, rules)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Priority < out[j].Priority
	})
	return out
}
