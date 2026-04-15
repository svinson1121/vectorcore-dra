package router

import "sort"

// Rule is a compiled routing rule evaluated against incoming Diameter messages.
type Rule struct {
	Priority  int
	DestRealm string // match condition: "" = wildcard
	AppID     uint32 // match condition: 0 = wildcard
	DestHost string // routing target: specific peer FQDN; "" = use LBGroup
	LBGroup  string // routing target: lb group name; "" = auto-select
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
