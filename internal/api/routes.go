package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/svinson1121/vectorcore-dra/internal/config"
	"github.com/svinson1121/vectorcore-dra/internal/router"
)

type RouteRuleResponse struct {
	Index     int    `json:"index"`
	Priority  int    `json:"priority"`
	DestHost  string `json:"dest_host"`
	DestRealm string `json:"dest_realm"`
	AppID     uint32 `json:"app_id"`
	PeerGroup string `json:"peer_group"`
	Peer      string `json:"peer"`
	Action    string `json:"action"`
	Enabled   bool   `json:"enabled"`
}

type RouteRuleCreateRequest struct {
	Priority  int    `json:"priority"   required:"true"`
	DestHost  string `json:"dest_host"  required:"false"`
	DestRealm string `json:"dest_realm" required:"false"`
	AppID     uint32 `json:"app_id"     required:"false"`
	PeerGroup string `json:"peer_group" required:"false"`
	Peer      string `json:"peer"       required:"false"`
	Action    string `json:"action"     required:"true" enum:"route,reject,drop"`
	Enabled   bool   `json:"enabled"    required:"false" default:"true"`
}

func registerRoutes(api huma.API, s *Server) {
	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/api/v1/routes",
		Summary: "List route rules",
		Tags:    []string{"Routes"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []RouteRuleResponse }, error) {
		resp := make([]RouteRuleResponse, 0, len(s.cfg.RouteRules))
		for i, r := range s.cfg.RouteRules {
			resp = append(resp, routeToResponse(i, r))
		}
		return &struct{ Body []RouteRuleResponse }{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/api/v1/routes/{index}",
		Summary: "Get a route rule",
		Tags:    []string{"Routes"},
	}, func(ctx context.Context, input *struct {
		Index int `path:"index"`
	}) (*struct{ Body RouteRuleResponse }, error) {
		if input.Index < 0 || input.Index >= len(s.cfg.RouteRules) {
			return nil, huma.Error404NotFound(fmt.Sprintf("route index %d not found", input.Index))
		}
		return &struct{ Body RouteRuleResponse }{Body: routeToResponse(input.Index, s.cfg.RouteRules[input.Index])}, nil
	})

	huma.Register(api, huma.Operation{
		Method:  http.MethodPost,
		Path:    "/api/v1/routes",
		Summary: "Create a route rule",
		Tags:    []string{"Routes"},
	}, func(ctx context.Context, input *struct {
		Body RouteRuleCreateRequest
	}) (*struct{ Body RouteRuleResponse }, error) {
		b := input.Body
		rule := config.RouteRule{
			Priority:  b.Priority,
			DestHost:  b.DestHost,
			DestRealm: b.DestRealm,
			AppID:     b.AppID,
			PeerGroup: b.PeerGroup,
			Peer:      b.Peer,
			Action:    b.Action,
			Enabled:   b.Enabled,
		}
		s.cfg.RouteRules = append(s.cfg.RouteRules, rule)
		if err := s.saveConfig(); err != nil {
			s.cfg.RouteRules = s.cfg.RouteRules[:len(s.cfg.RouteRules)-1]
			return nil, huma.Error500InternalServerError(err.Error())
		}
		s.router.UpdateRules(configRulesToRouterRulesLocal(s.cfg.RouteRules))
		idx := len(s.cfg.RouteRules) - 1
		return &struct{ Body RouteRuleResponse }{Body: routeToResponse(idx, rule)}, nil
	})

	huma.Register(api, huma.Operation{
		Method:  http.MethodPut,
		Path:    "/api/v1/routes/{index}",
		Summary: "Replace a route rule",
		Tags:    []string{"Routes"},
	}, func(ctx context.Context, input *struct {
		Index int `path:"index"`
		Body  RouteRuleCreateRequest
	}) (*struct{ Body RouteRuleResponse }, error) {
		if input.Index < 0 || input.Index >= len(s.cfg.RouteRules) {
			return nil, huma.Error404NotFound(fmt.Sprintf("route index %d not found", input.Index))
		}
		b := input.Body
		rule := config.RouteRule{
			Priority:  b.Priority,
			DestHost:  b.DestHost,
			DestRealm: b.DestRealm,
			AppID:     b.AppID,
			PeerGroup: b.PeerGroup,
			Peer:      b.Peer,
			Action:    b.Action,
			Enabled:   b.Enabled,
		}
		s.cfg.RouteRules[input.Index] = rule
		if err := s.saveConfig(); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		s.router.UpdateRules(configRulesToRouterRulesLocal(s.cfg.RouteRules))
		return &struct{ Body RouteRuleResponse }{Body: routeToResponse(input.Index, rule)}, nil
	})

	huma.Register(api, huma.Operation{
		Method:        http.MethodDelete,
		Path:          "/api/v1/routes/{index}",
		Summary:       "Delete a route rule",
		Tags:          []string{"Routes"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		Index int `path:"index"`
	}) (*struct{}, error) {
		if input.Index < 0 || input.Index >= len(s.cfg.RouteRules) {
			return nil, huma.Error404NotFound(fmt.Sprintf("route index %d not found", input.Index))
		}
		s.cfg.RouteRules = append(s.cfg.RouteRules[:input.Index], s.cfg.RouteRules[input.Index+1:]...)
		if err := s.saveConfig(); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		s.router.UpdateRules(configRulesToRouterRulesLocal(s.cfg.RouteRules))
		return nil, nil
	})
}

func routeToResponse(i int, r config.RouteRule) RouteRuleResponse {
	return RouteRuleResponse{
		Index:     i,
		Priority:  r.Priority,
		DestHost:  r.DestHost,
		DestRealm: r.DestRealm,
		AppID:     r.AppID,
		PeerGroup: r.PeerGroup,
		Peer:      r.Peer,
		Action:    r.Action,
		Enabled:   r.Enabled,
	}
}

func configRulesToRouterRulesLocal(cfgRules []config.RouteRule) []router.Rule {
	rules := make([]router.Rule, 0, len(cfgRules))
	for _, r := range cfgRules {
		rules = append(rules, router.Rule{
			Priority:  r.Priority,
			DestHost:  r.DestHost,
			DestRealm: r.DestRealm,
			AppID:     r.AppID,
			PeerGroup: r.PeerGroup,
			Peer:      r.Peer,
			Action:    r.Action,
			Enabled:   r.Enabled,
		})
	}
	return rules
}
