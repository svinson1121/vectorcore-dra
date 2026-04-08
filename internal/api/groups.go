package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/svinson1121/vectorcore-dra/internal/config"
)

type PeerGroupResponse struct {
	Name     string `json:"name"`
	LBPolicy string `json:"lb_policy"`
}

type PeerGroupCreateRequest struct {
	Name     string `json:"name"      required:"true"`
	LBPolicy string `json:"lb_policy" required:"false" enum:"round_robin,weighted,least_conn" default:"round_robin"`
}

func registerPeerGroups(api huma.API, s *Server) {
	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/api/v1/peer-groups",
		Summary: "List peer groups",
		Tags:    []string{"Peer Groups"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []PeerGroupResponse }, error) {
		resp := make([]PeerGroupResponse, 0, len(s.cfg.PeerGroups))
		for _, g := range s.cfg.PeerGroups {
			resp = append(resp, PeerGroupResponse{Name: g.Name, LBPolicy: g.LBPolicy})
		}
		return &struct{ Body []PeerGroupResponse }{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		Method:  http.MethodPost,
		Path:    "/api/v1/peer-groups",
		Summary: "Create a peer group",
		Tags:    []string{"Peer Groups"},
	}, func(ctx context.Context, input *struct {
		Body PeerGroupCreateRequest
	}) (*struct{ Body PeerGroupResponse }, error) {
		b := input.Body
		for _, g := range s.cfg.PeerGroups {
			if g.Name == b.Name {
				return nil, huma.Error409Conflict(fmt.Sprintf("peer group %q already exists", b.Name))
			}
		}
		policy := b.LBPolicy
		if policy == "" {
			policy = "round_robin"
		}
		s.cfg.PeerGroups = append(s.cfg.PeerGroups, config.PeerGroup{Name: b.Name, LBPolicy: policy})
		s.router.SetGroupPolicy(b.Name, policy)
		if err := s.saveConfig(); err != nil {
			s.cfg.PeerGroups = s.cfg.PeerGroups[:len(s.cfg.PeerGroups)-1]
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &struct{ Body PeerGroupResponse }{Body: PeerGroupResponse{Name: b.Name, LBPolicy: policy}}, nil
	})

	huma.Register(api, huma.Operation{
		Method:        http.MethodDelete,
		Path:          "/api/v1/peer-groups/{name}",
		Summary:       "Delete a peer group",
		Tags:          []string{"Peer Groups"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		Name string `path:"name"`
	}) (*struct{}, error) {
		filtered := s.cfg.PeerGroups[:0]
		found := false
		for _, g := range s.cfg.PeerGroups {
			if g.Name == input.Name {
				found = true
				continue
			}
			filtered = append(filtered, g)
		}
		if !found {
			return nil, huma.Error404NotFound(fmt.Sprintf("peer group %q not found", input.Name))
		}
		s.cfg.PeerGroups = filtered
		if err := s.saveConfig(); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return nil, nil
	})
}
