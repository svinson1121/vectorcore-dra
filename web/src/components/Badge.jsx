import React from 'react'

const STATE_MAP = {
  OPEN: { cls: 'badge-open', label: 'OPEN' },
  CLOSED: { cls: 'badge-closed', label: 'CLOSED' },
  WAIT_CEA: { cls: 'badge-wait_cea', label: 'WAIT CEA' },
  WAIT_CONN_ACK: { cls: 'badge-wait_conn_ack', label: 'WAIT ACK' },
  CLOSING: { cls: 'badge-closing', label: 'CLOSING' },
  DISABLED: { cls: 'badge-disabled', label: 'DISABLED' },
  active: { cls: 'badge-active', label: 'active' },
  passive: { cls: 'badge-passive', label: 'passive' },
  tcp: { cls: 'badge-tcp', label: 'tcp' },
  sctp: { cls: 'badge-sctp', label: 'sctp' },
  'tcp+tls': { cls: 'badge-tcptls', label: 'tcp+tls' },
  'sctp+tls': { cls: 'badge-sctptls', label: 'sctp+tls' },
  route: { cls: 'badge-route', label: 'route' },
  reject: { cls: 'badge-reject', label: 'reject' },
  drop: { cls: 'badge-drop', label: 'drop' },
}

export default function Badge({ state, label: labelOverride }) {
  if (!state) return null
  const entry = STATE_MAP[state] || { cls: 'badge-disabled', label: state }
  return (
    <span className={`badge ${entry.cls}`}>
      {labelOverride || entry.label}
    </span>
  )
}
