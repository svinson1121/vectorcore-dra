# VectorCore DRA - REST API Reference

Base URL: `http://<host>:8080/api/v1`

Interactive Swagger UI: `http://<host>:8080/api/v1/docs`
OpenAPI 3.1 schema: `http://<host>:8080/api/v1/openapi.json`

All request and response bodies are JSON. Successful responses that return data use HTTP 200. Successful deletes and reloads return HTTP 204 (no body). Errors return a JSON body with a `detail` field describing the problem.

---

## Table of Contents

- [Peers](#peers)
- [Peer Status](#peer-status)
- [Peer Groups](#peer-groups)
- [Route Rules](#route-rules)
- [IMSI Routes](#imsi-routes)
- [OAM](#oam)
- [Observability](#observability)

---

## Peers

Configuration-level peer management. These endpoints read and write `config.yaml`. For live connection state use [Peer Status](#peer-status).

---

### `GET /api/v1/peers` - List configured peers

Returns all configured peers (enabled and disabled). Config data only.

```bash
curl http://localhost:8080/api/v1/peers
```

**Response `200`**
```json
[
  {
    "name": "hss01",
    "fqdn": "hss01.epc.mnc435.mcc311.3gppnetwork.org",
    "address": "10.0.0.10",
    "port": 3868,
    "transport": "tcp",
    "mode": "active",
    "realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "peer_group": "hss_group",
    "weight": 1,
    "enabled": true
  }
]
```

---

### `GET /api/v1/peers/{name}` - Get a single peer

```bash
curl http://localhost:8080/api/v1/peers/hss01
```

**Response `200`** - same shape as one element from the list above.

**Response `404`**
```json
{ "detail": "peer \"hss01\" not found" }
```

---

### `POST /api/v1/peers` - Add a peer

Adds the peer to config and triggers an immediate connect attempt (active mode).

**Required fields**: `name`, `fqdn`, `address`, `port`, `transport`, `realm`

```bash
curl -X POST http://localhost:8080/api/v1/peers \
  -H 'Content-Type: application/json' \
  -d '{
    "name":       "pcrf01",
    "fqdn":       "pcrf01.epc.mnc435.mcc311.3gppnetwork.org",
    "address":    "10.0.1.10",
    "port":       3868,
    "transport":  "sctp",
    "mode":       "active",
    "realm":      "epc.mnc435.mcc311.3gppnetwork.org",
    "peer_group": "pcrf_group",
    "weight":     1,
    "enabled":    true
  }'
```

**Fields**

| Field        | Type    | Required | Values                              | Default    |
|--------------|---------|----------|-------------------------------------|------------|
| `name`       | string  | yes      | unique identifier                   | -          |
| `fqdn`       | string  | yes      | Diameter Identity of the peer       | -          |
| `address`    | string  | yes      | IP address or FQDN                  | -          |
| `port`       | integer | yes      | 1-65535                             | -          |
| `transport`  | string  | yes      | `tcp` `tcp+tls` `sctp` `sctp+tls`  | -          |
| `mode`       | string  | no       | `active` `passive`                  | `active`   |
| `realm`      | string  | yes      | Diameter Realm                      | -          |
| `peer_group` | string  | no       | name of a peer group                | `""`       |
| `weight`     | integer | no       | used with `lb_policy: weighted`     | `1`        |
| `enabled`    | boolean | no       |                                     | `true`     |

**Response `200`** - the created peer config object.

**Response `409`** - peer name already exists.

---

### `PATCH /api/v1/peers/{name}` - Update a peer

All fields are optional. Only supplied fields are changed. Connection-level changes (`address`, `port`, `transport`, `mode`, `fqdn`, `realm`) trigger a graceful reconnect.

```bash
# Disable a peer
curl -X PATCH http://localhost:8080/api/v1/peers/pcrf01 \
  -H 'Content-Type: application/json' \
  -d '{ "enabled": false }'

# Move peer to a different group and change weight
curl -X PATCH http://localhost:8080/api/v1/peers/hss02 \
  -H 'Content-Type: application/json' \
  -d '{ "peer_group": "hss_group_2", "weight": 5 }'

# Change address and port (triggers reconnect)
curl -X PATCH http://localhost:8080/api/v1/peers/hss01 \
  -H 'Content-Type: application/json' \
  -d '{ "address": "10.0.0.20", "port": 3869 }'
```

**Response `200`** - updated peer config object.

**Response `404`** - peer not found.

---

### `DELETE /api/v1/peers/{name}` - Remove a peer

Gracefully disconnects (sends DPR, waits for DPA or 5s timeout), removes from config, and saves.

```bash
curl -X DELETE http://localhost:8080/api/v1/peers/pcrf01
```

**Response `204`** - no body.

**Response `404`** - peer not found.

---

## Peer Status

Live connection state for all configured peers. Poll this endpoint for real-time peer health. Separate from the config endpoint because a peer may connect on a different transport than configured (e.g. configured `sctp` but connected inbound via `tcp`).

---

### `GET /api/v1/peers/status` - All peer connection states

```bash
curl http://localhost:8080/api/v1/peers/status
```

**Response `200`**
```json
[
  {
    "name":                 "hss01",
    "fqdn":                 "hss01.epc.mnc435.mcc311.3gppnetwork.org",
    "state":                "OPEN",
    "actual_transport":     "tcp",
    "configured_transport": "tcp",
    "remote_addr":          "10.0.0.10:3868",
    "peer_fqdn":            "hss01.epc.mnc435.mcc311.3gppnetwork.org",
    "peer_realm":           "epc.mnc435.mcc311.3gppnetwork.org",
    "app_ids":              [16777251],
    "applications":         ["S6a"],
    "in_flight":            3,
    "connected_at":         "2026-03-23T10:15:00Z"
  },
  {
    "name":                 "pcrf01",
    "fqdn":                 "pcrf01.epc.mnc435.mcc311.3gppnetwork.org",
    "state":                "DISABLED",
    "actual_transport":     "sctp",
    "configured_transport": "sctp",
    "in_flight":            0
  }
]
```

**State values**

| State        | Meaning                                                  |
|--------------|----------------------------------------------------------|
| `OPEN`       | CER/CEA complete, peer is healthy and ready for routing  |
| `CONNECTING` | TCP/SCTP connect in progress or CER/CEA exchange pending |
| `DRAINING`   | DPR sent, waiting for DPA before closing                 |
| `CLOSED`     | Enabled but not connected (reconnect pending)            |
| `DISABLED`   | `enabled: false` in config                               |

---

## Peer Groups

Peer groups define load-balancing policy for a set of peers.

---

### `GET /api/v1/peer-groups` - List peer groups

```bash
curl http://localhost:8080/api/v1/peer-groups
```

**Response `200`**
```json
[
  { "name": "hss_group",  "lb_policy": "round_robin" },
  { "name": "pcrf_group", "lb_policy": "least_conn"  }
]
```

---

### `POST /api/v1/peer-groups` - Create a peer group

```bash
curl -X POST http://localhost:8080/api/v1/peer-groups \
  -H 'Content-Type: application/json' \
  -d '{ "name": "roaming_ipe", "lb_policy": "weighted" }'
```

**Fields**

| Field       | Type   | Required | Values                                   | Default        |
|-------------|--------|----------|------------------------------------------|----------------|
| `name`      | string | yes      | unique group name                        | -              |
| `lb_policy` | string | no       | `round_robin` `weighted` `least_conn`    | `round_robin`  |

**lb_policy values**

| Policy        | Description                                                      |
|---------------|------------------------------------------------------------------|
| `round_robin` | Cycle through OPEN peers in order                                |
| `weighted`    | Select peer weighted by the `weight` field on each peer config   |
| `least_conn`  | Route to the peer with fewest in-flight requests                 |

**Response `200`** - created group.

**Response `409`** - group name already exists.

---

### `DELETE /api/v1/peer-groups/{name}` - Delete a peer group

```bash
curl -X DELETE http://localhost:8080/api/v1/peer-groups/roaming_ipe
```

**Response `204`** - no body.

**Response `404`** - group not found.

---

## Route Rules

Explicit routing rules evaluated after IMSI routes. Sorted by `priority` (lower = first). First match wins.

Route rules are **optional** - without any rules the DRA routes `Destination-Realm` to any OPEN peer whose realm matches. Add rules to override routing per application or to add explicit reject/drop actions.

Rules are identified by zero-based **index** in the ordered list.

---

### `GET /api/v1/routes` - List route rules

```bash
curl http://localhost:8080/api/v1/routes
```

**Response `200`**
```json
[
  {
    "index":      0,
    "priority":   20,
    "dest_host":  "",
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "app_id":     16777238,
    "peer_group": "pcrf_group",
    "peer":       "",
    "action":     "route",
    "enabled":    true
  },
  {
    "index":      1,
    "priority":   100,
    "dest_host":  "",
    "dest_realm": "",
    "app_id":     0,
    "peer_group": "",
    "peer":       "",
    "action":     "reject",
    "enabled":    true
  }
]
```

---

### `GET /api/v1/routes/{index}` - Get a route rule

```bash
curl http://localhost:8080/api/v1/routes/0
```

**Response `200`** - single rule object.

**Response `404`** - index out of range.

---

### `POST /api/v1/routes` - Create a route rule

The rule is appended to the list. Reorder by updating the `priority` fields.

```bash
# Route Gx traffic to the PCRF group
curl -X POST http://localhost:8080/api/v1/routes \
  -H 'Content-Type: application/json' \
  -d '{
    "priority":   20,
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "app_id":     16777238,
    "peer_group": "pcrf_group",
    "action":     "route",
    "enabled":    true
  }'

# Reject traffic to a specific realm
curl -X POST http://localhost:8080/api/v1/routes \
  -H 'Content-Type: application/json' \
  -d '{
    "priority":   50,
    "dest_realm": "blocked.example.com",
    "action":     "reject",
    "enabled":    true
  }'

# Catch-all reject for anything unmatched
curl -X POST http://localhost:8080/api/v1/routes \
  -H 'Content-Type: application/json' \
  -d '{
    "priority": 100,
    "action":   "reject",
    "enabled":  true
  }'
```

**Fields**

| Field        | Type    | Required | Description                                            |
|--------------|---------|----------|--------------------------------------------------------|
| `priority`   | integer | yes      | Lower evaluated first                                  |
| `dest_realm` | string  | no       | Diameter realm to match; `""` = wildcard               |
| `dest_host`  | string  | no       | Diameter identity to match; `""` = wildcard            |
| `app_id`     | integer | no       | Application-ID to match; `0` = any                     |
| `peer_group` | string  | no       | Route to this group (required when `action: route`)    |
| `peer`       | string  | no       | Route to a specific peer by name (overrides peer_group)|
| `action`     | string  | yes      | `route` `reject` `drop`                                |
| `enabled`    | boolean | no       | `false` disables without removing; default `true`      |

**Response `200`** - created rule with its assigned index.

---

### `PUT /api/v1/routes/{index}` - Replace a route rule

Replaces the rule at the given index entirely (all fields required).

```bash
curl -X PUT http://localhost:8080/api/v1/routes/0 \
  -H 'Content-Type: application/json' \
  -d '{
    "priority":   20,
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "app_id":     16777251,
    "peer_group": "hss_group",
    "action":     "route",
    "enabled":    true
  }'
```

**Response `200`** - updated rule.

**Response `404`** - index out of range.

---

### `DELETE /api/v1/routes/{index}` - Delete a route rule

Note: deleting a rule shifts the indices of all rules after it.

```bash
curl -X DELETE http://localhost:8080/api/v1/routes/1
```

**Response `204`** - no body.

**Response `404`** - index out of range.

---

## IMSI Routes

IMSI prefix routing table. Evaluated **before** route rules. Longest prefix wins (6-digit prefix takes priority over 5-digit). On match, `Destination-Realm` is overwritten with `dest_realm` and the message is routed to `peer_group`.

IMSI is extracted from:
1. `Subscription-Id` AVP with `Subscription-Id-Type = END_USER_IMSI` (used by Gx, Gy)
2. `User-Name` AVP in NAI format `<IMSI>@<realm>` (used by S6a, SWx)

Rules are identified by zero-based **index** in the ordered list.

---

### `GET /api/v1/imsi-routes` - List IMSI routes

```bash
curl http://localhost:8080/api/v1/imsi-routes
```

**Response `200`**
```json
[
  {
    "index":      0,
    "prefix":     "311435",
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "peer_group": "hss_group",
    "priority":   10,
    "enabled":    true
  },
  {
    "index":      1,
    "prefix":     "310260",
    "dest_realm": "epc.mnc260.mcc310.3gppnetwork.org",
    "peer_group": "roaming_ipe",
    "priority":   10,
    "enabled":    true
  }
]
```

---

### `GET /api/v1/imsi-routes/{index}` - Get an IMSI route

```bash
curl http://localhost:8080/api/v1/imsi-routes/0
```

---

### `POST /api/v1/imsi-routes` - Create an IMSI route

```bash
# Add home network prefix (MCC=311 MNC=435)
curl -X POST http://localhost:8080/api/v1/imsi-routes \
  -H 'Content-Type: application/json' \
  -d '{
    "prefix":     "311435",
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "peer_group": "hss_group",
    "priority":   10,
    "enabled":    true
  }'

# Add roaming partner (T-Mobile US, MCC=310 MNC=260)
curl -X POST http://localhost:8080/api/v1/imsi-routes \
  -H 'Content-Type: application/json' \
  -d '{
    "prefix":     "310260",
    "dest_realm": "epc.mnc260.mcc310.3gppnetwork.org",
    "peer_group": "roaming_ipe",
    "priority":   10,
    "enabled":    true
  }'

# 5-digit prefix - MCC=204 MNC=16 (T-Mobile NL)
curl -X POST http://localhost:8080/api/v1/imsi-routes \
  -H 'Content-Type: application/json' \
  -d '{
    "prefix":     "20416",
    "dest_realm": "epc.mnc016.mcc204.3gppnetwork.org",
    "peer_group": "roaming_ipe",
    "priority":   10,
    "enabled":    true
  }'
```

**Fields**

| Field        | Type    | Required | Description                                              |
|--------------|---------|----------|----------------------------------------------------------|
| `prefix`     | string  | yes      | MCC+MNC digits - 5 chars (2-digit MNC) or 6 chars (3-digit MNC) |
| `dest_realm` | string  | yes      | Diameter realm to inject as `Destination-Realm`          |
| `peer_group` | string  | yes      | Route matched messages to this peer group                |
| `priority`   | integer | no       | Used for ordering; default `10`                          |
| `enabled`    | boolean | no       | Default `true`                                           |

**Prefix format**: digits only, no dashes. MNC is **not** zero-padded in the prefix (use `"20416"` not `"204016"`). The derived `dest_realm` should use the 3GPP zero-padded format per TS 23.003: `epc.mnc<MNC3>.mcc<MCC>.3gppnetwork.org`.

**Response `200`** - created route with its assigned index.

---

### `PUT /api/v1/imsi-routes/{index}` - Replace an IMSI route

```bash
curl -X PUT http://localhost:8080/api/v1/imsi-routes/1 \
  -H 'Content-Type: application/json' \
  -d '{
    "prefix":     "310260",
    "dest_realm": "epc.mnc260.mcc310.3gppnetwork.org",
    "peer_group": "roaming_ipe_2",
    "priority":   10,
    "enabled":    true
  }'
```

**Response `200`** - updated route.

**Response `404`** - index out of range.

---

### `DELETE /api/v1/imsi-routes/{index}` - Delete an IMSI route

```bash
curl -X DELETE http://localhost:8080/api/v1/imsi-routes/2
```

**Response `204`** - no body.

**Response `404`** - index out of range.

---

## OAM

Operations and management endpoints: status, config reload, log level, recent messages, and metrics.

---

### `GET /api/v1/oam/status` - DRA status

```bash
curl http://localhost:8080/api/v1/oam/status
```

**Response `200`**
```json
{
  "identity":       "dra.epc.mnc435.mcc311.3gppnetwork.org",
  "realm":          "epc.mnc435.mcc311.3gppnetwork.org",
  "product_name":   "VectorCore DRA",
  "uptime":         "2h35m10s",
  "uptime_seconds": 9310.0,
  "started_at":     "2026-03-23T08:00:00Z",
  "peers_total":    4,
  "peers_open":     3,
  "peers_closed":   1,
  "version":        "1.0.0",
  "log_level":      "info"
}
```

---

### `POST /api/v1/oam/reload` - Force config reload

Re-reads `config.yaml` from disk and applies changes to peers, IMSI routes, and route rules. Equivalent to the file watcher triggering on a change.

```bash
curl -X POST http://localhost:8080/api/v1/oam/reload
```

**Response `204`** - no body.

---

### `POST /api/v1/oam/log-level` - Change log level at runtime

Changes the log verbosity without restart. Does **not** persist across restarts (use the config file for permanent changes).

```bash
# Enable debug logging
curl -X POST http://localhost:8080/api/v1/oam/log-level \
  -H 'Content-Type: application/json' \
  -d '{ "level": "debug" }'

# Return to info
curl -X POST http://localhost:8080/api/v1/oam/log-level \
  -H 'Content-Type: application/json' \
  -d '{ "level": "info" }'
```

**Valid levels**: `debug` `info` `warn` `error`

**Response `204`** - no body.

---

### `GET /api/v1/oam/recent-messages` - Recent Diameter messages

Returns the last 20 Diameter messages processed by the DRA. FSM messages (CER/CEA, DWR/DWA, DPR/DPA) are excluded - only routed application traffic is shown.

```bash
curl http://localhost:8080/api/v1/oam/recent-messages
```

**Response `200`**
```json
[
  {
    "timestamp":    "2026-03-23T10:30:15.123Z",
    "direction":    "in",
    "from_peer":    "mme01.epc.mnc435.mcc311.3gppnetwork.org",
    "to_peer":      "hss01.epc.mnc435.mcc311.3gppnetwork.org",
    "command_code": 316,
    "command_name": "ULR",
    "app_id":       16777251,
    "app_name":     "S6a",
    "is_request":   true,
    "result_code":  0,
    "session_id":   "mme01.epc.mnc435.mcc311.3gppnetwork.org;1234567890;1;mme01"
  },
  {
    "timestamp":    "2026-03-23T10:30:15.145Z",
    "direction":    "out",
    "from_peer":    "hss01.epc.mnc435.mcc311.3gppnetwork.org",
    "to_peer":      "mme01.epc.mnc435.mcc311.3gppnetwork.org",
    "command_code": 316,
    "command_name": "ULA",
    "app_id":       16777251,
    "app_name":     "S6a",
    "is_request":   false,
    "result_code":  2001,
    "session_id":   "mme01.epc.mnc435.mcc311.3gppnetwork.org;1234567890;1;mme01"
  }
]
```

**Fields**

| Field          | Description                                                  |
|----------------|--------------------------------------------------------------|
| `timestamp`    | When the message was processed (RFC 3339)                    |
| `direction`    | `"in"` = received from peer, `"out"` = sent to peer         |
| `from_peer`    | FQDN of the originating peer                                 |
| `to_peer`      | FQDN of the destination peer (empty for unrouted messages)   |
| `command_code` | Diameter Command-Code                                        |
| `command_name` | Human-readable command name (e.g. `ULR`, `CCR`, `RAR`)      |
| `app_id`       | Diameter Application-ID                                      |
| `app_name`     | Human-readable application name (e.g. `S6a`, `Gx`)          |
| `is_request`   | `true` = request (R-bit set), `false` = answer               |
| `result_code`  | Result-Code AVP value; `0` = not present (requests)         |
| `session_id`   | Session-Id AVP value                                         |

Results are ordered newest-first. The buffer holds the last 20 messages.

---

### `GET /api/v1/oam/metrics` - Diameter metrics snapshot (JSON)

Returns current values for all `dra_*` Prometheus metrics as JSON. For Prometheus scraping use the `/metrics` endpoint instead.

```bash
curl http://localhost:8080/api/v1/oam/metrics
```

**Response `200`**
```json
{
  "collected_at": "2026-03-23T10:30:00Z",
  "metrics": [
    {
      "name":    "dra_messages_total",
      "help":    "Total Diameter messages processed",
      "type":    "COUNTER",
      "samples": [
        {
          "labels": { "peer": "hss01", "direction": "in", "command_code": "316" },
          "value":  14823
        }
      ]
    },
    {
      "name":    "dra_peer_state",
      "help":    "Current FSM state of each peer",
      "type":    "GAUGE",
      "samples": [
        {
          "labels": { "peer": "hss01", "transport": "tcp", "mode": "active" },
          "value":  2
        }
      ]
    }
  ]
}
```

Peer state gauge values: `0`=CLOSED, `1`=CONNECTING, `2`=OPEN, `3`=DRAINING.

---

## Observability

These endpoints are not under `/api/v1` and have no authentication.

---

### `GET /metrics` - Prometheus metrics

Standard Prometheus exposition format. Scrape this endpoint with your Prometheus server.

```bash
curl http://localhost:8080/metrics
```

```
# HELP dra_messages_total Total Diameter messages processed
# TYPE dra_messages_total counter
dra_messages_total{peer="hss01",direction="in",command_code="316"} 14823
dra_messages_total{peer="hss01",direction="out",command_code="316"} 14823
...
# HELP dra_peer_state Current FSM state of each peer (0=closed 1=connecting 2=open 3=draining)
# TYPE dra_peer_state gauge
dra_peer_state{peer="hss01",transport="tcp",mode="active"} 2
...
# HELP dra_answer_latency_seconds Request to answer latency in seconds
# TYPE dra_answer_latency_seconds histogram
dra_answer_latency_seconds_bucket{peer="hss01",command_code="316",le="0.001"} 1200
dra_answer_latency_seconds_bucket{peer="hss01",command_code="316",le="0.005"} 14600
...
dra_answer_latency_seconds_sum{peer="hss01",command_code="316"} 45.32
dra_answer_latency_seconds_count{peer="hss01",command_code="316"} 14823
```

Example Prometheus scrape config:

```yaml
scrape_configs:
  - job_name: vectorcore_dra
    static_configs:
      - targets: ['<dra-host>:8080']
```

---

### `GET /health` - Liveness probe

Returns HTTP 200 when the process is running. Use for load balancer health checks and container probes.

```bash
curl http://localhost:8080/health
```

**Response `200`**
```json
{ "status": "ok" }
```

---

## Common Diameter Application IDs

| Application    | ID         | Interface       |
|----------------|------------|-----------------|
| Gx             | 16777238   | PCEF <-> PCRF     |
| S6a            | 16777251   | MME <-> HSS       |
| Rx             | 16777236   | P-CSCF <-> PCRF   |
| Gy / Ro        | 16777224   | OCS             |
| Cx / Dx        | 16777216   | P-CSCF <-> HSS    |
| S13            | 16777252   | MME <-> EIR       |
| SWx            | 16777265   | AAA <-> HSS       |
| S6b            | 16777272   | PGW <-> AAA       |
| SLh            | 16777291   | LRF <-> HSS       |
| Relay          | 4294967295 | Any             |

---

## Error Responses

All error responses follow the same shape:

```json
{
  "title":  "Not Found",
  "status": 404,
  "detail": "peer \"pcrf99\" not found"
}
```

| HTTP Status | Meaning                                              |
|-------------|------------------------------------------------------|
| 400         | Request validation failed (missing/invalid fields)   |
| 404         | Resource not found                                   |
| 409         | Conflict (duplicate name)                            |
| 500         | Internal error (config save failure, etc.)           |
