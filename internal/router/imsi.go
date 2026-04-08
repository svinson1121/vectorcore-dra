package router

import (
	"sort"
	"strings"

	"github.com/svinson1121/vectorcore-dra/internal/diameter/avp"
	"github.com/svinson1121/vectorcore-dra/internal/diameter/message"
)

// IMSIRoute maps an IMSI MCC+MNC prefix to a destination realm and peer group.
// Mirrors config.IMSIRoute but lives in the router package to avoid import cycles.
type IMSIRoute struct {
	Prefix    string // e.g. "311435" (MCC=311 MNC=435)
	DestRealm string
	PeerGroup string
	Priority  int
	Enabled   bool
}

// imsiTable is a sorted, immutable snapshot of IMSI routes.
// Sorted by prefix length descending (longest first) so longest-prefix wins,
// then by Priority ascending within equal-length prefixes.
type imsiTable []IMSIRoute

func newIMSITable(routes []IMSIRoute) imsiTable {
	enabled := make([]IMSIRoute, 0, len(routes))
	for _, r := range routes {
		if r.Enabled {
			enabled = append(enabled, r)
		}
	}
	sort.Slice(enabled, func(i, j int) bool {
		li, lj := len(enabled[i].Prefix), len(enabled[j].Prefix)
		if li != lj {
			return li > lj // longer prefix first
		}
		return enabled[i].Priority < enabled[j].Priority // lower priority number first
	})
	return enabled
}

// match returns the best-matching IMSIRoute for the given IMSI string,
// or the zero value and false if no prefix matches.
func (t imsiTable) match(imsi string) (IMSIRoute, bool) {
	for _, r := range t {
		if strings.HasPrefix(imsi, r.Prefix) {
			return r, true
		}
	}
	return IMSIRoute{}, false
}

// extractIMSI attempts to extract the IMSI from a Diameter message.
// Checks in order:
//  1. Subscription-Id AVP (code 443) with Subscription-Id-Type = END_USER_IMSI (1)
//     - used by Gx, Gy, Ro
//  2. User-Name AVP (code 1) - NAI format "<IMSI>@<realm>" - used by S6a, EAP, SWx
//
// Returns the IMSI digits only (no realm suffix), or "" if not found.
func extractIMSI(msg *message.Message) string {
	// --- Subscription-Id (Grouped AVP 443) ---
	// Contains Subscription-Id-Type (450) and Subscription-Id-Data (444)
	for _, a := range msg.FindAVPs(avp.CodeSubscriptionID, 0) {
		children, err := avp.DecodeGrouped(a)
		if err != nil {
			continue
		}
		var sidType uint32
		var sidData string
		for _, child := range children {
			switch child.Code {
			case avp.CodeSubscriptionIDType:
				v, err := child.Uint32()
				if err == nil {
					sidType = v
				}
			case avp.CodeSubscriptionIDData:
				s, err := child.String()
				if err == nil {
					sidData = s
				}
			}
		}
		// END_USER_IMSI = 1
		if sidType == 1 && sidData != "" {
			return sidData
		}
	}

	// --- User-Name (AVP 1) - NAI format: <IMSI>@<realm> ---
	if a := msg.FindAVP(avp.CodeUserName, 0); a != nil {
		s, err := a.String()
		if err == nil && s != "" {
			// Strip @realm suffix if present
			if idx := strings.IndexByte(s, '@'); idx >= 0 {
				return s[:idx]
			}
			return s
		}
	}

	return ""
}
