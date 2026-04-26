# VectorCore DRA REST API

Base URL: `http://<host>:8080/api/v1`

- Swagger UI: `http://<host>:8080/api/v1/docs`
- OpenAPI JSON: `http://<host>:8080/api/v1/openapi.json`
- Health: `http://<host>:8080/health`
- Prometheus text: `http://<host>:8080/metrics`

All request and response bodies are JSON.

- `GET`, `POST`, `PUT`, and `PATCH` return `200` with a body unless noted.
- `DELETE` endpoints return `204 No Content`.
- `POST /oam/reload` and `POST /oam/log-level` return `204 No Content`.
- Errors return JSON with a `detail` field.

## Conventions

- Peer, route, and IMSI route config is persisted back to the YAML config file.
- `lb_group` is the field name used throughout the API.
- `weight` is stored on peers for compatibility, but there is no weighted balancing policy today.
- Routes and IMSI routes are addressed by zero-based list index.

## Peers

Configured peers:

### `GET /peers`

```bash
curl http://localhost:8080/api/v1/peers
```

Example response:

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
    "lb_group": "hss_group",
    "weight": 1,
    "enabled": true
  }
]
```

### `GET /peers/{name}`

```bash
curl http://localhost:8080/api/v1/peers/hss01
```

### `POST /peers`

Supported `transport` values for API-managed peers:

- `tcp`
- `tcp+tls`
- `sctp`

```bash
curl -X POST http://localhost:8080/api/v1/peers \
  -H 'Content-Type: application/json' \
  -d '{
    "name": "hss01",
    "fqdn": "hss01.epc.mnc435.mcc311.3gppnetwork.org",
    "address": "10.0.0.10",
    "port": 3868,
    "transport": "tcp",
    "mode": "active",
    "realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "lb_group": "hss_group",
    "enabled": true
  }'
```

Fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `name` | string | yes | unique key |
| `fqdn` | string | yes | peer Diameter identity |
| `address` | string | yes | IP or hostname |
| `port` | integer | yes | `1-65535` |
| `transport` | string | yes | `tcp`, `tcp+tls`, `sctp` |
| `mode` | string | no | `active` or `passive`, default `active` |
| `realm` | string | yes | peer realm |
| `lb_group` | string | no | load-balancing group name |
| `weight` | integer | no | stored but not used by routing today |
| `enabled` | boolean | no | default `true` |

### `PATCH /peers/{name}`

Only provided fields are changed.

```bash
curl -X PATCH http://localhost:8080/api/v1/peers/hss01 \
  -H 'Content-Type: application/json' \
  -d '{ "enabled": false }'
```

Changing `fqdn`, `address`, `port`, `transport`, `mode`, or `realm` causes the peer to restart.

### `DELETE /peers/{name}`

```bash
curl -X DELETE http://localhost:8080/api/v1/peers/hss01
```

## Peer Status

Live peer state:

### `GET /peers/status`

```bash
curl http://localhost:8080/api/v1/peers/status
```

Example response:

```json
[
  {
    "name": "hss01",
    "fqdn": "hss01.epc.mnc435.mcc311.3gppnetwork.org",
    "state": "OPEN",
    "actual_transport": "tcp",
    "configured_transport": "tcp",
    "remote_addr": "10.0.0.10:3868",
    "peer_fqdn": "hss01.epc.mnc435.mcc311.3gppnetwork.org",
    "peer_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "app_ids": [16777251],
    "applications": ["S6a"],
    "in_flight": 2,
    "connected_at": "2026-04-23T12:00:00Z"
  },
  {
    "name": "mme01",
    "fqdn": "mme01.epc.mnc435.mcc311.3gppnetwork.org",
    "state": "CLOSED",
    "actual_transport": "tcp",
    "configured_transport": "tcp",
    "in_flight": 0
  }
]
```

Observed `state` values:

- `WAIT_CONN_ACK`
- `WAIT_CEA`
- `OPEN`
- `CLOSING`
- `CLOSED`
- `DISABLED`

`actual_transport` may differ from `configured_transport` for inbound connections or legacy unsupported transport configs.

## LB Groups

### `GET /lb-groups`

```bash
curl http://localhost:8080/api/v1/lb-groups
```

### `POST /lb-groups`

Supported `lb_policy` values:

- `round_robin`
- `least_conn`

```bash
curl -X POST http://localhost:8080/api/v1/lb-groups \
  -H 'Content-Type: application/json' \
  -d '{ "name": "hss_group", "lb_policy": "least_conn" }'
```

### `PATCH /lb-groups/{name}`

```bash
curl -X PATCH http://localhost:8080/api/v1/lb-groups/hss_group \
  -H 'Content-Type: application/json' \
  -d '{ "lb_policy": "round_robin" }'
