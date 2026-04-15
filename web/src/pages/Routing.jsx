import React, { useState, useCallback } from 'react'
import { Plus, Trash2, Edit3, RefreshCw, XCircle } from 'lucide-react'
import Badge from '../components/Badge.jsx'
import Spinner from '../components/Spinner.jsx'
import Modal from '../components/Modal.jsx'
import { useToast } from '../components/Toast.jsx'
import { usePoller } from '../hooks/usePoller.js'
import {
  getRoutes, createRoute, updateRoute, deleteRoute,
  getIMSIRoutes, createIMSIRoute, updateIMSIRoute, deleteIMSIRoute,
  getLBGroups, createLBGroup, updateLBGroup, deleteLBGroup,
} from '../api/client.js'

const APP_ID_NAMES = {
  0: 'any',
  1: 'NASREQ',
  2: 'MobileIPv4',
  3: 'BaseAcct',
  4: 'CreditControl',
  16777216: 'Cx/Dx',
  16777217: 'Sh',
  16777219: 'Wx',
  16777221: 'Zh',
  16777236: 'Rx',
  16777238: 'Gx',
  16777239: 'Gy',
  16777251: 'S6a',
  16777252: 'S13',
  16777264: 'SWm',
  16777265: 'SWx',
  16777267: 'S9',
  16777272: 'S6b',
  16777291: 'SLh',
  16777312: 'S6c',
  16777313: 'SGd',
  4294967295: 'Relay',
}

function appLabel(id) {
  const name = APP_ID_NAMES[id]
  if (id === 0) return 'any (wildcard)'
  return name ? `${name} (${id})` : String(id)
}

