const BASE = '/api/v1'

async function request(method, path, body) {
  const opts = {
    method,
    headers: {},
  }
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json'
    opts.body = JSON.stringify(body)
  }
  const res = await fetch(`${BASE}${path}`, opts)
  if (res.status === 204) return null
  if (!res.ok) {
    let msg = `HTTP ${res.status}`
    try {
      const data = await res.json()
      if (Array.isArray(data.errors) && data.errors.length > 0) {
        msg = data.errors.map(e => `${e.path ?? ''}: ${e.message ?? e}`).join('; ')
      } else {
        msg = data.detail || data.message || data.error || msg
      }
    } catch {
      // ignore parse error
    }
    throw new Error(msg)
  }
  return res.json()
}

// ---------- Peers ----------
export const getPeers = () => request('GET', '/peers')
export const getPeer = (name) => request('GET', `/peers/${encodeURIComponent(name)}`)
export const getPeerStatus = () => request('GET', '/peers/status')
export const createPeer = (data) => request('POST', '/peers', data)
export const updatePeer = (name, data) => request('PATCH', `/peers/${encodeURIComponent(name)}`, data)
export const deletePeer = (name) => request('DELETE', `/peers/${encodeURIComponent(name)}`)

// ---------- LB Groups ----------
export const getLBGroups = () => request('GET', '/lb-groups')
export const createLBGroup = (data) => request('POST', '/lb-groups', data)
export const updateLBGroup = (name, data) => request('PATCH', `/lb-groups/${encodeURIComponent(name)}`, data)
export const deleteLBGroup = (name) => request('DELETE', `/lb-groups/${encodeURIComponent(name)}`)

// ---------- Route Rules ----------
export const getRoutes = () => request('GET', '/routes')
export const createRoute = (data) => request('POST', '/routes', data)
export const updateRoute = (id, data) => request('PUT', `/routes/${encodeURIComponent(id)}`, data)
export const deleteRoute = (id) => request('DELETE', `/routes/${encodeURIComponent(id)}`)

// ---------- IMSI Routes ----------
export const getIMSIRoutes = () => request('GET', '/imsi-routes')
export const createIMSIRoute = (data) => request('POST', '/imsi-routes', data)
export const updateIMSIRoute = (id, data) => request('PUT', `/imsi-routes/${encodeURIComponent(id)}`, data)
export const deleteIMSIRoute = (id) => request('DELETE', `/imsi-routes/${encodeURIComponent(id)}`)

// ---------- OAM / Status ----------
export const getStatus = () => request('GET', '/oam/status')
export const reload = () => request('POST', '/oam/reload')
export const setLogLevel = (level) => request('POST', '/oam/log-level', { level })
export const getMetrics = () => request('GET', '/oam/metrics')
export const getRecentMessages = () => request('GET', '/oam/recent-messages')

// ---------- Health ----------
export async function getHealth() {
  const res = await fetch('/health')
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.json()
}

// ---------- Raw Prometheus ----------
export async function getPrometheusText() {
  const res = await fetch('/metrics')
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return res.text()
}

// ---------- Prometheus text parser ----------
export function parsePrometheusText(text) {
  const metrics = {}
  if (!text) return metrics

  const lines = text.split('\n')
  let currentHelp = {}
  let currentType = {}

  for (const raw of lines) {
    const line = raw.trim()
    if (!line || line.startsWith('#')) {
      if (line.startsWith('# HELP ')) {
        const rest = line.slice(7)
        const sp = rest.indexOf(' ')
        const name = rest.slice(0, sp)
        const help = rest.slice(sp + 1)
        currentHelp[name] = help
      } else if (line.startsWith('# TYPE ')) {
        const parts = line.slice(7).split(' ')
        currentType[parts[0]] = parts[1]
      }
      continue
    }

    // parse metric line: name{labels} value [timestamp]
    const braceOpen = line.indexOf('{')
    const spaceIdx = line.lastIndexOf(' ')
    let name, labelsStr, value

    if (braceOpen !== -1) {
      const braceClose = line.indexOf('}')
      name = line.slice(0, braceOpen)
      labelsStr = line.slice(braceOpen + 1, braceClose)
      const rest = line.slice(braceClose + 1).trim()
      value = parseFloat(rest.split(' ')[0])
    } else {
      name = line.slice(0, spaceIdx)
      labelsStr = ''
      value = parseFloat(line.slice(spaceIdx + 1).split(' ')[0])
    }

    const labels = {}
    if (labelsStr) {
      const re = /(\w+)="([^"]*)"/g
      let m
      while ((m = re.exec(labelsStr)) !== null) {
        labels[m[1]] = m[2]
      }
    }

    if (!metrics[name]) {
      metrics[name] = {
        name,
        help: currentHelp[name] || '',
        type: currentType[name] || 'untyped',
        samples: [],
      }
    }
    metrics[name].samples.push({ labels, value })
  }

  return metrics
}

export function sumMetric(metrics, name) {
  const m = metrics[name]
  if (!m) return 0
  return m.samples.reduce((acc, s) => acc + (isNaN(s.value) ? 0 : s.value), 0)
}

export function getMetricByLabel(metrics, name, labelKey, labelValue) {
  const m = metrics[name]
  if (!m) return null
  return m.samples.find(s => s.labels[labelKey] === labelValue) || null
}
