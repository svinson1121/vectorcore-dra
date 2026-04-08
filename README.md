# VectorCore DRA

A  **Diameter Routing Agent (DRA)** written in Go, per RFC 6733, RFC 7075, and 3GPP TS 29.215. for carrier telecom networks.

---

## What It Does

- Accepts inbound Diameter connections from network peers (MME, HSS, PCRF, P-CSCF, EIR, etc.)
- Routes Diameter requests based on dynamic Application-ID matching or static priority-ordered rules
- IMSI-prefix routing for roaming / home-network separation (3GPP TS 29.215, GSMA IR.88)
- Add and remove peers live without restart - no service interruption
- Manages full peer lifecycle: CER/CEA, DWR/DWA, DPR/DPA across all transports
- Embedded React web UI for real-time monitoring and configuration
- REST management API with OpenAPI docs
- Exports Diameter-specific Prometheus metrics

The DRA is a **stateless relay/routing agent** - it does not terminate Diameter sessions. Session affinity for mid-session messages (e.g. CCR-U) is handled via Session-ID hash -> peer mapping without maintaining a session table.

---

## Transport Support

| Transport   | Default Port | Notes                                      |
|-------------|--------------|--------------------------------------------|
| TCP         | 3868         | RFC 6733 standard                          |
| TCP + TLS   | 5868         | RFC 6733 s4.3, inband TLS via CER          |
| SCTP        | 3868         | RFC 6733 s2.1, multi-stream                |
| SCTP + TLS  | 5868         | RFC 3436 / DTLS                            |

All configured listeners start **concurrently** at startup.

---

## Requirements

- **Go** 1.21 or later
- **Node.js** 18 or later (for UI build)
- **Linux** - SCTP transport requires the kernel `sctp` module:
  ```
  modprobe sctp
  ```
- No external database. Configuration is a single YAML file.

---

## Build

```bash
# Build everything (UI + Go binary)
make all

# Or step by step:
make ui       # compile React UI into web/dist/
make build    # compile Go binary (embeds web/dist/) -> bin/dra

# Run tests
make test

# Clean build artifacts
make clean
```

The `make all` target is equivalent to `make ui && make build`. The UI must be built before the Go binary - `make build` embeds the compiled UI at compile time via `//go:embed`.

### Development UI (Vite dev server)

For UI development with hot reload, run the Go binary first then:

```bash
make dev-ui   # starts Vite on :5173, proxies /api and /metrics to :8080
```

---

## Running

```bash
# Normal operation (logs to file only)
./bin/dra -c config.yaml

# Debug mode (logs to file + console, forces debug level)
./bin/dra -c config.yaml -d
```

### Flags

| Flag      | Default       | Description                                   |
|-----------|---------------|-----------------------------------------------|
| `-c` | `config.yaml` | Path to configuration file                    |
| `-d`      | off           | Debug mode: debug log level, also log to stderr |

On **SIGTERM** or **SIGINT** the DRA performs a graceful shutdown: stops accepting new connections, sends DPR to all open peers, drains in-flight messages, then exits.

---

## Web UI

Once running, the embedded web UI is available at:

```
http://<host>:8080/ui/
```

### Pages

| Page        | Description                                                              |
|-------------|--------------------------------------------------------------------------|
| **Dashboard**  | Peer summary table, real-time message rate chart (live + 1m average), uptime and total message count |
| **Peers**      | Full peer table with live FSM state, transport, in-flight counts; add/edit/delete/enable peers |
| **Routing**    | Route rules (priority-ordered), IMSI prefix routes, peer groups         |
| **Metrics**    | Recent Diameter messages, Prometheus charts (latency, peer state, reconnects) |
| **Config/OAM** | DRA identity, listener config, log level control                        |

All data polls every **5 seconds** via the REST API.

---

## API & OpenAPI Docs

The interactive API documentation (Swagger UI) is served at:

```
http://<host>:8080/api/v1/docs
```

The raw OpenAPI 3.1 schema is at:

```
http://<host>:8080/api/v1/openapi.json
```

For full endpoint reference with curl examples, see **[docs/API.md](docs/API.md)**.

---

## Prometheus Metrics

Prometheus metrics are available in standard exposition format at:

```
http://<host>:8080/metrics
```

