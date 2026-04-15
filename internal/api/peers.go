package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/svinson1121/vectorcore-dra/internal/config"
	diampeer "github.com/svinson1121/vectorcore-dra/internal/diameter/peer"
)

// knownAppIDs maps Diameter Application-ID to a short human-readable name.
var knownAppIDs = map[uint32]string{
	0:          "Common",
	1:          "NASREQ",
	2:          "MobileIPv4",
	3:          "Accounting",
	4:          "DCCA",
	16777216:   "Cx/Dx",
	16777217:   "Sh",
	16777219:   "Wx",
	16777222:   "Gq",
	16777224:   "Ro",
	16777236:   "Rx",
	16777238:   "Gx",
	16777239:   "Gy",
	16777251:   "S6a",
	16777252:   "S13",
	16777255:   "SLg",
	16777264:   "SWm",
	16777265:   "SWx",
	16777267:   "S9",
	16777268:   "S6b",
	16777272:   "S6b-PGW",
	16777291:   "SLh",
	16777312:   "S6c",
	16777313:   "SGd",
	16777221:   "Zh",
	16777302:   "PC4a",
	0xFFFFFFFF: "Relay",
}

// appIDName returns a human-readable label for the given Diameter Application-ID.
func appIDName(id uint32) string {
	if name, ok := knownAppIDs[id]; ok {
		return name
	}
	return fmt.Sprintf("%d", id)
}

// PeerResponse is the configured (static) view of a peer — data from config.yaml only.
// Live connection state is in PeerStatusResponse from GET /api/v1/peers/status.
type PeerResponse struct {
	Name      string `json:"name"`
	FQDN      string `json:"fqdn"`
	Address   string `json:"address"`
	Port      int    `json:"port"`
	Transport string `json:"transport"`
	Mode      string `json:"mode"`
	Realm     string `json:"realm"`
	LBGroup  string `json:"lb_group"`
	Weight   int    `json:"weight"`
	Enabled  bool   `json:"enabled"`
}

// PeerStatusResponse is the live connection state for a peer.
// One entry per configured peer; disabled peers show State = "DISABLED".
type PeerStatusResponse struct {
	Name                string     `json:"name"`
	FQDN                string     `json:"fqdn"`                  // configured FQDN
	State               string     `json:"state"`                 // FSM state
	ActualTransport     string     `json:"actual_transport"`      // transport in use (may differ from config)
	ConfiguredTransport string     `json:"configured_transport"`  // transport from config.yaml
	RemoteAddr          string     `json:"remote_addr,omitempty"` // actual IP:port of connection
	PeerFQDN            string     `json:"peer_fqdn,omitempty"`   // FQDN from CEA
	PeerRealm           string     `json:"peer_realm,omitempty"`  // realm from CEA
	AppIDs              []uint32   `json:"app_ids,omitempty"`
	Applications        []string   `json:"applications,omitempty"`
	InFlight            int64      `json:"in_flight"`
	ConnectedAt         *time.Time `json:"connected_at,omitempty"`
}

// PeerCreateRequest is the body for POST /api/v1/peers.
type PeerCreateRequest struct {
	Name      string `json:"name"      required:"true"`
	FQDN      string `json:"fqdn"      required:"true"`
	Address   string `json:"address"   required:"true"`
	Port      int    `json:"port"      required:"true" minimum:"1" maximum:"65535"`
	Transport string `json:"transport" required:"true" enum:"tcp,tcp+tls,sctp,sctp+tls"`
	Mode      string `json:"mode"      required:"false" enum:"active,passive" default:"active"`
	Realm     string `json:"realm"     required:"true"`
	LBGroup  string `json:"lb_group"  required:"false"`
	Weight   int    `json:"weight"    required:"false" default:"1"`
	Enabled  bool   `json:"enabled"   required:"false" default:"true"`
}

// PeerPatchRequest is the body for PATCH /api/v1/peers/{name}.
// All fields are optional; only supplied fields are changed.
// Changing address, port, transport, mode, fqdn, or realm causes the peer to restart.
type PeerPatchRequest struct {
	FQDN      *string `json:"fqdn,omitempty"`
	Address   *string `json:"address,omitempty"`
	Port      *int    `json:"port,omitempty"    minimum:"1" maximum:"65535"`
	Transport *string `json:"transport,omitempty" enum:"tcp,tcp+tls,sctp,sctp+tls"`
	Mode      *string `json:"mode,omitempty"    enum:"active,passive"`
	Realm     *string `json:"realm,omitempty"`
	LBGroup   *string `json:"lb_group,omitempty"`
	Weight    *int    `json:"weight,omitempty"`
	Enabled   *bool   `json:"enabled,omitempty"`
}