export default function Routing() {
  const [tab, setTab] = useState('routes')

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Routing</div>
          <div className="page-subtitle">Route rules, IMSI prefix routing, and LB groups</div>
        </div>
      </div>

      <div className="tabs">
        {[
          { id: 'routes', label: 'Route Rules' },
          { id: 'imsi', label: 'IMSI Routes' },
          { id: 'groups', label: 'LB Groups' },
        ].map(t => (
          <button
            key={t.id}
            className={`tab-btn${tab === t.id ? ' active' : ''}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'routes' && <RouteRulesTab />}
      {tab === 'imsi' && <IMSIRoutesTab />}
      {tab === 'groups' && <PeerGroupsTab />}
    </div>
  )
}

/* ============================================================
   Route Rules
   ============================================================ */
const ROUTE_DEFAULTS = {
  priority: 10,
  dest_host: '',
  dest_realm: '',
  app_id: 0,
  lb_group: '',
  action: 'route',
  enabled: true,
}

function RouteRulesTab() {
  const toast = useToast()
  const { data: routes, error, loading, refresh } = usePoller(getRoutes)
  const { data: lbGroups } = usePoller(getLBGroups, 15000)
  const [showModal, setShowModal] = useState(false)
  const [editTarget, setEditTarget] = useState(null)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [actionLoading, setActionLoading] = useState({})

  const handleToggle = useCallback(async (rule) => {
    const idx = rule.index
    setActionLoading(prev => ({ ...prev, [idx]: true }))
    try {
      const { index: _i, ...ruleBody } = rule
      await updateRoute(idx, { ...ruleBody, enabled: !rule.enabled })
      toast.success(rule.enabled ? 'Rule disabled' : 'Rule enabled')
      refresh()
    } catch (err) {
      toast.error('Action failed', err.message)
    } finally {
      setActionLoading(prev => ({ ...prev, [idx]: false }))
    }
  }, [toast, refresh])

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return
    const idx = deleteTarget.index
    setActionLoading(prev => ({ ...prev, [idx]: true }))
    try {
      await deleteRoute(idx)
      toast.success('Route rule deleted')
      setDeleteTarget(null)
      refresh()
    } catch (err) {
      toast.error('Delete failed', err.message)
    } finally {
      setActionLoading(prev => { const n = { ...prev }; delete n[idx]; return n })
    }
  }, [deleteTarget, toast, refresh])

  const sorted = Array.isArray(routes)
    ? [...routes].sort((a, b) => (a.priority ?? 99) - (b.priority ?? 99))
    : []

  if (loading) return <div className="loading-center"><Spinner size="md" /></div>
  if (error && !routes) {
    return (
      <div className="error-state">
        <XCircle size={28} className="error-icon" />
        <div>{error}</div>
        <button className="btn btn-ghost mt-12" onClick={refresh}><RefreshCw size={13} /> Retry</button>
      </div>
    )
  }

  return (
    <div>
      <div className="flex justify-between mb-12">
        <span className="text-muted text-sm">{sorted.length} rule{sorted.length !== 1 ? 's' : ''}</span>
        <div className="flex gap-8">
          <button className="btn btn-ghost btn-sm" onClick={refresh}><RefreshCw size={12} /></button>
          <button className="btn btn-primary btn-sm" onClick={() => { setEditTarget(null); setShowModal(true) }}>
            <Plus size={12} /> Add Rule
          </button>
        </div>
      </div>

      {sorted.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon" style={{ fontSize: 28 }}>◎</div>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>No route rules configured</div>
          <div className="text-muted text-sm">Traffic will be dropped without route rules.</div>
          <button className="btn btn-primary btn-sm mt-12" onClick={() => { setEditTarget(null); setShowModal(true) }}>
            <Plus size={12} /> Add First Rule
          </button>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Priority</th>
                <th>Dest Realm</th>
                <th>Dest Host</th>
                <th>App ID</th>
                <th>Action</th>
                <th>Dest Host / LB Group</th>
                <th>Enabled</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((rule) => {
                const idx = rule.index
                return (
                  <tr key={idx}>
                    <td className="mono" style={{ fontWeight: 700 }}>{rule.priority ?? '—'}</td>
                    <td className="mono" style={{ fontSize: '0.8rem', color: rule.dest_realm ? 'var(--text)' : 'var(--text-muted)' }}>
                      {rule.dest_realm || '*'}
                    </td>
                    <td className="mono" style={{ fontSize: '0.75rem', color: rule.dest_host ? 'var(--text)' : 'var(--text-muted)' }}>
                      {rule.dest_host || '*'}
                    </td>
                    <td className="mono" style={{ fontSize: '0.75rem' }}>{appLabel(rule.app_id ?? 0)}</td>
                    <td><Badge state={rule.action} /></td>
                    <td style={{ fontFamily: 'var(--font-mono)', fontSize: '0.8rem' }}>
                      {rule.dest_host || rule.lb_group || <span className="text-muted">—</span>}
                    </td>
                    <td>
                      <label className="toggle">
                        <input
                          type="checkbox"
                          checked={rule.enabled ?? true}
                          disabled={actionLoading[idx]}
                          onChange={() => handleToggle(rule)}
                        />
                        <span className="toggle-slider" />
                      </label>
                    </td>
                    <td>
                      <div className="flex gap-6">
                        <button className="btn-icon" title="Edit"
                          onClick={() => { setEditTarget(rule); setShowModal(true) }}>
                          <Edit3 size={13} />
                        </button>
                        <button className="btn-icon danger" title="Delete"
                          disabled={actionLoading[idx]}
                          onClick={() => setDeleteTarget(rule)}>
                          <Trash2 size={13} />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {showModal && (
        <RouteModal
          initial={editTarget}
          lbGroups={Array.isArray(lbGroups) ? lbGroups : []}
          onClose={() => setShowModal(false)}
          onSaved={() => { setShowModal(false); refresh() }}
        />
      )}

      {deleteTarget && (
        <ConfirmDeleteModal
          label={`route rule (priority ${deleteTarget.priority})`}
          onClose={() => setDeleteTarget(null)}
          onConfirm={handleDelete}
          loading={!!actionLoading[deleteTarget?.index]}
        />
      )}
    </div>
  )
}

function RouteModal({ initial, lbGroups, onClose, onSaved }) {
  const toast = useToast()
  const [form, setForm] = useState(initial ? { ...ROUTE_DEFAULTS, ...initial } : { ...ROUTE_DEFAULTS })
  const [submitting, setSubmitting] = useState(false)

  const set = useCallback((k, v) => setForm(prev => ({ ...prev, [k]: v })), [])

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault()
    setSubmitting(true)
    try {
      const { index: _i, ...formBody } = form
      const payload = { ...formBody, priority: Number(form.priority), app_id: Number(form.app_id) }
      if (initial?.index != null) {
        await updateRoute(initial.index, payload)
        toast.success('Route rule updated')
      } else {
        await createRoute(payload)
        toast.success('Route rule created')
      }
      onSaved()
    } catch (err) {
      toast.error('Save failed', err.message)
    } finally {
      setSubmitting(false)
    }
  }, [form, initial, toast, onSaved])

  return (
    <Modal title={initial ? 'Edit Route Rule' : 'Add Route Rule'} onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="modal-body">
          <div className="form-row">
            <div className="form-group">
              <label className="form-label">Priority</label>
              <input className="input mono" type="number" min={1}
                value={form.priority} onChange={e => set('priority', e.target.value)} />
              <span className="form-hint">Lower = evaluated first</span>
            </div>
            <div className="form-group">
              <label className="form-label">Action</label>
              <select className="select" value={form.action} onChange={e => set('action', e.target.value)}>
                <option value="route">route</option>
                <option value="reject">reject</option>
                <option value="drop">drop</option>
              </select>
            </div>
          </div>

          <div className="form-group">
            <label className="form-label">Dest Realm</label>
            <input className="input mono"
              placeholder="epc.mnc001.mcc001.3gppnetwork.org (blank = wildcard)"
              value={form.dest_realm}
              onChange={e => set('dest_realm', e.target.value)} />
          </div>

          <div className="form-group">
            <label className="form-label">Dest Host</label>
            <input className="input mono"
              placeholder="specific peer FQDN (blank = any)"
              value={form.dest_host}
              onChange={e => set('dest_host', e.target.value)} />
          </div>

          <div className="form-row">
            <div className="form-group">
              <label className="form-label">App ID</label>
              <select className="select" value={form.app_id} onChange={e => set('app_id', e.target.value)}>
                <option value={0}>0 — any (wildcard)</option>
                <option value={1}>1 — NASREQ</option>
                <option value={3}>3 — Base Accounting</option>
                <option value={4}>4 — Credit-Control / Gy/Ro (RFC 4006)</option>
                <option value={16777216}>16777216 — Cx/Dx</option>
                <option value={16777217}>16777217 — Sh</option>
                <option value={16777219}>16777219 — Wx</option>
                <option value={16777221}>16777221 — Zh</option>
                <option value={16777236}>16777236 — Rx</option>
                <option value={16777238}>16777238 — Gx</option>
                <option value={16777239}>16777239 — Gy (3GPP vendor-specific)</option>
                <option value={16777251}>16777251 — S6a</option>
                <option value={16777252}>16777252 — S13</option>
                <option value={16777264}>16777264 — SWm</option>
                <option value={16777265}>16777265 — SWx</option>
                <option value={16777267}>16777267 — S9</option>
                <option value={16777272}>16777272 — S6b</option>
                <option value={16777291}>16777291 — SLh</option>
                <option value={16777312}>16777312 — S6c</option>
                <option value={16777313}>16777313 — SGd</option>
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">LB Group</label>
              <LBGroupSelect
                value={form.lb_group}
                onChange={v => set('lb_group', v)}
                lbGroups={lbGroups}
              />
            </div>
          </div>

          <label className="checkbox-wrap">
            <input type="checkbox"
              checked={form.enabled ?? true}
              onChange={e => set('enabled', e.target.checked)} />
            <span>Enabled</span>
          </label>
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={submitting}>
            {submitting ? <Spinner size="sm" /> : null}
            {initial ? 'Save Changes' : 'Add Rule'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

/* ============================================================
   IMSI Routes
   ============================================================ */
const IMSI_DEFAULTS = {
  prefix: '',
  dest_realm: '',
  lb_group: '',
  priority: 10,
}

function IMSIRoutesTab() {
  const toast = useToast()
  const { data: routes, error, loading, refresh } = usePoller(getIMSIRoutes)
  const { data: lbGroups } = usePoller(getLBGroups, 15000)
  const [showModal, setShowModal] = useState(false)
  const [editTarget, setEditTarget] = useState(null)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [actionLoading, setActionLoading] = useState({})

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return
    const idx = deleteTarget.index
    setActionLoading(prev => ({ ...prev, [idx]: true }))
    try {
      await deleteIMSIRoute(idx)
      toast.success('IMSI route deleted')
      setDeleteTarget(null)
      refresh()
    } catch (err) {
      toast.error('Delete failed', err.message)
    } finally {
      setActionLoading(prev => { const n = { ...prev }; delete n[idx]; return n })
    }
  }, [deleteTarget, toast, refresh])

  const sorted = Array.isArray(routes)
    ? [...routes].sort((a, b) => {
        // Longest prefix first (BGP-style), then by priority
        const lenDiff = (b.prefix?.length ?? 0) - (a.prefix?.length ?? 0)
        return lenDiff !== 0 ? lenDiff : (a.priority ?? 0) - (b.priority ?? 0)
      })
    : []

  if (loading) return <div className="loading-center"><Spinner size="md" /></div>
  if (error && !routes) {
    return (
      <div className="error-state">
        <XCircle size={28} className="error-icon" />
        <div>{error}</div>
        <button className="btn btn-ghost mt-12" onClick={refresh}><RefreshCw size={13} /> Retry</button>
      </div>
    )
  }

  return (
    <div>
      <div className="flex justify-between mb-12">
        <span className="text-muted text-sm">{sorted.length} IMSI route{sorted.length !== 1 ? 's' : ''}</span>
        <div className="flex gap-8">
          <button className="btn btn-ghost btn-sm" onClick={refresh}><RefreshCw size={12} /></button>
          <button className="btn btn-primary btn-sm" onClick={() => { setEditTarget(null); setShowModal(true) }}>
            <Plus size={12} /> Add Route
          </button>
        </div>
      </div>

      {sorted.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon" style={{ fontSize: 28 }}>◎</div>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>No IMSI prefix routes configured</div>
          <div className="text-muted text-sm">IMSI routes enable roaming partner separation by MCC/MNC prefix.</div>
          <button className="btn btn-primary btn-sm mt-12" onClick={() => { setEditTarget(null); setShowModal(true) }}>
            <Plus size={12} /> Add Route
          </button>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Prefix</th>
                <th>MCC / MNC</th>
                <th>Dest Realm</th>
                <th>LB Group</th>
                <th>Priority</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((route) => {
                const idx = route.index
                const p = route.prefix || ''
                const mcc = p.slice(0, 3)
                const mnc = p.slice(3)
                return (
                  <tr key={idx}>
                    <td className="mono" style={{ fontWeight: 700, letterSpacing: '0.08em', fontSize: '0.9rem' }}>
                      {route.prefix || '—'}
                    </td>
                    <td className="mono" style={{ color: 'var(--text-muted)', fontSize: '0.75rem' }}>
                      {mcc && mnc ? `MCC ${mcc} / MNC ${mnc}` : '—'}
                    </td>
                    <td className="mono" style={{ fontSize: '0.8rem' }}>{route.dest_realm || '—'}</td>
                    <td style={{ fontFamily: 'var(--font-mono)', fontSize: '0.8rem' }}>{route.lb_group || <span className="text-muted">—</span>}</td>
                    <td className="mono">{route.priority ?? '—'}</td>
                    <td>
                      <div className="flex gap-6">
                        <button className="btn-icon" title="Edit"
                          onClick={() => { setEditTarget(route); setShowModal(true) }}>
                          <Edit3 size={13} />
                        </button>
                        <button className="btn-icon danger" title="Delete"
                          disabled={!!actionLoading[idx]}
                          onClick={() => setDeleteTarget(route)}>
                          <Trash2 size={13} />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {showModal && (
        <IMSIModal
          initial={editTarget}
          lbGroups={Array.isArray(lbGroups) ? lbGroups : []}
          onClose={() => setShowModal(false)}
          onSaved={() => { setShowModal(false); refresh() }}
        />
      )}

      {deleteTarget && (
        <ConfirmDeleteModal
          label={`IMSI route prefix ${deleteTarget.prefix}`}
          onClose={() => setDeleteTarget(null)}
          onConfirm={handleDelete}
          loading={!!actionLoading[deleteTarget?.index]}
        />
      )}
    </div>
  )
}

function IMSIModal({ initial, lbGroups, onClose, onSaved }) {
  const toast = useToast()
  const [form, setForm] = useState(initial ? { ...IMSI_DEFAULTS, ...initial } : { ...IMSI_DEFAULTS })
  const [submitting, setSubmitting] = useState(false)

  const set = useCallback((k, v) => setForm(prev => ({ ...prev, [k]: v })), [])

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault()
    if (!form.prefix || !/^\d{5,6}$/.test(form.prefix)) {
      toast.error('Validation', 'Prefix must be exactly 5 or 6 digits (MCC+MNC).')
      return
    }
    if (!form.dest_realm) {
      toast.error('Validation', 'Dest realm is required.')
      return
    }
    setSubmitting(true)
    try {
      const payload = { ...form, priority: Number(form.priority) }
      delete payload.index
      if (!payload.lb_group) delete payload.lb_group
      if (initial?.index != null) {
        await updateIMSIRoute(initial.index, payload)
        toast.success('IMSI route updated')
      } else {
        await createIMSIRoute(payload)
        toast.success('IMSI route created')
      }
      onSaved()
    } catch (err) {
      toast.error('Save failed', err.message)
    } finally {
      setSubmitting(false)
    }
  }, [form, initial, toast, onSaved])

  return (
    <Modal title={initial ? 'Edit IMSI Route' : 'Add IMSI Route'} onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="modal-body">
          <div className="form-group">
            <label className="form-label">IMSI Prefix *</label>
            <input className="input mono"
              placeholder="311435 (5 or 6 digits, MCC+MNC)"
              value={form.prefix}
              onChange={e => set('prefix', e.target.value)}
              maxLength={6}
              required />
            <span className="form-hint">
              First 5-6 digits of IMSI. MCC=3 digits + MNC=2 or 3 digits. Longest match wins.
            </span>
          </div>
          <div className="form-group">
            <label className="form-label">Dest Realm *</label>
            <input className="input mono"
              placeholder="epc.mnc435.mcc311.3gppnetwork.org"
              value={form.dest_realm}
              onChange={e => set('dest_realm', e.target.value)}
              required />
          </div>
          <div className="form-row">
            <div className="form-group">
              <label className="form-label">LB Group</label>
              <LBGroupSelect
                value={form.lb_group}
                onChange={v => set('lb_group', v)}
                lbGroups={lbGroups}
              />
            </div>
            <div className="form-group">
              <label className="form-label">Priority</label>
              <input className="input mono" type="number" min={1}
                value={form.priority}
                onChange={e => set('priority', e.target.value)} />
            </div>
          </div>
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={submitting}>
            {submitting ? <Spinner size="sm" /> : null}
            {initial ? 'Save Changes' : 'Add Route'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

/* ============================================================
   LB Groups
   ============================================================ */
const GROUP_DEFAULTS = {
  name: '',
  lb_policy: 'round_robin',
}

function PeerGroupsTab() {
  const toast = useToast()
  const { data: groups, error, loading, refresh } = usePoller(getLBGroups)
  const [showModal, setShowModal] = useState(false)
  const [editTarget, setEditTarget] = useState(null)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [actionLoading, setActionLoading] = useState({})

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return
    const name = deleteTarget.name
    setActionLoading(prev => ({ ...prev, [name]: true }))
    try {
      await deleteLBGroup(name)
      toast.success('LB group deleted', name)
      setDeleteTarget(null)
      refresh()
    } catch (err) {
      toast.error('Delete failed', err.message)
    } finally {
      setActionLoading(prev => { const n = { ...prev }; delete n[name]; return n })
    }
  }, [deleteTarget, toast, refresh])

  const list = Array.isArray(groups) ? groups : []

  if (loading) return <div className="loading-center"><Spinner size="md" /></div>
  if (error && !groups) {
    return (
      <div className="error-state">
        <XCircle size={28} className="error-icon" />
        <div>{error}</div>
        <button className="btn btn-ghost mt-12" onClick={refresh}><RefreshCw size={13} /> Retry</button>
      </div>
    )
  }

  return (
    <div>
      <div className="flex justify-between mb-12">
        <span className="text-muted text-sm">{list.length} group{list.length !== 1 ? 's' : ''}</span>
        <div className="flex gap-8">
          <button className="btn btn-ghost btn-sm" onClick={refresh}><RefreshCw size={12} /></button>
          <button className="btn btn-primary btn-sm" onClick={() => { setEditTarget(null); setShowModal(true) }}>
            <Plus size={12} /> Add Group
          </button>
        </div>
      </div>

      {list.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon" style={{ fontSize: 28 }}>◎</div>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>No LB groups configured</div>
          <div className="text-muted text-sm">LB groups aggregate peers for load balancing and failover.</div>
          <button className="btn btn-primary btn-sm mt-12" onClick={() => { setEditTarget(null); setShowModal(true) }}>
            <Plus size={12} /> Add Group
          </button>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th>Name</th>
                <th>LB Policy</th>
                <th>Members</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {list.map(grp => (
                <tr key={grp.name}>
                  <td style={{ fontWeight: 600, fontFamily: 'var(--font-mono)' }}>{grp.name}</td>
                  <td>
                    <span className="badge badge-info">{grp.lb_policy || 'round_robin'}</span>
                  </td>
                  <td className="mono text-muted">{grp.member_count ?? grp.members?.length ?? '—'}</td>
                  <td>
                    <div className="flex gap-6">
                      <button className="btn-icon" title="Edit"
                        onClick={() => { setEditTarget(grp); setShowModal(true) }}>
                        <Edit3 size={13} />
                      </button>
                      <button className="btn-icon danger" title="Delete"
                        disabled={!!actionLoading[grp.name]}
                        onClick={() => setDeleteTarget(grp)}>
                        <Trash2 size={13} />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showModal && (
        <LBGroupModal
          initial={editTarget}
          onClose={() => setShowModal(false)}
          onSaved={() => { setShowModal(false); refresh() }}
        />
      )}

      {deleteTarget && (
        <ConfirmDeleteModal
          label={`LB group "${deleteTarget.name}"`}
          onClose={() => setDeleteTarget(null)}
          onConfirm={handleDelete}
          loading={!!actionLoading[deleteTarget?.name]}
        />
      )}
    </div>
  )
}

function LBGroupModal({ initial, onClose, onSaved }) {
  const toast = useToast()
  const [form, setForm] = useState(initial ? { ...GROUP_DEFAULTS, ...initial } : { ...GROUP_DEFAULTS })
  const [submitting, setSubmitting] = useState(false)

  const set = useCallback((k, v) => setForm(prev => ({ ...prev, [k]: v })), [])

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault()
    if (!form.name.trim()) {
      toast.error('Validation', 'Name is required.')
      return
    }
    setSubmitting(true)
    try {
      if (initial && initial.name) {
        await updateLBGroup(initial.name, { lb_policy: form.lb_policy })
        toast.success('LB group updated', form.name)
      } else {
        await createLBGroup(form)
        toast.success('LB group created', form.name)
      }
      onSaved()
    } catch (err) {
      toast.error('Save failed', err.message)
    } finally {
      setSubmitting(false)
    }
  }, [form, initial, toast, onSaved])

  return (
    <Modal title={initial ? 'Edit LB Group' : 'Add LB Group'} onClose={onClose}>
      <form onSubmit={handleSubmit}>
        <div className="modal-body">
          <div className="form-group">
            <label className="form-label">Name *</label>
            <input className="input"
              placeholder="pcrf_group"
              value={form.name}
              onChange={e => set('name', e.target.value)}
              disabled={!!initial}
              required />
          </div>
          <div className="form-group">
            <label className="form-label">Load Balancing Policy</label>
            <select className="select" value={form.lb_policy} onChange={e => set('lb_policy', e.target.value)}>
              <option value="round_robin">round_robin — sequential rotation</option>
              <option value="least_conn">least_conn — fewest in-flight</option>
            </select>
          </div>
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={submitting}>
            {submitting ? <Spinner size="sm" /> : null}
            {initial ? 'Save Changes' : 'Add Group'}
          </button>
        </div>
      </form>
    </Modal>
  )
}

/* ============================================================
   Shared helpers
   ============================================================ */
function LBGroupSelect({ value, onChange, lbGroups }) {
  const groups = Array.isArray(lbGroups) ? lbGroups : []
  const valueInList = groups.some(g => g.name === value)
  return (
    <select className="select" value={value} onChange={e => onChange(e.target.value)}>
      <option value="">-- none --</option>
      {groups.map(g => (
        <option key={g.name} value={g.name}>{g.name}</option>
      ))}
      {value && !valueInList && (
        <option value={value}>{value} (not in list)</option>
      )}
    </select>
  )
}

function ConfirmDeleteModal({ label, onClose, onConfirm, loading }) {
  return (
    <Modal title="Confirm Delete" onClose={onClose}>
      <div className="modal-body">
        <p>Delete {label}?</p>
        <p className="text-muted text-sm" style={{ marginTop: 6 }}>This action cannot be undone.</p>
      </div>
      <div className="modal-footer">
        <button className="btn btn-ghost" onClick={onClose}>Cancel</button>
        <button className="btn btn-danger" onClick={onConfirm} disabled={loading}>
          {loading ? <Spinner size="sm" /> : <Trash2 size={13} />}
          Delete
        </button>
      </div>
    </Modal>
  )
}
