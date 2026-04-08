import React, { useState, useCallback, useEffect, useRef } from 'react'
import { RefreshCw, CheckCircle, XCircle, Terminal, Activity, Shield, Settings } from 'lucide-react'
import Spinner from '../components/Spinner.jsx'
import { useToast } from '../components/Toast.jsx'
import { usePoller } from '../hooks/usePoller.js'
import { getStatus, reload, setLogLevel, getHealth } from '../api/client.js'

const LOG_LEVELS = ['debug', 'info', 'warn', 'error']

function formatUptime(seconds) {
  if (!seconds && seconds !== 0) return '—'
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  const s = Math.floor(seconds % 60)
  if (d > 0) return `${d}d ${h}h ${m}m ${s}s`
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function formatDateTime(iso) {
  if (!iso) return '—'
  try {
    return new Date(iso).toLocaleString('en-US', {
      year: 'numeric',
      month: 'short',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
      hour12: false,
    })
  } catch {
    return iso
  }
}

export default function OAM() {
  const toast = useToast()
  const { data: status, error: statusError, loading, refresh: refreshStatus } = usePoller(getStatus)

  const [currentLevel, setCurrentLevel] = useState(null)
  const [levelLoading, setLevelLoading] = useState(false)
  const [reloadLoading, setReloadLoading] = useState(false)
  const [lastReload, setLastReload] = useState(null)
  const [health, setHealth] = useState(null)
  const [healthError, setHealthError] = useState(null)
  const healthTimerRef = useRef(null)
  const mountedRef = useRef(true)

  useEffect(() => {
    mountedRef.current = true
    return () => { mountedRef.current = false }
  }, [])

  // Fetch health independently
  const fetchHealth = useCallback(async () => {
    try {
      const h = await getHealth()
      if (mountedRef.current) {
        setHealth(h)
        setHealthError(null)
      }
    } catch (err) {
      if (mountedRef.current) {
        setHealth(null)
        setHealthError(err.message || 'Health check failed')
      }
    }
  }, [])

  useEffect(() => {
    fetchHealth()
    healthTimerRef.current = setInterval(fetchHealth, 5000)
    return () => clearInterval(healthTimerRef.current)
  }, [fetchHealth])

  // Extract log level from status if available
  useEffect(() => {
    if (status?.log_level) {
      setCurrentLevel(status.log_level)
    }
  }, [status])

  const handleSetLogLevel = useCallback(async (level) => {
    setLevelLoading(true)
    try {
      await setLogLevel(level)
      setCurrentLevel(level)
      toast.success('Log level changed', `Set to ${level}`)
    } catch (err) {
      toast.error('Failed to set log level', err.message)
    } finally {
      setLevelLoading(false)
    }
  }, [toast])

  const handleReload = useCallback(async () => {
    setReloadLoading(true)
    try {
      await reload()
      setLastReload(new Date())
      toast.success('Config reloaded', 'Configuration re-read from file')
      refreshStatus()
    } catch (err) {
      toast.error('Reload failed', err.message)
    } finally {
      setReloadLoading(false)
    }
  }, [toast, refreshStatus])

  if (loading && !status) {
    return (
      <div className="loading-center">
        <Spinner size="lg" />
        <span>Loading status...</span>
      </div>
    )
  }

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">OAM</div>
          <div className="page-subtitle">Operations, administration, and maintenance</div>
        </div>
        <button className="btn btn-ghost" onClick={refreshStatus}>
          <RefreshCw size={14} /> Refresh
        </button>
      </div>

      {/* System Identity */}
      <div className="oam-section">
        <div className="flex items-center gap-8 mb-16">
          <Shield size={16} style={{ color: 'var(--accent)' }} />
          <h3 className="card-title">System Identity</h3>
        </div>

        {statusError && !status ? (
          <div className="error-state" style={{ padding: '20px 0' }}>
            <XCircle size={20} className="error-icon" />
            <div>{statusError}</div>
          </div>
        ) : (
          <div className="detail-grid">
            <div className="detail-row">
              <span className="detail-label">Diameter Identity</span>
              <span className="detail-value mono">{status?.identity || '—'}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Realm</span>
              <span className="detail-value mono">{status?.realm || '—'}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Version</span>
              <span className="detail-value mono">{status?.version || '—'}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Uptime</span>
              <span className="detail-value mono">{formatUptime(status?.uptime_seconds)}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Started At</span>
              <span className="detail-value mono">{formatDateTime(status?.started_at)}</span>
            </div>
            <div className="detail-row">
              <span className="detail-label">Peers Open / Total</span>
              <span className="detail-value">
                <span style={{ color: 'var(--success)', fontWeight: 700 }}>{status?.peers_open ?? '—'}</span>
                <span style={{ color: 'var(--text-muted)' }}> / {status?.peers_total ?? '—'}</span>
              </span>
            </div>
          </div>
        )}
      </div>

      {/* Health */}
      <div className="oam-section">
        <div className="flex items-center gap-8 mb-16">
          <Activity size={16} style={{ color: healthError ? 'var(--danger)' : 'var(--success)' }} />
          <h3 className="card-title">Health</h3>
          <button className="btn-icon btn-sm" onClick={fetchHealth} title="Refresh health">
            <RefreshCw size={12} />
          </button>
        </div>

        <div className="flex items-center gap-12">
          {healthError ? (
            <>
              <XCircle size={20} style={{ color: 'var(--danger)' }} />
              <div>
                <div style={{ fontWeight: 600, color: 'var(--danger)' }}>UNHEALTHY</div>
                <div className="text-muted text-sm">{healthError}</div>
              </div>
            </>
          ) : health ? (
            <>
              <CheckCircle size={20} style={{ color: 'var(--success)' }} />
              <div>
                <div style={{ fontWeight: 600, color: 'var(--success)' }}>
                  {health.status?.toUpperCase() || 'OK'}
                </div>
              </div>
            </>
          ) : (
            <div className="flex items-center gap-8 text-muted text-sm">
              <Spinner size="sm" /> Checking...
            </div>
          )}
        </div>
      </div>

      {/* Log Level */}
      <div className="oam-section">
        <div className="flex items-center gap-8 mb-16">
          <Terminal size={16} style={{ color: 'var(--warning)' }} />
          <h3 className="card-title">Log Level</h3>
          {levelLoading && <Spinner size="sm" />}
        </div>

        <div style={{ marginBottom: 12 }}>
          <span className="text-muted text-sm">
            Current level:{' '}
            <span className="mono" style={{
              color: {
                debug: 'var(--info)',
                info: 'var(--success)',
                warn: 'var(--warning)',
                error: 'var(--danger)',
              }[currentLevel] || 'var(--text)',
              fontWeight: 600,
            }}>
              {currentLevel || 'unknown'}
            </span>
          </span>
        </div>

        <div className="log-level-btns">
          {LOG_LEVELS.map(level => (
            <button
              key={level}
              className={`log-level-btn${currentLevel === level ? ` active-${level}` : ''}`}
              onClick={() => handleSetLogLevel(level)}
              disabled={levelLoading || currentLevel === level}
              title={`Set log level to ${level}`}
            >
              {level}
            </button>
          ))}
        </div>

        <div className="text-muted text-xs mt-8">
          Runtime change — takes effect immediately. Does not persist to config file.
          Use <span className="mono">-d</span> flag at startup for debug mode with dual output.
        </div>
      </div>

      {/* Config Reload */}
      <div className="oam-section">
        <div className="flex items-center gap-8 mb-16">
          <Settings size={16} style={{ color: 'var(--accent)' }} />
          <h3 className="card-title">Configuration</h3>
        </div>

        <p className="text-muted text-sm" style={{ marginBottom: 16 }}>
          Re-reads <span className="mono">config.yaml</span> from disk. Diffs against running state:{' '}
          new peers connect, removed peers receive DPR, route/IMSI tables swap atomically.
        </p>

        <div className="flex items-center gap-12" style={{ flexWrap: 'wrap' }}>
          <button
            className="btn btn-ghost"
            onClick={handleReload}
            disabled={reloadLoading}
          >
            {reloadLoading ? <Spinner size="sm" /> : <RefreshCw size={14} />}
            Reload Config
          </button>

          {lastReload && (
            <span className="text-muted text-sm">
              Last manual reload: <span className="mono">{lastReload.toLocaleTimeString('en-US', { hour12: false })}</span>
            </span>
          )}
        </div>

        <div className="text-muted text-xs mt-12">
          Config is also hot-reloaded automatically via fsnotify when the file changes on disk.
        </div>
      </div>
    </div>
  )
}