func registerPeers(api huma.API, s *Server) {
	// GET /api/v1/peers — config view (static data from config.yaml)
	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/v1/peers",
		Summary:     "List configured peers",
		Description: "Returns all configured peers (enabled and disabled). Config data only — for live connection state use GET /api/v1/peers/status.",
		Tags:        []string{"Peers"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []PeerResponse }, error) {
		resp := make([]PeerResponse, 0, len(s.cfg.Peers))
		for _, cfgPeer := range s.cfg.Peers {
			resp = append(resp, configPeerResponse(cfgPeer))
		}
		return &struct{ Body []PeerResponse }{Body: resp}, nil
	})

	// GET /api/v1/peers/status — live connection state for all configured peers
	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/v1/peers/status",
		Summary:     "Peer connection status",
		Description: "Returns live FSM state, actual transport, negotiated app IDs, and in-flight counts for all configured peers. Poll this endpoint for real-time peer health.",
		Tags:        []string{"Peers"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []PeerStatusResponse }, error) {
		resp := make([]PeerStatusResponse, 0, len(s.cfg.Peers))
		for _, cfgPeer := range s.cfg.Peers {
			if p, ok := s.mgr.Get(cfgPeer.Name); ok {
				resp = append(resp, peerToStatus(p, cfgPeer.Transport))
			} else {
				resp = append(resp, disabledPeerStatus(cfgPeer))
			}
		}
		return &struct{ Body []PeerStatusResponse }{Body: resp}, nil
	})

	// GET /api/v1/peers/{name}
	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/api/v1/peers/{name}",
		Summary: "Get a peer",
		Tags:    []string{"Peers"},
	}, func(ctx context.Context, input *struct {
		Name string `path:"name"`
	}) (*struct{ Body PeerResponse }, error) {
		for _, pc := range s.cfg.Peers {
			if pc.Name == input.Name {
				return &struct{ Body PeerResponse }{Body: configPeerResponse(pc)}, nil
			}
		}
		return nil, huma.Error404NotFound(fmt.Sprintf("peer %q not found", input.Name))
	})

	// POST /api/v1/peers
	huma.Register(api, huma.Operation{
		Method:      http.MethodPost,
		Path:        "/api/v1/peers",
		Summary:     "Add a peer",
		Description: "Adds a peer to config and triggers an immediate connect attempt (active mode).",
		Tags:        []string{"Peers"},
	}, func(ctx context.Context, input *struct {
		Body PeerCreateRequest
	}) (*struct{ Body PeerResponse }, error) {
		b := input.Body

		// Validate no duplicate name (check config, not just running peers).
		for _, pc := range s.cfg.Peers {
			if pc.Name == b.Name {
				return nil, huma.Error409Conflict(fmt.Sprintf("peer %q already exists", b.Name))
			}
		}

		mode := b.Mode
		if mode == "" {
			mode = "active"
		}
		weight := b.Weight
		if weight == 0 {
			weight = 1
		}

		cfgPeer := config.Peer{
			Name:      b.Name,
			FQDN:      b.FQDN,
			Address:   b.Address,
			Port:      b.Port,
			Transport: b.Transport,
			Mode:      mode,
			Realm:     b.Realm,
			LBGroup:   b.LBGroup,
			Weight:    weight,
			Enabled:   b.Enabled,
		}

		// Add to in-memory config
		s.cfg.Peers = append(s.cfg.Peers, cfgPeer)
		if err := s.saveConfig(); err != nil {
			// Remove from slice on save failure
			s.cfg.Peers = s.cfg.Peers[:len(s.cfg.Peers)-1]
			return nil, huma.Error500InternalServerError("failed to persist config: " + err.Error())
		}

		// Sync triggers connect for enabled peers.
		localIP := getLocalIPForAPI()
		s.mgr.Sync(s.ctx, s.cfg.Peers, s.cfg.DRA, s.cfg.Watchdog, s.cfg.Reconnect, localIP)

		return &struct{ Body PeerResponse }{Body: configPeerResponse(cfgPeer)}, nil
	})

	// PATCH /api/v1/peers/{name}
	huma.Register(api, huma.Operation{
		Method:      http.MethodPatch,
		Path:        "/api/v1/peers/{name}",
		Summary:     "Update a peer",
		Description: "Updates peer config. All fields are optional. Connection-level changes (address, port, transport, mode, fqdn, realm) trigger a reconnect.",
		Tags:        []string{"Peers"},
	}, func(ctx context.Context, input *struct {
		Name string `path:"name"`
		Body PeerPatchRequest
	}) (*struct{ Body PeerResponse }, error) {
		name := input.Name
		found := false
		for i, pc := range s.cfg.Peers {
			if pc.Name != name {
				continue
			}
			found = true
			b := input.Body
			if b.FQDN != nil {
				s.cfg.Peers[i].FQDN = *b.FQDN
			}
			if b.Address != nil {
				s.cfg.Peers[i].Address = *b.Address
			}
			if b.Port != nil {
				s.cfg.Peers[i].Port = *b.Port
			}
			if b.Transport != nil {
				s.cfg.Peers[i].Transport = *b.Transport
			}
			if b.Mode != nil {
				s.cfg.Peers[i].Mode = *b.Mode
			}
			if b.Realm != nil {
				s.cfg.Peers[i].Realm = *b.Realm
			}
			if b.LBGroup != nil {
				s.cfg.Peers[i].LBGroup = *b.LBGroup
			}
			if b.Weight != nil {
				s.cfg.Peers[i].Weight = *b.Weight
			}
			if b.Enabled != nil {
				s.cfg.Peers[i].Enabled = *b.Enabled
			}
			break
		}
		if !found {
			return nil, huma.Error404NotFound(fmt.Sprintf("peer %q not found", name))
		}
		if err := s.saveConfig(); err != nil {
			return nil, huma.Error500InternalServerError("failed to persist config: " + err.Error())
		}
		localIP := getLocalIPForAPI()
		s.mgr.Sync(s.ctx, s.cfg.Peers, s.cfg.DRA, s.cfg.Watchdog, s.cfg.Reconnect, localIP)

		for _, pc := range s.cfg.Peers {
			if pc.Name == name {
				return &struct{ Body PeerResponse }{Body: configPeerResponse(pc)}, nil
			}
		}
		return nil, huma.Error404NotFound(fmt.Sprintf("peer %q not found after update", name))
	})

	// DELETE /api/v1/peers/{name}
	huma.Register(api, huma.Operation{
		Method:        http.MethodDelete,
		Path:          "/api/v1/peers/{name}",
		Summary:       "Remove a peer",
		Description:   "Gracefully disconnects (DPR/DPA) and removes the peer from config.",
		Tags:          []string{"Peers"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		Name string `path:"name"`
	}) (*struct{}, error) {
		name := input.Name
		found := false
		filtered := s.cfg.Peers[:0]
		for _, p := range s.cfg.Peers {
			if p.Name == name {
				found = true
				continue
			}
			filtered = append(filtered, p)
		}
		if !found {
			return nil, huma.Error404NotFound(fmt.Sprintf("peer %q not found", name))
		}
		s.cfg.Peers = filtered
		if err := s.saveConfig(); err != nil {
			return nil, huma.Error500InternalServerError("failed to persist config: " + err.Error())
		}
		localIP := getLocalIPForAPI()
		s.mgr.Sync(s.ctx, s.cfg.Peers, s.cfg.DRA, s.cfg.Watchdog, s.cfg.Reconnect, localIP)
		return nil, nil
	})
}

