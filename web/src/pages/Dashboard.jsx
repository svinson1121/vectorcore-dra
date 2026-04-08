import React, { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer
} from 'recharts'
import { Activity, Users, CheckCircle, XCircle, Clock, MessageSquare } from 'lucide-react'
import StatCard from '../components/StatCard.jsx'
import Badge from '../components/Badge.jsx'
import Spinner from '../components/Spinner.jsx'
import { getStatus, getPeers, getPeerStatus, getPrometheusText, parsePrometheusText, sumMetric } from '../api/client.js'

const MAX_HISTORY = 60

function formatUptime(seconds) {
  if (seconds == null || isNaN(seconds)) return '—'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (d > 0) return `${d}d ${h}h ${m}m`
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function formatTime(date) {
  return date.toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

const CustomTooltip = ({ active, payload, label }) => {
  if (!active || !payload || !payload.length) return null
  return (
    <div style={{
      background: 'var(--bg-elevated)',
      border: '1px solid var(--border)',
      borderRadius: 'var(--radius-sm)',
      padding: '8px 12px',
      fontSize: '0.75rem',
    }}>
      <div style={{ color: 'var(--text-muted)', marginBottom: 4 }}>{label}</div>
      {payload.map(p => (
        <div key={p.dataKey} style={{ color: p.color }}>
          {p.name}: <strong>{p.value}</strong> <span style={{ opacity: 0.7 }}>msg/s</span>
        </div>
      ))}
    </div>
  )
}

export default function Dashboard() {
  const navigate = useNavigate()
  const [status, setStatus] = useState(null)
  const [peers, setPeers] = useState([])
  const [peerStatus, setPeerStatus] = useState([])
  const [totalMessages, setTotalMessages] = useState(0)
  const [msgHistory, setMsgHistory] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const prevTotalRef = useRef(null)
  const timerRef = useRef(null)
  const mountedRef = useRef(true)

  const fetchAll = useCallback(async () => {
    try {
      const [s, p, ps, promText] = await Promise.all([
        getStatus(), getPeers(), getPeerStatus(), getPrometheusText()
      ])
      if (!mountedRef.current) return

      const metrics = parsePrometheusText(promText)
      const total = sumMetric(metrics, 'dra_messages_total')

      const now = new Date()
      setMsgHistory(prev => {
        const rate = prevTotalRef.current !== null ? Math.max(0, (total - prevTotalRef.current) / 5) : 0
        prevTotalRef.current = total
        const next = [...prev, { time: formatTime(now), rate: parseFloat(rate.toFixed(2)) }]
        const trimmed = next.length > MAX_HISTORY ? next.slice(next.length - MAX_HISTORY) : next
        // Compute 1-minute rolling average (last 12 points × 5s = 60s) for each point
        return trimmed.map((p, i, arr) => {
          const window = arr.slice(Math.max(0, i - 11), i + 1)
          const avg = window.reduce((s, x) => s + x.rate, 0) / window.length
          return { ...p, avg1m: parseFloat(avg.toFixed(2)) }
        })
      })

      setStatus(s)
      setPeers(Array.isArray(p) ? p : [])
      setPeerStatus(Array.isArray(ps) ? ps : [])
      setTotalMessages(total)
      setError(null)
      setLoading(false)
    } catch (err) {
      if (!mountedRef.current) return
      setError(err.message || 'Failed to load data')
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    mountedRef.current = true
    fetchAll()
    timerRef.current = setInterval(fetchAll, 5000)
    return () => {
      mountedRef.current = false
      clearInterval(timerRef.current)
    }
  }, [fetchAll])

  if (loading) {
    return (
      <div className="loading-center">
        <Spinner size="lg" />
        <span>Loading dashboard...</span>
      </div>
    )
  }

  if (error && !status) {
    return (
      <div className="error-state">
        <XCircle size={32} className="error-icon" />
        <div>{error}</div>
        <button className="btn btn-ghost mt-12" onClick={fetchAll}>Retry</button>
      </div>
    )
  }

  // Build merged peer list for the table
  const statusMap = {}
  for (const s of peerStatus) statusMap[s.name] = s

  const openCount = peerStatus.filter(s => s.state === 'OPEN').length
  const disabledCount = peers.filter(p => !p.enabled).length
  const totalCount = peers.length

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Dashboard</div>
          <div className="page-subtitle">Diameter Routing Agent — real-time overview</div>
        </div>
      </div>

      <div className="stats-grid">
        <StatCard
          title="Peers Open"
          value={openCount}
          icon={<CheckCircle size={18} />}
          color="var(--success)"
          subtitle={`of ${totalCount} configured`}
        />
        <StatCard
          title="Peers Disabled"
          value={disabledCount}
          icon={<XCircle size={18} />}
          color="var(--text-muted)"
        />
        <StatCard
          title="Total Messages"
          value={totalMessages.toLocaleString()}
          icon={<MessageSquare size={18} />}
          color="var(--accent)"
          subtitle="cumulative since start"
        />
        <StatCard
          title="Uptime"
          value={formatUptime(status?.uptime_seconds)}
          icon={<Clock size={18} />}
          color="var(--warning)"
          subtitle={status?.started_at ? `since ${new Date(status.started_at).toLocaleString()}` : undefined}
        />
      </div>

      <div className="chart-card mb-16">
        <div className="chart-title">
          <span>Message Rate (msg/s)</span>
          <span style={{ display: 'flex', alignItems: 'center', gap: 12, fontSize: '0.72rem', fontWeight: 400 }}>
            <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <span style={{ display: 'inline-block', width: 16, height: 2, background: 'var(--accent)' }} />
              <span style={{ color: 'var(--text-muted)' }}>live</span>
            </span>
            <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <span style={{ display: 'inline-block', width: 16, height: 2, background: 'var(--warning)', borderTop: '2px dashed var(--warning)' }} />
              <span style={{ color: 'var(--text-muted)' }}>1m avg</span>
            </span>
            <Activity size={14} style={{ color: 'var(--text-muted)' }} />
          </span>
        </div>
        <ResponsiveContainer width="100%" height={200}>
          <LineChart data={msgHistory} margin={{ top: 4, right: 8, left: 0, bottom: 0 }}>
            <CartesianGrid strokeDasharray="3 3" stroke="var(--border-subtle)" />
            <XAxis
              dataKey="time"
              tick={{ fontSize: 10, fill: 'var(--text-muted)' }}
              interval={Math.floor(msgHistory.length / 6) || 1}
            />
            <YAxis tick={{ fontSize: 10, fill: 'var(--text-muted)' }} width={36} allowDecimals={false} />
            <Tooltip content={<CustomTooltip />} />
            <Line
              type="monotone"
              dataKey="rate"
              name="msg/s"
              stroke="var(--accent)"
              strokeWidth={1.5}
              dot={false}
              isAnimationActive={false}
            />
            <Line
              type="monotone"
              dataKey="avg1m"
              name="1m avg"
              stroke="var(--warning)"
              strokeWidth={1.5}
              strokeDasharray="4 2"
              dot={false}
              isAnimationActive={false}
            />
          </LineChart>
        </ResponsiveContainer>
      </div>

      <div className="section-title">Peer Status</div>
      {peers.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon"><Users size={32} /></div>
          <div>No peers configured</div>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>FQDN</th>
                <th>Transport</th>
                <th>State</th>
                <th>Applications</th>
                <th>In-Flight</th>
              </tr>
            </thead>
            <tbody>
              {peers.map(peer => {
                const st = statusMap[peer.name]
                const state = st ? st.state : (peer.enabled ? 'CLOSED' : 'DISABLED')
                const apps = st?.applications || []
                const inFlight = st?.in_flight ?? 0
                const transport = st?.actual_transport || peer.transport
                return (
                  <tr
                    key={peer.name}
                    className="expandable"
                    onClick={() => navigate('/peers')}
                    title="Go to Peers page"
                  >
                    <td style={{ fontWeight: 600 }}>{peer.name}</td>
                    <td className="mono truncate" style={{ maxWidth: 260, fontSize: '0.78rem', color: 'var(--text-muted)' }}>
                      {peer.fqdn || '—'}
                    </td>
                    <td><Badge state={transport} /></td>
                    <td><Badge state={state} /></td>
                    <td>
                      <div className="flex gap-4" style={{ flexWrap: 'wrap' }}>
                        {apps.map(a => <span key={a} className="app-tag">{a}</span>)}
                      </div>
                    </td>
                    <td className="mono" style={{ color: inFlight > 0 ? 'var(--warning)' : 'var(--text-muted)' }}>
                      {inFlight}
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
