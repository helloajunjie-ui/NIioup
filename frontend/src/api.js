const TOKEN_KEY = 'anonfile_admin_token'
const GUEST_KEY = 'anonfile_guest_uid'
export const getToken = () => localStorage.getItem(TOKEN_KEY)
export const setToken = (t) => localStorage.setItem(TOKEN_KEY, t)
export const clearToken = () => localStorage.removeItem(TOKEN_KEY)

// 游客 UID：本机生成并持久化，用于区分访客、防冲突
export function getGuestUid() { return localStorage.getItem(GUEST_KEY) || '' }
export function ensureGuestUid() {
  let u = getGuestUid()
  if (!u) {
    u = 'G' + Math.random().toString(36).slice(2, 12)
    localStorage.setItem(GUEST_KEY, u)
  }
  return u
}

async function req(path, opts = {}) {
  const headers = { ...(opts.headers || {}) }
  const token = getToken()
  if (token) headers.Authorization = 'Bearer ' + token
  const res = await fetch(path, { ...opts, headers })
  let data = {}
  try { data = await res.json() } catch (_) {}
  if (!res.ok) {
    const err = new Error(data.error || ('请求失败 ' + res.status))
    err.status = res.status
    throw err
  }
  return data
}

// 带进度的上传（XHR 才有 upload.onprogress），附带游客 UID 与分类
function upload(file, opts = {}, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/api/upload')
    const token = getToken()
    if (token) xhr.setRequestHeader('Authorization', 'Bearer ' + token)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onload = () => {
      let data = {}
      try { data = JSON.parse(xhr.responseText) } catch (_) {}
      if (xhr.status >= 200 && xhr.status < 300) resolve(data)
      else reject(new Error(data.error || '上传失败 ' + xhr.status))
    }
    xhr.onerror = () => reject(new Error('网络错误'))
    const fd = new FormData()
    fd.append('file', file)
    if (opts.guestUid) fd.append('guest_uid', opts.guestUid)
    if (opts.category) fd.append('category', opts.category)
    xhr.send(fd)
  })
}

// 带进度上传备份包到恢复接口
function restore(file, onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/api/admin/restore')
    xhr.setRequestHeader('Authorization', 'Bearer ' + getToken())
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onload = () => {
      let data = {}
      try { data = JSON.parse(xhr.responseText) } catch (_) {}
      if (xhr.status >= 200 && xhr.status < 300) resolve(data)
      else reject(new Error(data.error || '恢复失败 ' + xhr.status))
    }
    xhr.onerror = () => reject(new Error('网络错误'))
    const fd = new FormData()
    fd.append('file', file)
    xhr.send(fd)
  })
}

// 一键备份：带鉴权下载 zip
function downloadBackup(onProgress) {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/api/admin/backup')
    xhr.setRequestHeader('Authorization', 'Bearer ' + getToken())
    xhr.responseType = 'blob'
    xhr.onprogress = (e) => {
      if (e.lengthComputable && onProgress) onProgress(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) resolve(xhr.response)
      else reject(new Error('备份失败 ' + xhr.status))
    }
    xhr.onerror = () => reject(new Error('网络错误'))
    xhr.send()
  })
}

export const api = {
  info: () => req('/api/info'),
  upload,
  restore,
  publicFiles: (limit = 20) => req('/api/files?limit=' + limit),
  selfDelete: (token) =>
    req('/api/delete', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ token }) }),
  login: (password) =>
    req('/api/admin/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password }) }),
  adminFiles: (q, offset, limit) =>
    req('/api/admin/files?q=' + encodeURIComponent(q || '') + '&offset=' + offset + '&limit=' + limit),
  adminDelete: (id) => req('/api/admin/files/' + id, { method: 'DELETE' }),
  getConfig: () => req('/api/admin/config'),
  putConfig: (cfg) =>
    req('/api/admin/config', { method: 'PUT', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(cfg) }),
  backup: downloadBackup,
}

export function formatSize(bytes) {
  if (bytes == null) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return (i === 0 ? v : v.toFixed(1)) + ' ' + units[i]
}

export function formatTime(unix) {
  if (!unix) return '-'
  const d = new Date(unix * 1000)
  const p = (n) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

export function copyText(text) {
  return navigator.clipboard.writeText(text)
}

export function origin() {
  return location.origin
}