```

### `DELETE /lb-groups/{name}`

```bash
curl -X DELETE http://localhost:8080/api/v1/lb-groups/hss_group
```

## Route Rules

Important behavior:

- Message `Destination-Host` is handled before these rules.
- Route rules match on `dest_realm` and `app_id`.
- `dest_host` in a rule is the target peer FQDN to send to, not an extra match condition.
- If `action` is `route` and both `dest_host` and `lb_group` are empty, the router falls back to implicit realm routing.

### `GET /routes`

```bash
curl http://localhost:8080/api/v1/routes
```

Example response:

```json
[
  {
    "index": 0,
    "priority": 20,
    "dest_host": "",
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "app_id": 16777238,
    "lb_group": "pcrf_group",
    "action": "route",
    "enabled": true
  }
]
```

### `GET /routes/{index}`

```bash
curl http://localhost:8080/api/v1/routes/0
```

### `POST /routes`

```bash
curl -X POST http://localhost:8080/api/v1/routes \
  -H 'Content-Type: application/json' \
  -d '{
    "priority": 20,
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "app_id": 16777238,
    "lb_group": "pcrf_group",
    "action": "route",
    "enabled": true
  }'
```

Fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `priority` | integer | yes | lower runs first |
| `dest_realm` | string | no | empty means wildcard |
| `app_id` | integer | no | `0` means wildcard |
| `dest_host` | string | no | route target peer FQDN |
| `lb_group` | string | no | route target group |
| `action` | string | yes | `route`, `reject`, `drop` |
| `enabled` | boolean | no | default `true` |

### `PUT /routes/{index}`

```bash
curl -X PUT http://localhost:8080/api/v1/routes/0 \
  -H 'Content-Type: application/json' \
  -d '{
    "priority": 20,
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "app_id": 16777251,
    "lb_group": "hss_group",
    "action": "route",
    "enabled": true
  }'
```

### `DELETE /routes/{index}`

```bash
curl -X DELETE http://localhost:8080/api/v1/routes/0
```

## IMSI Routes

Note: IMSI routing is a work in progress and is not currently functional. The endpoints exist, but the feature should be treated as unfinished.

Important behavior:

- IMSI routes are evaluated only after no explicit route rule matches.
- Longest prefix wins; lower `priority` wins among equal-length prefixes.
- `dest_realm` is used for routing decisions and implicit-realm fallback.
- The API and router do not rewrite the message AVPs in place.

### `GET /imsi-routes`

```bash
curl http://localhost:8080/api/v1/imsi-routes
```

Example response:

```json
[
  {
    "index": 0,
    "prefix": "311435",
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "lb_group": "hss_group",
    "priority": 10,
    "enabled": true
  }
]
```

### `GET /imsi-routes/{index}`

```bash
curl http://localhost:8080/api/v1/imsi-routes/0
```

### `POST /imsi-routes`

```bash
curl -X POST http://localhost:8080/api/v1/imsi-routes \
  -H 'Content-Type: application/json' \
  -d '{
    "prefix": "311435",
    "dest_realm": "epc.mnc435.mcc311.3gppnetwork.org",
    "lb_group": "hss_group",
    "priority": 10,
    "enabled": true
  }'
```

Fields:

| Field | Type | Required | Notes |
|---|---|---|---|
| `prefix` | string | yes | 5 or 6 digits |
| `dest_realm` | string | yes | derived routing realm |
| `lb_group` | string | no | target group |
| `priority` | integer | no | default `10` |
| `enabled` | boolean | no | default `true` |

### `PUT /imsi-routes/{index}`

```bash
curl -X PUT http://localhost:8080/api/v1/imsi-routes/0 \
  -H 'Content-Type: application/json' \
  -d '{
    "prefix": "310260",
    "dest_realm": "epc.mnc260.mcc310.3gppnetwork.org",
    "lb_group": "roaming_ipe",
    "priority": 10,
    "enabled": true
  }'
```

### `DELETE /imsi-routes/{index}`

```bash
curl -X DELETE http://localhost:8080/api/v1/imsi-routes/0
```

## OAM

### `GET /oam/status`

```bash
curl http://localhost:8080/api/v1/oam/status
```

Example response:

```json
{
  "identity": "dra.epc.mnc435.mcc311.3gppnetwork.org",
  "realm": "epc.mnc435.mcc311.3gppnetwork.org",
  "product_name": "VectorCore DRA",
  "uptime": "1h2m3s",
  "uptime_seconds": 3723,
  "started_at": "2026-04-23T11:00:00Z",
  "peers_total": 3,
  "peers_open": 2,
  "peers_closed": 1,
  "version": "0.4.0B",
  "log_level": "info"
}
```

`peers_total`, `peers_open`, and `peers_closed` are derived from the running peer manager, so they reflect enabled/runtime peers rather than the raw configured peer count.

### `POST /oam/reload`

Re-reads the config file from disk and reapplies the mutable runtime sections.

```bash
curl -X POST http://localhost:8080/api/v1/oam/reload
```

### `POST /oam/log-level`

```bash
curl -X POST http://localhost:8080/api/v1/oam/log-level \
  -H 'Content-Type: application/json' \
  -d '{ "level": "debug" }'
```

Accepted values: `debug`, `info`, `warn`, `error`.

### `GET /oam/recent-messages`

Returns the most recent 20 non-FSM Diameter messages.

```bash
curl http://localhost:8080/api/v1/oam/recent-messages
```

### `GET /oam/metrics`

Returns a JSON snapshot of all `dra_*` Prometheus metrics.

```bash
curl http://localhost:8080/api/v1/oam/metrics
```

## Health And Prometheus

### `GET /health`

```bash
curl http://localhost:8080/health
```

Response:

```json
{ "status": "ok" }
```

### `GET /metrics`

```bash
curl http://localhost:8080/metrics
```
