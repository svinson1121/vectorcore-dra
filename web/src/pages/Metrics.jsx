import React, { useCallback } from 'react'
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
} from 'recharts'
import { Activity, TrendingUp, AlertTriangle, Clock, XCircle, RefreshCw } from 'lucide-react'
import StatCard from '../components/StatCard.jsx'
import Spinner from '../components/Spinner.jsx'
import { usePoller } from '../hooks/usePoller.js'
import { getPrometheusText, parsePrometheusText, sumMetric, getRecentMessages } from '../api/client.js'

const STATE_NAMES = { 0: 'CLOSED', 1: 'CONNECTING', 2: 'OPEN', 3: 'DRAINING' }
const STATE_COLORS = {
  CLOSED: 'var(--danger)',
  CONNECTING: 'var(--warning)',
  OPEN: 'var(--success)',
  DRAINING: 'var(--warning)',
}

// Build per-peer message counts (in + out) from dra_messages_total
function buildPeerMsgMap(metrics) {
  const m = metrics['dra_messages_total']
  if (!m) return {}
  const byPeer = {}
  for (const s of m.samples) {
    const peer = s.labels.peer
    if (!peer) continue
    const dir = s.labels.direction || 'in'
    if (!byPeer[peer]) byPeer[peer] = { in: 0, out: 0 }
    byPeer[peer][dir] = (byPeer[peer][dir] || 0) + (isNaN(s.value) ? 0 : s.value)
  }
  return byPeer
}

// Compute average answer latency (ms) per peer from histogram _sum / _count
function buildLatencyData(metrics) {
  const counts = {}
  const sums = {}
  const cnt = metrics['dra_answer_latency_seconds_count']
  const sum = metrics['dra_answer_latency_seconds_sum']
  if (cnt) {
    for (const s of cnt.samples) {
      const peer = s.labels.peer
      if (!peer) continue
      counts[peer] = (counts[peer] || 0) + (isNaN(s.value) ? 0 : s.value)
    }
  }
  if (sum) {
    for (const s of sum.samples) {
      const peer = s.labels.peer
      if (!peer) continue
      sums[peer] = (sums[peer] || 0) + (isNaN(s.value) ? 0 : s.value)
    }
  }
  return Object.keys(counts)
    .filter(peer => counts[peer] > 0)
    .map(peer => ({
      peer,
      avgMs: parseFloat(((sums[peer] || 0) / counts[peer] * 1000).toFixed(2)),
      count: counts[peer],
    }))
    .sort((a, b) => b.avgMs - a.avgMs)
    .slice(0, 20)
}

function buildPeerStateData(metrics, msgMap) {
  const m = metrics['dra_peer_state']
  if (!m) return []
  // Deduplicate by peer — keep the highest state value (OPEN=2 wins over CLOSED=0)
  const byPeer = {}
  for (const s of m.samples) {
    const peer = s.labels.peer || '—'
    if (!byPeer[peer] || s.value > byPeer[peer].stateVal) {
      const msgs = msgMap[peer] || { in: 0, out: 0 }
      byPeer[peer] = {
        peer,
        stateVal: s.value,
        stateName: STATE_NAMES[s.value] || String(s.value),
        msgsIn: msgs.in,
        msgsOut: msgs.out,
      }
    }
  }
  return Object.values(byPeer)
}

function buildReconnectData(metrics) {
  const m = metrics['dra_reconnect_attempts_total']
  if (!m) return []
  return m.samples.map(s => ({
    peer: s.labels.peer || '—',
    attempts: isNaN(s.value) ? 0 : s.value,
  })).sort((a, b) => b.attempts - a.attempts)
}

// Truncate FQDN to first label for chart axis (shorter version for axis ticks)
function fqdnShortAxis(fqdn) {
  if (!fqdn) return ''
  const first = fqdn.split('.')[0]
  return first.length > 16 ? first.slice(0, 14) + '…' : first
}

const CustomTooltip = ({ active, payload, label }) => {
  if (!active || !payload || !payload.length) return null
  return (
    <div style={{
      background: 'var(--bg-elevated)', border: '1px solid var(--border)',
      borderRadius: 'var(--radius-sm)', padding: '8px 12px', fontSize: '0.75rem',
    }}>
      <div style={{ color: 'var(--text-muted)', marginBottom: 4, fontFamily: 'var(--font-mono)', fontSize: '0.7rem' }}>
        {label}
      </div>
      {payload.map(p => (
        <div key={p.dataKey} style={{ color: p.fill || p.color }}>
          {p.name}: <strong>{p.value}</strong>
        </div>
      ))}
    </div>
  )
}

