import React, { useState, useCallback, useMemo } from 'react'
import { Plus, Trash2, Pencil, ChevronDown, ChevronRight, ToggleLeft, ToggleRight, RefreshCw, XCircle } from 'lucide-react'
import Badge from '../components/Badge.jsx'
import Modal from '../components/Modal.jsx'
import Spinner from '../components/Spinner.jsx'
import { useToast } from '../components/Toast.jsx'
import { usePoller } from '../hooks/usePoller.js'
import { getPeers, getPeerStatus, createPeer, updatePeer, deletePeer } from '../api/client.js'

const APP_ID_NAMES = {
  0: 'Common', 1: 'NASREQ', 2: 'MobileIPv4', 3: 'BaseAcct', 4: 'DCCA',
  16777216: 'Cx', 16777217: 'Sh', 16777219: 'Wx', 16777221: 'Zh', 16777222: 'Gq', 16777224: 'Ro',
  16777236: 'Rx', 16777238: 'Gx', 16777239: 'Gy', 16777251: 'S6a',
  16777252: 'S13', 16777255: 'SLg', 16777264: 'SWm', 16777265: 'SWx', 16777267: 'S9', 16777272: 'S6b',
  16777291: 'SLh', 16777312: 'S6c', 16777313: 'SGd', 4294967295: 'Relay',
}
function appName(id) { return APP_ID_NAMES[id] || String(id) }

const EMPTY_FORM = {
  name: '', fqdn: '', address: '', port: 3868,
  transport: 'tcp', mode: 'active', realm: '', peer_group: '', weight: 1, enabled: true,
}