All DRA metrics are prefixed `dra_`. Key metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `dra_messages_total` | Counter | Messages processed, labelled by peer/direction/app_id/command_code |
| `dra_peer_state` | Gauge | FSM state per peer (0=CLOSED, 1=CONNECTING, 2=OPEN, 3=DRAINING) |
| `dra_answer_latency_seconds` | Histogram | Request-to-answer latency per peer and command |
| `dra_forwarded_total` | Counter | Successfully forwarded messages |
| `dra_route_misses_total` | Counter | Routing failures by reason |
| `dra_reconnect_attempts_total` | Counter | Reconnect attempts per active peer |
| `dra_watchdog_timeout_total` | Counter | DWR watchdog timeouts per peer |
| `dra_inflight_requests` | Gauge | In-flight requests per peer |
| `dra_rejected_connections_total` | Counter | Connections rejected (unknown source IP) |

---

## Configuration File

The DRA reads a single YAML file (default `config.yaml`). The file is **hot-reloaded** - changes to peers, IMSI routes, and route rules are applied live without restart. The REST API also writes to this file atomically.

A full annotated example is in [`config.example.yaml`](config.example.yaml). The sections are:

---

### `dra` - Identity

```yaml
dra:
  identity: "dra.epc.mnc435.mcc311.3gppnetwork.org"   # Diameter Identity (Origin-Host)
  realm:    "epc.mnc435.mcc311.3gppnetwork.org"        # Diameter Realm (Origin-Realm)
```

The `identity` value is sent as `Origin-Host` in all CER/CEA and DWR/DWA messages. The `realm` is sent as `Origin-Realm`.

---

### `listeners` - Diameter Transports

```yaml
listeners:
  - transport: tcp
    address: "0.0.0.0"
    port: 3868

  - transport: sctp
    address: "0.0.0.0"
    port: 3868

  - transport: tcp+tls
    address: "0.0.0.0"
    port: 5868

  - transport: sctp+tls
    address: "0.0.0.0"
    port: 5868
```

All listeners start concurrently. Use `"::"` for IPv4+IPv6 dual-stack. TLS listeners share the certificate configured in the `tls` section.

**Security**: only source IPs listed in the `peers` section are accepted. Connections from unknown addresses are dropped at accept before any Diameter processing.

---

### `tls` - TLS Certificate (shared by all TLS listeners)

```yaml
tls:
  cert: /etc/vectorcore-dra/server.crt
  key:  /etc/vectorcore-dra/server.key
  ca:   /etc/vectorcore-dra/ca.crt     # omit to use system roots
```

---

### `api` - Management API

```yaml
api:
  address: "0.0.0.0"
  port: 8080
```

The REST API and embedded web UI are served from this address. Scope this to a management interface in production.

---

### `logging`

```yaml
logging:
  level: info          # debug | info | warn | error
  file: logs/dra.log
```

Normal operation logs to the file only. The `-d` flag at startup overrides this: forces `debug` level and also writes to stderr. Runtime log level can be changed via the API without restart (does not persist across restarts).

---

### `watchdog` - DWR/DWA (RFC 6733 s5.5)

```yaml
watchdog:
  interval_seconds: 30   # how often to send DWR
  max_failures: 3        # consecutive DWA failures before declaring peer down
```

---

### `reconnect` - Backoff for Active Peers

```yaml
reconnect:
  initial_backoff_seconds: 1
  max_backoff_seconds: 60
```

Exponential backoff: 1s -> 2s -> 4s -> ... -> 60s. Resets on successful CER/CEA. Only applies to `mode: active` peers.

---

### `peer_groups` - Load Balancing Groups

```yaml
peer_groups:
  - name: hss_group
    lb_policy: round_robin    # round_robin | weighted | least_conn

  - name: pcrf_group
    lb_policy: least_conn
```

Groups referenced in `peers` or `route_rules` but not listed here default to `round_robin`. **Hot-reloaded.**

---

### `peers` - Diameter Peer Definitions

```yaml
peers:
  - name: hss01
    fqdn: hss01.epc.mnc435.mcc311.3gppnetwork.org   # Diameter Identity of the peer
    address: 10.0.0.10    # IP or FQDN; FQDNs are DNS-resolved on each connect attempt
    port: 3868
    transport: tcp        # tcp | tcp+tls | sctp | sctp+tls
    mode: active          # active = DRA connects | passive = peer connects to us
    realm: epc.mnc435.mcc311.3gppnetwork.org
    peer_group: hss_group
    weight: 1             # used with lb_policy: weighted
    enabled: true
```

