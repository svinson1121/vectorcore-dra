package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/svinson1121/vectorcore-dra/internal/config"
	"github.com/svinson1121/vectorcore-dra/internal/router"
)

type IMSIRouteResponse struct {
	Index     int    `json:"index"`
	Prefix    string `json:"prefix"`
	DestRealm string `json:"dest_realm"`
	PeerGroup string `json:"peer_group"`
	Priority  int    `json:"priority"`
	Enabled   bool   `json:"enabled"`
}

type IMSIRouteCreateRequest struct {
	Prefix    string `json:"prefix"     required:"true"  minLength:"5" maxLength:"6"`
	DestRealm string `json:"dest_realm" required:"true"`
	PeerGroup string `json:"peer_group" required:"true"`
	Priority  int    `json:"priority"   required:"false" default:"10"`
	Enabled   bool   `json:"enabled"    required:"false" default:"true"`
}

func registerIMSIRoutes(api huma.API, s *Server) {
	huma.Register(api, huma.Operation{
		Method:      http.MethodGet,
		Path:        "/api/v1/imsi-routes",
		Summary:     "List IMSI prefix routes",
		Description: "Returns the IMSI prefix routing table (roaming/home routing).",
		Tags:        []string{"IMSI Routes"},
	}, func(ctx context.Context, input *struct{}) (*struct{ Body []IMSIRouteResponse }, error) {
		resp := make([]IMSIRouteResponse, 0, len(s.cfg.IMSIRoutes))
		for i, r := range s.cfg.IMSIRoutes {
			resp = append(resp, imsiToResponse(i, r))
		}
		return &struct{ Body []IMSIRouteResponse }{Body: resp}, nil
	})

	huma.Register(api, huma.Operation{
		Method:  http.MethodGet,
		Path:    "/api/v1/imsi-routes/{index}",
		Summary: "Get an IMSI route",
		Tags:    []string{"IMSI Routes"},
	}, func(ctx context.Context, input *struct {
		Index int `path:"index"`
	}) (*struct{ Body IMSIRouteResponse }, error) {
		if input.Index < 0 || input.Index >= len(s.cfg.IMSIRoutes) {
			return nil, huma.Error404NotFound(fmt.Sprintf("IMSI route index %d not found", input.Index))
		}
		return &struct{ Body IMSIRouteResponse }{Body: imsiToResponse(input.Index, s.cfg.IMSIRoutes[input.Index])}, nil
	})

	huma.Register(api, huma.Operation{
		Method:  http.MethodPost,
		Path:    "/api/v1/imsi-routes",
		Summary: "Create an IMSI route",
		Tags:    []string{"IMSI Routes"},
	}, func(ctx context.Context, input *struct {
		Body IMSIRouteCreateRequest
	}) (*struct{ Body IMSIRouteResponse }, error) {
		b := input.Body
		route := config.IMSIRoute{
			Prefix:    b.Prefix,
			DestRealm: b.DestRealm,
			PeerGroup: b.PeerGroup,
			Priority:  b.Priority,
			Enabled:   b.Enabled,
		}
		s.cfg.IMSIRoutes = append(s.cfg.IMSIRoutes, route)
		if err := s.saveConfig(); err != nil {
			s.cfg.IMSIRoutes = s.cfg.IMSIRoutes[:len(s.cfg.IMSIRoutes)-1]
			return nil, huma.Error500InternalServerError(err.Error())
		}
		s.router.UpdateIMSIRoutes(configIMSIToRouterIMSILocal(s.cfg.IMSIRoutes))
		idx := len(s.cfg.IMSIRoutes) - 1
		return &struct{ Body IMSIRouteResponse }{Body: imsiToResponse(idx, route)}, nil
	})

	huma.Register(api, huma.Operation{
		Method:  http.MethodPut,
		Path:    "/api/v1/imsi-routes/{index}",
		Summary: "Replace an IMSI route",
		Tags:    []string{"IMSI Routes"},
	}, func(ctx context.Context, input *struct {
		Index int `path:"index"`
		Body  IMSIRouteCreateRequest
	}) (*struct{ Body IMSIRouteResponse }, error) {
		if input.Index < 0 || input.Index >= len(s.cfg.IMSIRoutes) {
			return nil, huma.Error404NotFound(fmt.Sprintf("IMSI route index %d not found", input.Index))
		}
		b := input.Body
		route := config.IMSIRoute{
			Prefix:    b.Prefix,
			DestRealm: b.DestRealm,
			PeerGroup: b.PeerGroup,
			Priority:  b.Priority,
			Enabled:   b.Enabled,
		}
		s.cfg.IMSIRoutes[input.Index] = route
		if err := s.saveConfig(); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		s.router.UpdateIMSIRoutes(configIMSIToRouterIMSILocal(s.cfg.IMSIRoutes))
		return &struct{ Body IMSIRouteResponse }{Body: imsiToResponse(input.Index, route)}, nil
	})

	huma.Register(api, huma.Operation{
		Method:        http.MethodDelete,
		Path:          "/api/v1/imsi-routes/{index}",
		Summary:       "Delete an IMSI route",
		Tags:          []string{"IMSI Routes"},
		DefaultStatus: http.StatusNoContent,
	}, func(ctx context.Context, input *struct {
		Index int `path:"index"`
	}) (*struct{}, error) {
		if input.Index < 0 || input.Index >= len(s.cfg.IMSIRoutes) {
			return nil, huma.Error404NotFound(fmt.Sprintf("IMSI route index %d not found", input.Index))
		}
		s.cfg.IMSIRoutes = append(s.cfg.IMSIRoutes[:input.Index], s.cfg.IMSIRoutes[input.Index+1:]...)
		if err := s.saveConfig(); err != nil {
			return nil, huma.Error500InternalServerError(err.Error())
		}
		s.router.UpdateIMSIRoutes(configIMSIToRouterIMSILocal(s.cfg.IMSIRoutes))
		return nil, nil
	})
}

func imsiToResponse(i int, r config.IMSIRoute) IMSIRouteResponse {
	return IMSIRouteResponse{
		Index:     i,
		Prefix:    r.Prefix,
		DestRealm: r.DestRealm,
		PeerGroup: r.PeerGroup,
		Priority:  r.Priority,
		Enabled:   r.Enabled,
	}
}

func configIMSIToRouterIMSILocal(cfgRoutes []config.IMSIRoute) []router.IMSIRoute {
	routes := make([]router.IMSIRoute, 0, len(cfgRoutes))
	for _, r := range cfgRoutes {
		routes = append(routes, router.IMSIRoute{
			Prefix:    r.Prefix,
			DestRealm: r.DestRealm,
			PeerGroup: r.PeerGroup,
			Priority:  r.Priority,
			Enabled:   r.Enabled,
		})
	}
	return routes
}
