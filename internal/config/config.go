package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure for the DRA.
type Config struct {
	DRA        DRAConfig        `yaml:"dra"`
	Listeners  []ListenerConfig `yaml:"listeners"`
	TLS        TLSConfig        `yaml:"tls"`        // shared TLS cert for all TLS listeners
	API        APIConfig        `yaml:"api"`         // HTTP management API (was "http")
	Logging    LoggingConfig    `yaml:"logging"`
	Watchdog   WatchdogConfig   `yaml:"watchdog"`
	Reconnect  ReconnectConfig  `yaml:"reconnect"`
	LBGroups   []LBGroup        `yaml:"lb_groups"`   // optional: define LB policy per group
	Peers      []Peer           `yaml:"peers"`
	IMSIRoutes []IMSIRoute      `yaml:"imsi_routes"`
	RouteRules []RouteRule      `yaml:"route_rules"`
}

// DRAConfig holds the DRA's own identity configuration.
type DRAConfig struct {
	Identity string `yaml:"identity"`
	Realm    string `yaml:"realm"`
	VendorID uint32 `yaml:"vendor_id"`
}

// ListenerConfig describes one listening socket.
type ListenerConfig struct {
	Transport string `yaml:"transport"` // tcp | tcp+tls | sctp | sctp+tls
	Address   string `yaml:"address"`   // bind address; use "0.0.0.0" or "::" for all interfaces
	Port      int    `yaml:"port"`
	// TLS cert/key/CA are NOT per-listener - set them once under the top-level tls: block.
}

// TLSConfig holds the shared TLS certificate used by all TLS listeners (tcp+tls, sctp+tls).
type TLSConfig struct {
	CertFile string `yaml:"cert"`  // PEM certificate file
	KeyFile  string `yaml:"key"`   // PEM private key file
	CAFile   string `yaml:"ca"`    // PEM CA bundle for peer verification; "" = system roots
}

// APIConfig holds the HTTP management API server configuration.
type APIConfig struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

// LoggingConfig controls log output.
type LoggingConfig struct {
	Level string `yaml:"level"` // debug | info | warn | error
	File  string `yaml:"file"`  // path to log file; relative or absolute
}

// WatchdogConfig holds watchdog timer settings.
type WatchdogConfig struct {
	IntervalSeconds int `yaml:"interval_seconds"`
	MaxFailures     int `yaml:"max_failures"`
}

// ReconnectConfig holds reconnect backoff settings.
type ReconnectConfig struct {
	InitialBackoffSeconds int `yaml:"initial_backoff_seconds"`
	MaxBackoffSeconds     int `yaml:"max_backoff_seconds"`
}

// LBGroup defines a named group of peers with a shared load-balancing policy.
type LBGroup struct {
	Name     string `yaml:"name"`
	LBPolicy string `yaml:"lb_policy"` // round_robin | weighted | least_conn
}

// Peer describes a remote Diameter peer.
type Peer struct {
	Name      string `yaml:"name"`
	FQDN      string `yaml:"fqdn"`
	Address   string `yaml:"address"`  // IP or FQDN; resolved via DNS if not an IP
	Port      int    `yaml:"port"`
	Transport string `yaml:"transport"` // tcp | tcp+tls | sctp | sctp+tls
	Mode      string `yaml:"mode"`      // active (we connect) | passive (we wait)
	Realm     string `yaml:"realm"`
	LBGroup   string `yaml:"lb_group"`   // name of the LBGroup this peer belongs to
	Weight    int    `yaml:"weight"`
	Enabled   bool   `yaml:"enabled"`
}

// IMSIRoute maps an IMSI MCC+MNC prefix to a destination realm and peer group.
// Evaluated after explicit route_rules. Longest prefix wins.
type IMSIRoute struct {
	Prefix    string `yaml:"prefix"`     // e.g. "311435" (MCC=311 MNC=435)
	DestRealm string `yaml:"dest_realm"` // e.g. "epc.mnc435.mcc311.3gppnetwork.org"
	LBGroup   string `yaml:"lb_group"`
	Priority  int    `yaml:"priority"`
	// Enabled defaults to true when omitted from config. Set to false to disable.
	Enabled *bool `yaml:"enabled"`
}

// RouteRule describes a single routing rule.
type RouteRule struct {
	Priority  int    `yaml:"priority"`
	DestHost  string `yaml:"dest_host,omitempty"` // "" = wildcard; matched against Destination-Host AVP
	DestRealm string `yaml:"dest_realm"`          // "" = catch-all default route
	AppID     uint32 `yaml:"app_id"`              // 0 = any
	LBGroup   string `yaml:"lb_group,omitempty"`  // lb group to route to; omitted = auto-select
	Action    string `yaml:"action"`                // route | reject | drop
	// Enabled defaults to true when omitted from config. Set to false to disable.
	Enabled *bool `yaml:"enabled"`
}

// Load reads and parses a YAML config file at path.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("config: opening %q: %w", path, err)
	}
	defer f.Close()

	cfg := Default()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(false)
	if err := dec.Decode(cfg); err != nil {
		return nil, fmt.Errorf("config: parsing %q: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to path atomically (write to path+".tmp", then rename).
func Save(path string, cfg *Config) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("config: creating temp file %q: %w", tmp, err)
	}

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("config: encoding YAML: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("config: closing temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("config: renaming temp file to %q: %w", path, err)
	}
	return nil
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		DRA: DRAConfig{
			Identity: "dra.epc.example.com",
			Realm:    "epc.example.com",
		},
		Listeners: []ListenerConfig{
			{Transport: "tcp", Address: "0.0.0.0", Port: 3868},
		},
		API: APIConfig{
			Address: "0.0.0.0",
			Port:    8080,
		},
		Logging: LoggingConfig{
			Level: "info",
			File:  "logs/dra.log",
		},
		Watchdog: WatchdogConfig{
			IntervalSeconds: 30,
			MaxFailures:     3,
		},
		Reconnect: ReconnectConfig{
			InitialBackoffSeconds: 1,
			MaxBackoffSeconds:     60,
		},
	}
}