// configPeerResponse converts a config.Peer to the static PeerResponse shape.
func configPeerResponse(c config.Peer) PeerResponse {
	mode := c.Mode
	if mode == "" {
		mode = "active"
	}
	weight := c.Weight
	if weight == 0 {
		weight = 1
	}
	return PeerResponse{
		Name:      c.Name,
		FQDN:      c.FQDN,
		Address:   c.Address,
		Port:      c.Port,
		Transport: c.Transport,
		Mode:      mode,
		Realm:     c.Realm,
		LBGroup:   c.LBGroup,
		Weight:    weight,
		Enabled:   c.Enabled,
	}
}

// peerToStatus builds a PeerStatusResponse from a live running peer.
func peerToStatus(p *diampeer.Peer, configuredTransport string) PeerStatusResponse {
	c := p.Cfg()
	var apps []string
	for _, id := range p.PeerAppIDs {
		apps = append(apps, appIDName(id))
	}
	at := p.ConnectedAt()
	var connectedAt *time.Time
	if !at.IsZero() {
		connectedAt = &at
	}
	return PeerStatusResponse{
		Name:                c.Name,
		FQDN:                c.FQDN,
		State:               p.State().String(),
		ActualTransport:     p.ActualTransport,
		ConfiguredTransport: configuredTransport,
		RemoteAddr:          p.RemoteAddr(),
		PeerFQDN:            p.PeerFQDN,
		PeerRealm:           p.PeerRealm,
		AppIDs:              p.PeerAppIDs,
		Applications:        apps,
		InFlight:            p.InFlight(),
		ConnectedAt:         connectedAt,
	}
}

// disabledPeerStatus returns a PeerStatusResponse for a configured-but-not-running peer.
func disabledPeerStatus(c config.Peer) PeerStatusResponse {
	state := "DISABLED"
	if c.Enabled {
		state = "CLOSED"
	}
	return PeerStatusResponse{
		Name:                c.Name,
		FQDN:                c.FQDN,
		State:               state,
		ActualTransport:     c.Transport,
		ConfiguredTransport: c.Transport,
	}
}
