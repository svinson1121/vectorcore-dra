# VectorCore DRA

VectorCore DRA is a Go-based Diameter Routing Agent for carrier networks. It accepts Diameter peers over TCP, TCP+TLS, and SCTP, exposes an embedded React UI plus a REST API, and routes requests without maintaining a session database.

## What It Does

- Accepts inbound and outbound Diameter peers.
- Handles the standard peer lifecycle: `CER/CEA`, `DWR/DWA`, `DPR/DPA`.
- Routes requests in this order:
  1. `Destination-Host` AVP in the message
  2. Explicit `route_rules`
  3. `imsi_routes`
  4. Implicit realm routing
- Supports load balancing with `round_robin` and `least_conn`.
- Uses `Session-Id` hashing for deterministic peer selection inside a candidate set.
- Exposes Prometheus metrics at `/metrics`.
- Serves an embedded UI at `/ui/` and OpenAPI docs at `/api/v1/docs`.

`imsi_routes` are present in the config and API but are currently a work in progress and should not be considered functional.

The DRA is stateless. It forwards requests, tracks hop-by-hop IDs for answers in flight, and does not keep a persistent Diameter session table.

## Transport Support

| Transport | Status | Notes |
|---|---|---|
| `tcp` | supported | listeners and peers |
| `tcp+tls` | supported | listeners and outbound peers share the top-level `tls` config |
| `sctp` | supported | listeners and peers |
| `sctp+tls` | not implemented | legacy configs currently fall back to plain SCTP with a warning |

## Requirements

- Go 1.25+
- Node.js 18+ for UI builds
- Linux for SCTP support

Load the SCTP kernel module if you use SCTP:

```bash
modprobe sctp
```

## Build

```bash
make all      # UI + Go binary
make ui       # build web/dist
make build    # build bin/dra
make test     # run Go tests
make clean
```

For UI development:

```bash
make dev-ui
```

The Vite dev server runs on `:5173` and proxies `/api` and `/metrics` to `:8080`.

## Run

```bash
./bin/dra -c config.yaml
./bin/dra -c config.yaml -d
./bin/dra -v
```

Flags:

| Flag | Meaning |
|---|---|
| `-c` | config path; falls back to `DRA_CONFIG`, then `config.yaml` |
| `-d` | force debug logging and mirror logs to stderr |
| `-v` | print version and exit |

Shutdown on `SIGINT`/`SIGTERM` is graceful: listeners stop accepting, peers are stopped, and the process exits after a short drain period.

## UI And API

- UI: `http://<host>:8080/ui/`
- Swagger UI: `http://<host>:8080/api/v1/docs`
- OpenAPI JSON: `http://<host>:8080/api/v1/openapi.json`
- Health: `http://<host>:8080/health`
- Prometheus: `http://<host>:8080/metrics`

The main API groups are:

- `Peers`: configured peers and live peer status
- `LB Groups`: load-balancing groups
- `Routes`: explicit routing rules
- `IMSI Routes`: IMSI-prefix routing
- `OAM`: status, reload, log level, recent messages, JSON metrics snapshot

See [docs/API.md](docs/API.md) for endpoint details and curl examples.

## Metrics

Prometheus metrics use the `dra_` prefix. Important series include:

- `dra_messages_total`
- `dra_peer_state`
- `dra_peer_connects_total`
- `dra_peer_disconnects_total`
- `dra_route_hits_total`
- `dra_route_misses_total`
- `dra_imsi_route_hits_total`
- `dra_forwarded_total`
- `dra_answer_latency_seconds`
- `dra_watchdog_timeout_total`
- `dra_reconnect_attempts_total`
- `dra_inflight_requests`
- `dra_rejected_connections_total`

## Configuration

The annotated example config lives at [config/dra.yaml](config/dra.yaml).

Top-level sections:

- `dra`
- `listeners`
- `tls`
- `api`
- `logging`
- `watchdog`
- `reconnect`
- `lb_groups`
- `peers`
- `imsi_routes`
- `route_rules`

`dra.qos` controls DSCP/TOS handling for TCP, TCP+TLS, and SCTP transports:

```yaml
dra:
  qos:
    mode: clear   # clear | set
    dscp: 0       # 0..63, used only with mode: set
```

QoS support is a work in progress. The currently usable modes are `clear` and `set`.

- `clear`: send best-effort traffic class.
- `set`: force the configured DSCP value.
- `preserve`: experimental. It does not currently preserve received SCTP DSCP because Linux SCTP one-to-one stream sockets do not expose received packet DSCP to the application.

### Mutable Sections

These sections are applied live when the config file changes on disk or when `POST /api/v1/oam/reload` is called:

- `lb_groups`
- `peers`
- `imsi_routes`
- `route_rules`

The REST API also writes those sections back to the config file atomically.

### `lb_groups`

`lb_groups` define per-group load-balancing policy.

```yaml
lb_groups:
  - name: hss_group
    lb_policy: round_robin

  - name: pcrf_group
    lb_policy: least_conn
```

Supported policies:

- `round_robin`
- `least_conn`

Weighted balancing is not implemented.

### `peers`

Example:

```yaml
peers:
  - name: hss01
    fqdn: hss01.epc.mnc435.mcc311.3gppnetwork.org
    address: 10.0.0.10
    port: 3868
    transport: tcp
    mode: active
    realm: epc.mnc435.mcc311.3gppnetwork.org
    lb_group: hss_group
    enabled: true
```

Notes:

- `mode: active` means the DRA dials the peer.
- `mode: passive` means the peer connects inbound to the DRA.
- Inbound connections are allowed only from configured peer IPs.
- `address` may be an IP or hostname; active peers are DNS-resolved before connect attempts.
- `transport` for API-managed peers is `tcp`, `tcp+tls`, or `sctp`.

### `imsi_routes`

Note: IMSI routing is a work in progress and is not currently functional.

IMSI routes are evaluated only after no explicit route rule matches. Longest prefix wins, then lower `priority` wins among equal-length prefixes.

```yaml
imsi_routes:
  - prefix: "311435"
    dest_realm: "epc.mnc435.mcc311.3gppnetwork.org"
    lb_group: hss_group
    priority: 10
```

IMSI extraction checks:

1. `Subscription-Id` with `Subscription-Id-Type = END_USER_IMSI`
2. `User-Name`, stripping any `@realm` suffix

`dest_realm` is used for routing decisions and implicit-realm fallback. The message AVP is not rewritten in place.

### `route_rules`

Route rules are matched on:

- `dest_realm`
- `app_id`

They do not match on `dest_host`. If `action: route`:

- `dest_host` means “send to this specific peer FQDN”
- `lb_group` means “select a peer from this group”
- if neither is set, the router falls back to implicit realm routing

Example:

```yaml
route_rules:
  - priority: 20
    dest_realm: epc.mnc435.mcc311.3gppnetwork.org
    app_id: 16777238
    lb_group: pcrf_group
    action: route

  - priority: 50
    dest_realm: blocked.example.com
    action: reject
```

### Effective Routing Order

1. Message `Destination-Host`
2. `route_rules`
3. `imsi_routes`
4. Implicit realm routing against open peers

## Project Layout

```text
cmd/dra/main.go
config/dra.yaml
docs/API.md
internal/api/
internal/config/
internal/diameter/
internal/logging/
internal/metrics/
internal/peermgr/
internal/router/
internal/transport/
web/
```