function fqdnShort(fqdn) {
  if (!fqdn) return '—'
  const first = fqdn.split('.')[0]
  return first.length > 20 ? first.slice(0, 18) + '…' : first
}

function RecentMessages() {
  const fetchFn = useCallback(getRecentMessages, [])
  const { data: msgs, refresh } = usePoller(fetchFn, 5000)
  const list = Array.isArray(msgs) ? msgs : []

  return (
    <div className="mb-20">
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 10 }}>
        <div className="section-title" style={{ marginBottom: 0 }}>Recent Diameter Messages</div>
        <button className="btn btn-ghost" style={{ padding: '4px 10px', fontSize: '0.75rem' }} onClick={refresh}>
          <RefreshCw size={12} /> Refresh
        </button>
      </div>
      {list.length === 0 ? (
        <div className="empty-state" style={{ padding: '20px 0' }}>No messages yet — traffic will appear here as it flows through the DRA.</div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Time</th>
                <th>Dir</th>
                <th>From</th>
                <th>To</th>
                <th>Command</th>
                <th>App</th>
                <th>Type</th>
                <th>Result</th>
              </tr>
            </thead>
            <tbody>
              {list.map((m, i) => {
                const t = new Date(m.timestamp)
                const timeStr = t.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
                const isOk = !m.result_code || m.result_code === 2001 || m.result_code === 2002
                return (
                  <tr key={i}>
                    <td className="mono" style={{ fontSize: '0.75rem', color: 'var(--text-muted)', whiteSpace: 'nowrap' }}>
                      {timeStr}
                    </td>
                    <td>
                      <span style={{
                        fontSize: '0.7rem', fontWeight: 700, padding: '2px 6px',
                        borderRadius: 3, fontFamily: 'var(--font-mono)',
                        background: m.direction === 'out' ? 'rgba(88,166,255,0.15)' : 'rgba(63,185,80,0.15)',
                        color: m.direction === 'out' ? 'var(--accent)' : 'var(--success)',
                      }}>
                        {m.direction.toUpperCase()}
                      </span>
                    </td>
                    <td className="mono" style={{ fontSize: '0.78rem' }} title={m.from_peer}>
                      {fqdnShort(m.from_peer)}
                    </td>
                    <td className="mono" style={{ fontSize: '0.78rem', color: 'var(--text-muted)' }} title={m.to_peer}>
                      {m.to_peer ? fqdnShort(m.to_peer) : '—'}
                    </td>
                    <td>
                      <span className="mono" style={{ fontSize: '0.78rem', color: 'var(--accent)' }}>
                        {m.command_name}
                      </span>
                      <span style={{ fontSize: '0.65rem', color: 'var(--text-muted)', marginLeft: 4 }}>
                        ({m.command_code})
                      </span>
                    </td>
                    <td style={{ fontSize: '0.75rem', color: 'var(--text-muted)' }}>
                      {m.app_name}
                    </td>
                    <td>
                      <span style={{
                        fontSize: '0.7rem', color: m.is_request ? 'var(--warning)' : 'var(--text-muted)',
                      }}>
                        {m.is_request ? 'REQ' : 'ANS'}
                      </span>
                    </td>
                    <td className="mono" style={{
                      fontSize: '0.78rem',
                      color: m.result_code ? (isOk ? 'var(--success)' : 'var(--danger)') : 'var(--text-muted)',
                    }}>
                      {m.result_code || '—'}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}

export default function Metrics() {
  const fetchFn = useCallback(getPrometheusText, [])
  const { data: rawText, error, loading, refresh } = usePoller(fetchFn)

  if (loading) {
    return (
      <div className="loading-center">
        <Spinner size="lg" />
        <span>Loading metrics...</span>
      </div>
    )
  }

  if (error && !rawText) {
    return (
      <div className="error-state">
        <XCircle size={32} className="error-icon" />
        <div>{error}</div>
        <button className="btn btn-ghost mt-12" onClick={refresh}>
          <RefreshCw size={14} /> Retry
        </button>
      </div>
    )
  }

  const metrics = parsePrometheusText(rawText || '')
  const totalMsgs = sumMetric(metrics, 'dra_messages_total')
  const forwarded = sumMetric(metrics, 'dra_forwarded_total')
  const misses = sumMetric(metrics, 'dra_route_misses_total')
  const wdTimeouts = sumMetric(metrics, 'dra_watchdog_timeout_total')

  const msgMap = buildPeerMsgMap(metrics)
  const latencyData = buildLatencyData(metrics)
  const peerStateData = buildPeerStateData(metrics, msgMap)
  const reconnectData = buildReconnectData(metrics)

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Metrics</div>
        </div>
        <button className="btn btn-ghost" onClick={refresh}>
          <RefreshCw size={14} /> Refresh
        </button>
      </div>

      <RecentMessages />

      <div className="stats-grid">
        <StatCard title="Total Messages" value={totalMsgs.toLocaleString()} icon={<Activity size={18} />} color="var(--accent)" />
        <StatCard title="Forwarded" value={forwarded.toLocaleString()} icon={<TrendingUp size={18} />} color="var(--success)" />
        <StatCard title="Route Misses" value={misses.toLocaleString()} icon={<AlertTriangle size={18} />} color="var(--warning)" />
        <StatCard title="Watchdog Timeouts" value={wdTimeouts.toLocaleString()} icon={<Clock size={18} />} color="var(--danger)" />
      </div>

      {latencyData.length > 0 && (
        <div className="chart-card mb-16">
          <div className="chart-title">
            Avg Answer Latency per Peer
            <span style={{ fontSize: '0.72rem', color: 'var(--text-muted)', fontWeight: 400, marginLeft: 8 }}>
              (sum / count, ms)
            </span>
          </div>
          <ResponsiveContainer width="100%" height={200}>
            <BarChart data={latencyData} margin={{ top: 4, right: 8, left: 0, bottom: 80 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" />
              <XAxis
                dataKey="peer"
                tickFormatter={fqdnShortAxis}
                tick={{ fontSize: 10, fill: 'var(--text-muted)', fontFamily: 'var(--font-mono)' }}
                angle={-35} textAnchor="end" interval={0} height={80}
              />
              <YAxis tick={{ fontSize: 10, fill: 'var(--text-muted)' }} width={52} unit="ms" />
              <Tooltip
                content={({ active, payload, label }) => {
                  if (!active || !payload?.length) return null
                  const d = payload[0]?.payload
                  return (
                    <div style={{
                      background: 'var(--bg-elevated)', border: '1px solid var(--border)',
                      borderRadius: 'var(--radius-sm)', padding: '8px 12px', fontSize: '0.75rem',
                    }}>
                      <div style={{ color: 'var(--text-muted)', marginBottom: 4, fontFamily: 'var(--font-mono)', fontSize: '0.7rem' }}>{label}</div>
                      <div style={{ color: 'var(--warning)' }}>Avg: <strong>{d?.avgMs} ms</strong></div>
                      <div style={{ color: 'var(--text-muted)' }}>Samples: {d?.count?.toLocaleString()}</div>
                    </div>
                  )
                }}
              />
              <Bar dataKey="avgMs" name="Avg ms" fill="var(--warning)" radius={[2, 2, 0, 0]} maxBarSize={32} />
            </BarChart>
          </ResponsiveContainer>
        </div>
      )}

      <div className="section-title mt-20">Peer Status</div>
      {peerStateData.length > 0 ? (
        <div className="table-container mb-16">
          <table>
            <thead>
              <tr>
                <th>Peer FQDN</th>
                <th>State</th>
                <th>Msg In</th>
                <th>Msg Out</th>
              </tr>
            </thead>
            <tbody>
              {peerStateData.map((row, i) => (
                <tr key={i}>
                  <td className="mono" style={{ fontSize: '0.8rem' }}>{row.peer}</td>
                  <td>
                    <span style={{ color: STATE_COLORS[row.stateName] || 'var(--text-muted)', fontWeight: 600, fontSize: '0.8rem' }}>
                      {row.stateName}
                    </span>
                  </td>
                  <td className="mono" style={{ color: row.msgsIn > 0 ? 'var(--accent)' : 'var(--text-muted)' }}>
                    {row.msgsIn.toLocaleString()}
                  </td>
                  <td className="mono" style={{ color: row.msgsOut > 0 ? 'var(--success)' : 'var(--text-muted)' }}>
                    {row.msgsOut.toLocaleString()}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="empty-state mb-16">No peer state metrics available</div>
      )}

      <div className="section-title">Reconnect Attempts</div>
      {reconnectData.length > 0 ? (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Peer</th>
                <th>Reconnect Attempts</th>
              </tr>
            </thead>
            <tbody>
              {reconnectData.map((row, i) => (
                <tr key={i}>
                  <td className="mono" style={{ fontSize: '0.8rem' }}>{row.peer}</td>
                  <td className="mono" style={{ color: row.attempts > 0 ? 'var(--warning)' : 'var(--text-muted)', fontWeight: row.attempts > 0 ? 700 : 400 }}>
                    {row.attempts}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : (
        <div className="empty-state">No reconnect data available</div>
      )}
    </div>
  )
}