**`mode: active`** - DRA initiates the TCP/SCTP connection, sends CER, and reconnects automatically on failure using exponential backoff. DNS is re-resolved on each reconnect attempt.

**`mode: passive`** - DRA listens for an inbound connection from the peer. No reconnect logic - the peer must re-initiate after disconnect. The source IP of the inbound connection must match the configured `address`.

**Hot-reloaded.** Adding a peer triggers an immediate connect (active) or registers the address for inbound matching (passive). Removing a peer sends DPR/DPA before closing.

---

### `imsi_routes` - IMSI Prefix Routing

Evaluated **before** `route_rules`. Longest prefix wins (analogous to BGP longest-match).

```yaml
imsi_routes:
  - prefix: "311435"              # MCC=311 MNC=435 (home network)
    dest_realm: "epc.mnc435.mcc311.3gppnetwork.org"
    peer_group: hss_group
    priority: 10
    enabled: true

  - prefix: "310260"              # T-Mobile US (roaming partner)
    dest_realm: "epc.mnc260.mcc310.3gppnetwork.org"
    peer_group: roaming_ipe
    priority: 10
    enabled: true
```

**Prefix format**: MCC+MNC digits - 5 digits for 2-digit MNC, 6 digits for 3-digit MNC.

**IMSI extraction**: checks `Subscription-Id` AVP with `Subscription-Id-Type=END_USER_IMSI` (Gx/Gy), then `User-Name` AVP in NAI format (S6a/SWx).

On match, the DRA overwrites the `Destination-Realm` AVP with `dest_realm` and routes to `peer_group`. **Hot-reloaded.**

---

### `route_rules` - Explicit Route Rules

Evaluated after IMSI routes. Rules are sorted by `priority` (lower = first). First match wins.

```yaml
route_rules:
  - priority: 20
    dest_realm: epc.mnc435.mcc311.3gppnetwork.org
    app_id: 16777238          # Gx = 16777238; 0 = any application
    peer_group: pcrf_group
    action: route
    enabled: true

  - priority: 50
    dest_realm: blocked.example.com
    action: reject
    enabled: true

  - priority: 100
    action: reject            # catch-all: reject anything unmatched
    enabled: true
```

| Field        | Description |
|--------------|-------------|
| `priority`   | Lower evaluated first |
| `dest_realm` | Diameter realm to match; `""` = wildcard |
| `dest_host`  | Diameter identity to match; `""` = wildcard |
| `app_id`     | Application-ID to match; `0` = any |
| `peer_group` | Route to this peer group (`action: route` only) |
| `peer`       | Route to a specific peer by name (overrides peer_group) |
| `action`     | `route` \| `reject` \| `drop` |
| `enabled`    | `false` disables the rule without removing it |

Route rules are **optional**. Without any rules the DRA routes `Destination-Realm` to any OPEN peer whose realm matches - the same default behaviour as freeDiameter. Add rules only to override routing for specific applications or to add explicit reject/drop actions. **Hot-reloaded.**

---

### Routing Priority (full order)

1. **IMSI prefix** - match IMSI from message -> rewrite `Destination-Realm` -> route
2. **Explicit host** - `Destination-Host` matches a peer FQDN exactly
3. **Realm + App-ID** - `Destination-Realm` + `Application-ID` match a route rule
4. **Realm only** - `Destination-Realm` matches, any App-ID
5. **Default route** - catch-all rule with empty `dest_realm`

If no route is found or all peers in the target group are down, the DRA returns `UNABLE_TO_DELIVER (3002)` to the originator.

---

## Common Diameter Application IDs

| Application | ID         |
|-------------|------------|
| Gx (PCEF-PCRF) | 16777238 |
| S6a (MME-HSS)  | 16777251 |
| Rx (P-CSCF)    | 16777236 |
| Cx/Dx (IMS)    | 16777216 |
| S13 (EIR)      | 16777252 |
| SWx (WiFi)     | 16777265 |
| Gy/Ro          | 16777224 |
| Relay          | 4294967295 |

---