export default function Peers() {
  const toast = useToast()

  // Config list — slower poll, or manual refresh
  const { data: peers, error: peersErr, loading: peersLoading, refresh: refreshPeers } =
    usePoller(getPeers, 30000)

  // Live status — fast poll
  const { data: statusList, error: statusErr, refresh: refreshStatus } =
    usePoller(getPeerStatus, 5000)

  const refresh = useCallback(() => { refreshPeers(); refreshStatus() }, [refreshPeers, refreshStatus])

  // Merge config + status by name
  const merged = useMemo(() => {
    const list = Array.isArray(peers) ? peers : []
    const statusMap = {}
    if (Array.isArray(statusList)) {
      for (const s of statusList) statusMap[s.name] = s
    }
    return list.map(p => ({ ...p, status: statusMap[p.name] || null }))
  }, [peers, statusList])

  const [expanded, setExpanded] = useState({})
  const [showAdd, setShowAdd] = useState(false)
  const [editTarget, setEditTarget] = useState(null)
  const [deleteTarget, setDeleteTarget] = useState(null)
  const [actionLoading, setActionLoading] = useState({})

  const toggleExpand = useCallback((name) => {
    setExpanded(prev => ({ ...prev, [name]: !prev[name] }))
  }, [])

  const handleToggleEnabled = useCallback(async (peer) => {
    setActionLoading(prev => ({ ...prev, [peer.name]: true }))
    try {
      await updatePeer(peer.name, { enabled: !peer.enabled })
      toast.success(peer.enabled ? 'Peer disabled' : 'Peer enabled', peer.name)
      refresh()
    } catch (err) {
      toast.error('Action failed', err.message)
    } finally {
      setActionLoading(prev => ({ ...prev, [peer.name]: false }))
    }
  }, [toast, refresh])

  const handleDelete = useCallback(async () => {
    if (!deleteTarget) return
    setActionLoading(prev => ({ ...prev, [deleteTarget.name]: true }))
    try {
      await deletePeer(deleteTarget.name)
      toast.success('Peer removed', deleteTarget.name)
      setDeleteTarget(null)
      refresh()
    } catch (err) {
      toast.error('Delete failed', err.message)
    } finally {
      setActionLoading(prev => ({ ...prev, [deleteTarget?.name]: false }))
    }
  }, [deleteTarget, toast, refresh])

  if (peersLoading && !peers) {
    return (
      <div className="loading-center">
        <Spinner size="lg" />
        <span>Loading peers...</span>
      </div>
    )
  }

  if (peersErr && !peers) {
    return (
      <div className="error-state">
        <XCircle size={32} className="error-icon" />
        <div>{peersErr}</div>
        <button className="btn btn-ghost mt-12" onClick={refresh}>
          <RefreshCw size={14} /> Retry
        </button>
      </div>
    )
  }

  return (
    <div>
      <div className="page-header">
        <div>
          <div className="page-title">Peers</div>
          <div className="page-subtitle">{merged.length} peer{merged.length !== 1 ? 's' : ''} configured</div>
        </div>
        <div className="flex gap-8">
          <button className="btn btn-ghost" onClick={refresh}>
            <RefreshCw size={14} /> Refresh
          </button>
          <button className="btn btn-primary" onClick={() => setShowAdd(true)}>
            <Plus size={14} /> Add Peer
          </button>
        </div>
      </div>

      {merged.length === 0 ? (
        <div className="empty-state">
          <div className="empty-state-icon" style={{ fontSize: 32 }}>◎</div>
          <div style={{ fontWeight: 600, marginBottom: 4 }}>No peers configured</div>
          <div className="text-muted text-sm">Add a peer to start routing Diameter traffic.</div>
          <button className="btn btn-primary mt-12" onClick={() => setShowAdd(true)}>
            <Plus size={14} /> Add First Peer
          </button>
        </div>
      ) : (
        <div className="table-container">
          <table>
            <thead>
              <tr>
                <th style={{ width: 24 }}></th>
                <th>Name</th>
                <th>FQDN</th>
                <th>Transport</th>
                <th>Mode</th>
                <th>State</th>
                <th>Realm</th>
                <th>Applications</th>
                <th>In-Flight</th>
                <th>Actions</th>
              </tr>
            </thead>
            <tbody>
              {merged.map(peer => {
                const st = peer.status
                const state = st ? st.state : (peer.enabled ? 'CLOSED' : 'DISABLED')
                const actualTransport = st?.actual_transport || peer.transport
                const apps = st?.applications || []
                const appIds = st?.app_ids || []
                const inFlight = st?.in_flight ?? 0
                return (
                  <React.Fragment key={peer.name}>
                    <tr className="expandable" onClick={() => toggleExpand(peer.name)}>
                      <td style={{ color: 'var(--text-muted)', padding: '10px 8px' }}>
                        {expanded[peer.name] ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
                      </td>
                      <td style={{ fontWeight: 600 }}>{peer.name}</td>
                      <td className="mono truncate" style={{ maxWidth: 280 }}>
                        {peer.fqdn || '—'}
                      </td>
                      <td>
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                          <Badge state={actualTransport} />
                          {actualTransport !== peer.transport && (
                            <span style={{ fontSize: '0.65rem', color: 'var(--warning)' }}>
                              cfg: {peer.transport}
                            </span>
                          )}
                        </div>
                      </td>
                      <td><Badge state={peer.mode} /></td>
                      <td><Badge state={state} /></td>
                      <td className="mono truncate" style={{ maxWidth: 220, color: 'var(--text-muted)', fontSize: '0.75rem' }}>
                        {peer.realm || '—'}
                      </td>
                      <td>
                        <div className="flex gap-4" style={{ flexWrap: 'wrap' }}>
                          {apps.length > 0
                            ? apps.map(a => <span key={a} className="app-tag">{a}</span>)
                            : appIds.map(id => <span key={id} className="app-tag">{appName(id)}</span>)
                          }
                        </div>
                      </td>
                      <td className="mono" style={{ color: inFlight > 0 ? 'var(--warning)' : 'var(--text-muted)' }}>
                        {inFlight}
                      </td>
                      <td onClick={e => e.stopPropagation()}>
                        <div className="flex gap-6">
                          <button
                            className="btn-icon"
                            title={peer.enabled ? 'Disable peer' : 'Enable peer'}
                            disabled={actionLoading[peer.name]}
                            onClick={() => handleToggleEnabled(peer)}
                          >
                            {actionLoading[peer.name]
                              ? <Spinner size="sm" />
                              : peer.enabled
                                ? <ToggleRight size={15} style={{ color: 'var(--success)' }} />
                                : <ToggleLeft size={15} />
                            }
                          </button>
                          <button
                            className="btn-icon"
                            title="Edit peer"
                            disabled={actionLoading[peer.name]}
                            onClick={() => setEditTarget(peer)}
                          >
                            <Pencil size={14} />
                          </button>
                          <button
                            className="btn-icon danger"
                            title="Remove peer"
                            disabled={actionLoading[peer.name]}
                            onClick={() => setDeleteTarget(peer)}
                          >
                            <Trash2 size={14} />
                          </button>
                        </div>
                      </td>
                    </tr>
                    {expanded[peer.name] && (
                      <tr className="expanded-row">
                        <td colSpan={10}>
                          <PeerDetails peer={peer} status={st} />
                        </td>
                      </tr>
                    )}
                  </React.Fragment>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {showAdd && (
        <AddPeerModal
          onClose={() => setShowAdd(false)}
          onCreated={() => { setShowAdd(false); refresh() }}
        />
      )}

      {editTarget && (
        <EditPeerModal
          peer={editTarget}
          onClose={() => setEditTarget(null)}
          onSaved={() => { setEditTarget(null); refresh() }}
        />
      )}

      {deleteTarget && (
        <Modal title="Remove Peer" onClose={() => setDeleteTarget(null)}>
          <div className="modal-body">
            <p>Are you sure you want to remove <strong>{deleteTarget.name}</strong>?</p>
            {deleteTarget.fqdn && (
              <div className="mono" style={{
                margin: '10px 0',
                padding: '8px 12px',
                background: 'var(--bg-elevated)',
                borderRadius: 'var(--radius-sm)',
                border: '1px solid var(--border)',
                fontSize: '0.82rem',
                color: 'var(--danger)',
                wordBreak: 'break-all',
              }}>
                {deleteTarget.fqdn}
              </div>
            )}
            <p className="text-muted text-sm">
              This will send a DPR message, wait for DPA, then close the connection.
            </p>
          </div>
          <div className="modal-footer">
            <button className="btn btn-ghost" onClick={() => setDeleteTarget(null)}>Cancel</button>
            <button
              className="btn btn-danger"
              onClick={handleDelete}
              disabled={actionLoading[deleteTarget?.name]}
            >
              {actionLoading[deleteTarget?.name] ? <Spinner size="sm" /> : <Trash2 size={14} />}
              Remove
            </button>
          </div>
        </Modal>
      )}
    </div>
  )
}

function PeerDetails({ peer, status }) {
  return (
    <div className="expanded-content">
      <div className="expanded-field">
        <div className="expanded-label">Configured FQDN</div>
        <div className="expanded-value mono">{peer.fqdn || '—'}</div>
      </div>
      {status?.peer_fqdn && status.peer_fqdn !== peer.fqdn && (
        <div className="expanded-field">
          <div className="expanded-label">Peer FQDN (from CEA)</div>
          <div className="expanded-value mono" style={{ color: 'var(--accent)' }}>{status.peer_fqdn}</div>
        </div>
      )}
      <div className="expanded-field">
        <div className="expanded-label">Peer Realm</div>
        <div className="expanded-value mono">{status?.peer_realm || peer.realm || '—'}</div>
      </div>
      <div className="expanded-field">
        <div className="expanded-label">Address</div>
        <div className="expanded-value mono">{peer.address || '—'}:{peer.port || '—'}</div>
      </div>
      {status?.remote_addr && (
        <div className="expanded-field">
          <div className="expanded-label">Remote Addr (live)</div>
          <div className="expanded-value mono" style={{ color: 'var(--accent)' }}>{status.remote_addr}</div>
        </div>
      )}
      <div className="expanded-field">
        <div className="expanded-label">Configured Transport</div>
        <div className="expanded-value"><Badge state={peer.transport} /></div>
      </div>
      {status && status.actual_transport !== peer.transport && (
        <div className="expanded-field">
          <div className="expanded-label">Actual Transport</div>
          <div className="expanded-value">
            <Badge state={status.actual_transport} />
            <span style={{ marginLeft: 6, fontSize: '0.75rem', color: 'var(--warning)' }}>
              ⚠ differs from configured
            </span>
          </div>
        </div>
      )}
      <div className="expanded-field">
        <div className="expanded-label">Peer Group</div>
        <div className="expanded-value">{peer.peer_group || '—'}</div>
      </div>
      <div className="expanded-field">
        <div className="expanded-label">Weight</div>
        <div className="expanded-value">{peer.weight ?? 1}</div>
      </div>
      {status?.connected_at && (
        <div className="expanded-field">
          <div className="expanded-label">Connected At</div>
          <div className="expanded-value mono" style={{ fontSize: '0.8rem' }}>
            {new Date(status.connected_at).toLocaleString()}
          </div>
        </div>
      )}
      <div className="expanded-field">
        <div className="expanded-label">Application IDs</div>
        <div className="expanded-value">
          {status?.app_ids?.length > 0
            ? status.app_ids.map(id => `${id} (${appName(id)})`).join(', ')
            : '—'}
        </div>
      </div>
    </div>
  )
}

function EditPeerModal({ peer, onClose, onSaved }) {
  const toast = useToast()
  const [form, setForm] = useState({
    fqdn:       peer.fqdn       || '',
    address:    peer.address    || '',
    port:       peer.port       || 3868,
    transport:  peer.transport  || 'tcp',
    mode:       peer.mode       || 'active',
    realm:      peer.realm      || '',
    peer_group: peer.peer_group || '',
    weight:     peer.weight     ?? 1,
    enabled:    peer.enabled    ?? true,
  })
  const [submitting, setSubmitting] = useState(false)

  const set = useCallback((field, value) => {
    setForm(prev => ({ ...prev, [field]: value }))
  }, [])

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault()
    if (!form.fqdn.trim() || !form.address.trim()) {
      toast.error('Validation', 'FQDN and address are required.')
      return
    }
    setSubmitting(true)
    try {
      await updatePeer(peer.name, { ...form, port: Number(form.port), weight: Number(form.weight) })
      toast.success('Peer updated', peer.name)
      onSaved()
    } catch (err) {
      toast.error('Update failed', err.message)
    } finally {
      setSubmitting(false)
    }
  }, [form, peer.name, toast, onSaved])

  return (
    <Modal title={`Edit Peer — ${peer.name}`} onClose={onClose} size="lg">
      <form onSubmit={handleSubmit}>
        <div className="modal-body">
          <div className="form-group">
            <label className="form-label">Name</label>
            <input className="input mono" value={peer.name} disabled style={{ opacity: 0.5 }} />
            <span className="form-hint">Name cannot be changed</span>
          </div>
          <div className="form-row">
            <div className="form-group">
              <label className="form-label">Peer Group</label>
              <input className="input" placeholder="hss_group" value={form.peer_group} onChange={e => set('peer_group', e.target.value)} />
            </div>
            <div className="form-group">
              <label className="form-label">Weight</label>
              <input className="input" type="number" min={1} max={100} value={form.weight} onChange={e => set('weight', e.target.value)} />
            </div>
          </div>
          <div className="form-group">
            <label className="form-label">FQDN *</label>
            <input className="input mono" placeholder="hss01.epc.mnc435.mcc311.3gppnetwork.org" value={form.fqdn} onChange={e => set('fqdn', e.target.value)} required />
          </div>
          <div className="form-row">
            <div className="form-group">
              <label className="form-label">Address *</label>
              <input className="input mono" placeholder="10.0.0.10 or hostname" value={form.address} onChange={e => set('address', e.target.value)} required />
              <span className="form-hint">IP or FQDN — resolved via DNS on connect</span>
            </div>
            <div className="form-group">
              <label className="form-label">Port</label>
              <input className="input mono" type="number" min={1} max={65535} value={form.port} onChange={e => set('port', e.target.value)} />
            </div>
          </div>
          <div className="form-group">
            <label className="form-label">Realm</label>
            <input className="input mono" placeholder="epc.mnc435.mcc311.3gppnetwork.org" value={form.realm} onChange={e => set('realm', e.target.value)} />
          </div>
          <div className="form-row-3">
            <div className="form-group">
              <label className="form-label">Transport</label>
              <select className="select" value={form.transport} onChange={e => set('transport', e.target.value)}>
                <option value="tcp">tcp</option>
                <option value="sctp">sctp</option>
                <option value="tcp+tls">tcp+tls</option>
                <option value="sctp+tls">sctp+tls</option>
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">Mode</label>
              <select className="select" value={form.mode} onChange={e => set('mode', e.target.value)}>
                <option value="active">active (we dial)</option>
                <option value="passive">passive (they dial)</option>
              </select>
            </div>
          </div>
          <label className="checkbox-wrap">
            <input type="checkbox" checked={form.enabled} onChange={e => set('enabled', e.target.checked)} />
            <span>Enabled</span>
          </label>
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={submitting}>
            {submitting ? <Spinner size="sm" /> : <Pencil size={14} />}
            Save Changes
          </button>
        </div>
      </form>
    </Modal>
  )
}

function AddPeerModal({ onClose, onCreated }) {
  const toast = useToast()
  const [form, setForm] = useState(EMPTY_FORM)
  const [submitting, setSubmitting] = useState(false)

  const set = useCallback((field, value) => {
    setForm(prev => ({ ...prev, [field]: value }))
  }, [])

  const handleSubmit = useCallback(async (e) => {
    e.preventDefault()
    if (!form.name.trim() || !form.fqdn.trim() || !form.address.trim()) {
      toast.error('Validation', 'Name, FQDN, and address are required.')
      return
    }
    setSubmitting(true)
    try {
      await createPeer({ ...form, port: Number(form.port), weight: Number(form.weight) })
      toast.success('Peer added', form.name)
      onCreated()
    } catch (err) {
      toast.error('Create failed', err.message)
    } finally {
      setSubmitting(false)
    }
  }, [form, toast, onCreated])

  return (
    <Modal title="Add Peer" onClose={onClose} size="lg">
      <form onSubmit={handleSubmit}>
        <div className="modal-body">
          <div className="form-row">
            <div className="form-group">
              <label className="form-label">Name *</label>
              <input className="input" placeholder="hss01" value={form.name} onChange={e => set('name', e.target.value)} required />
            </div>
            <div className="form-group">
              <label className="form-label">Peer Group</label>
              <input className="input" placeholder="hss_group" value={form.peer_group} onChange={e => set('peer_group', e.target.value)} />
            </div>
          </div>
          <div className="form-group">
            <label className="form-label">FQDN *</label>
            <input className="input mono" placeholder="hss01.epc.mnc435.mcc311.3gppnetwork.org" value={form.fqdn} onChange={e => set('fqdn', e.target.value)} required />
          </div>
          <div className="form-row">
            <div className="form-group">
              <label className="form-label">Address *</label>
              <input className="input mono" placeholder="10.90.250.190 or hostname" value={form.address} onChange={e => set('address', e.target.value)} required />
              <span className="form-hint">IP or FQDN — resolved via DNS on connect</span>
            </div>
            <div className="form-group">
              <label className="form-label">Port</label>
              <input className="input mono" type="number" min={1} max={65535} value={form.port} onChange={e => set('port', e.target.value)} />
            </div>
          </div>
          <div className="form-group">
            <label className="form-label">Realm</label>
            <input className="input mono" placeholder="epc.mnc435.mcc311.3gppnetwork.org" value={form.realm} onChange={e => set('realm', e.target.value)} />
          </div>
          <div className="form-row-3">
            <div className="form-group">
              <label className="form-label">Transport</label>
              <select className="select" value={form.transport} onChange={e => set('transport', e.target.value)}>
                <option value="tcp">tcp</option>
                <option value="sctp">sctp</option>
                <option value="tcp+tls">tcp+tls</option>
                <option value="sctp+tls">sctp+tls</option>
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">Mode</label>
              <select className="select" value={form.mode} onChange={e => set('mode', e.target.value)}>
                <option value="active">active (we dial)</option>
                <option value="passive">passive (they dial)</option>
              </select>
            </div>
            <div className="form-group">
              <label className="form-label">Weight</label>
              <input className="input" type="number" min={1} max={100} value={form.weight} onChange={e => set('weight', e.target.value)} />
            </div>
          </div>
          <label className="checkbox-wrap">
            <input type="checkbox" checked={form.enabled} onChange={e => set('enabled', e.target.checked)} />
            <span>Enabled</span>
          </label>
        </div>
        <div className="modal-footer">
          <button type="button" className="btn btn-ghost" onClick={onClose}>Cancel</button>
          <button type="submit" className="btn btn-primary" disabled={submitting}>
            {submitting ? <Spinner size="sm" /> : <Plus size={14} />}
            Add Peer
          </button>
        </div>
      </form>
    </Modal>
  )
}
