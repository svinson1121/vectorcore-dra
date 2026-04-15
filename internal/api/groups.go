package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/svinson1121/vectorcore-dra/internal/config"
)

type LBGroupResponse struct {
	Name     string `json:"name"`
	LBPolicy string `json:"lb_policy"`
}

type LBGroupCreateRequest struct {
	Name     string `json:"name"      required:"true"`
	LBPolicy string `json:"lb_policy" required:"false" enum:"round_robin,least_conn" default:"round_robin"`
}

type LBGroupUpdateRequest struct {
	LBPolicy string `json:"lb_policy" required:"true" enum:"round_robin,least_conn"`
}

func registerPeerGroups(api huma.API, s *Server) {
	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/api/v1/lb-groups",
		Summary: "List lb groups",
		Tags:    []string{"LB Groups"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []LBGroupResponse }, error) {
		resp := make([]LBGroupResponse, 0, len(s.cfg.LBGroups))
		for _, g := range s.cfg.LBGroups {
			resp = append(resp, LBGroupResponse{Name: g.Name, LBPolicy: g.LBPolicy})
		}
		return &struct{ Body []LBGroupResponse }{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		Method:  http.MethodPost,
		Path:    "/api/v1/lb-groups",
		Summary: "Create an lb group",
		Tags:    []string{"LB Groups"},
	}, func(ctx context.Context, input *struct {
		Body LBGroupCreateRequest
	}) (*struct{ Body LBGroupResponse }, error) {
		b := input.Body
		for _, g := range s.cfg.LBGroups {
			if g.Name == b.Name {
				return nil, huma.Error409Conflict(fmt.Sprintf("lb group %q already exists", b.Name))
			}
		}
		policy := b.LBPolicy
		if policy == "" {
			policy = "round_robin"
		}
		s.cfg.LBGroups = append(s.cfg.LBGroups, config.LBGroup{Name: b.Name, LBPolicy: policy})
		s.router.SetGroupPolicy(b.Name, policy)
		if err := s.saveConfig(); err != nil {
			s.cfg.LBGroups = s.cfg.LBGroups[:len(s.cfg.LBGroups)-1]
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &struct{ Body LBGroupResponse }{Body: LBGroupResponse{Name: b.Name, LBPolicy: policy}}, nil
	})

	huma.Register(api, huma.Operation{
		Method:  http.MethodPatch,
		Path:    "/api/v1/lb-groups/{name}",
		Summary: "Update an lb group",
		Tags:    []string{"LB Groups"},
	}, func(ctx context.Context, input *struct {
		Name string `path:"name"`
		Body LBGroupUpdateRequest
	}) (*struct{ Body LBGroupResponse }, error) {
		found := false
		for i, g := range s.cfg.LBGroups {
			if g.Name == input.Name {
				s.cfg.LBGroups[i].LBPolicy = input.Body.LBPolicy
				found = true
				break
			}
		}
		if !found {
			return nil, huma.Error404NotFound(fmt.Sprintf("lb group %q not found", input.Name))
		}
		s.router.SetGroupPolicy(input.Name, input.Body.LBPolicy)
		if err := s.saveConfig(); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return &struct{ Body LBGroupResponse }{Body: LBGroupResponse{Name: input.Name, LBPolicy: input.Body.LBPolicy}}, nil
	})

	huma.Register(api, huma.Operation{
		Method:        http.MethodDelete,
		Path:          "/api/v1/lb-groups/{name}",
		Summary:       "Delete an lb group",
		Tags:          []string{"LB Groups"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		Name string `path:"name"`
	}) (*struct{}, error) {
		filtered := s.cfg.LBGroups[:0]
		found := false
		for _, g := range s.cfg.LBGroups {
			if g.Name == input.Name {
				found = true
				continue
			}
			filtered = append(filtered, g)
		}
		if !found {
			return nil, huma.Error404NotFound(fmt.Sprintf("lb group %q not found", input.Name))
		}
		s.cfg.LBGroups = filtered
		if err := s.saveConfig(); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		return nil, nil
	})
}
