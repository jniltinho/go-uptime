// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Client of the administration API of endpoints (/api/v1/admin)
import { PROTECTED_API_HEADERS, notifyUnauthorized } from '@/utils/auth'
import { parseRetryAfter, readBlobResponse } from '@/utils/adminBackup'

const BASE_URL = '/api/v1/admin'

export class AdminApiError extends Error {
  // Fork: data is the parsed JSON of the answer and retryAfter the seconds of Retry-After, used by the Backup tab
  constructor(status, message, { data = null, retryAfter = null } = {}) {
    super(message)
    this.status = status
    this.data = data
    this.retryAfter = retryAfter
  }
}

const encodeKey = (key) => encodeURIComponent(key)

const withKey = (path, key) => (key ? `${path}?key=${encodeKey(key)}` : path)

async function request(method, path, { body, contentType, version } = {}) {
  const headers = { ...PROTECTED_API_HEADERS }
  if (body !== undefined) {
    headers['Content-Type'] = contentType
  }
  if (version) {
    headers['If-Match'] = `"${version}"`
  }
  const response = await fetch(BASE_URL + path, { method, headers, body, credentials: 'include' })
  const text = await response.text()
  let data = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch (e) {
      data = { error: text }
    }
  }
  if (response.status === 401) {
    notifyUnauthorized()
  }
  if (!response.ok) {
    throw new AdminApiError(response.status, (data && data.error) || response.statusText, {
      data,
      retryAfter: parseRetryAfter(response.headers.get('Retry-After'))
    })
  }
  return { data, status: response.status }
}

// Fork: call that answers a file (backup): Blob and name of the Content-Disposition when it is ok, AdminApiError with
// the JSON {error} or the text of the answer otherwise (a 413 of Fiber or of a proxy is text)
async function requestBlob(method, path, { body, contentType } = {}) {
  const headers = { ...PROTECTED_API_HEADERS }
  if (body !== undefined) {
    headers['Content-Type'] = contentType
  }
  const response = await fetch(BASE_URL + path, { method, headers, body, credentials: 'include' })
  return readBlobResponse(response, {
    onUnauthorized: notifyUnauthorized,
    createError: (status, message, options) => new AdminApiError(status, message, options)
  })
}

export const jsonPayload = (document) => ({ body: JSON.stringify(document), contentType: 'application/json' })

export const yamlPayload = (text) => ({ body: text, contentType: 'application/yaml' })

export const adminApi = {
  metadata: () => request('GET', '/metadata'),
  list: () => request('GET', '/endpoints'),
  get: (key) => request('GET', `/endpoints/${encodeKey(key)}`),
  parse: (payload) => request('POST', '/endpoints/parse', payload),
  validate: (payload, key) => request('POST', withKey('/endpoints/validate', key), payload),
  test: (payload, key) => request('POST', withKey('/endpoints/test', key), payload),
  create: (payload) => request('POST', '/endpoints', payload),
  update: (key, payload, version) => request('PUT', `/endpoints/${encodeKey(key)}`, { ...payload, version }),
  setEnabled: (key, enabled, version) => request('POST', `/endpoints/${encodeKey(key)}/${enabled ? 'enable' : 'disable'}`, { version }),
  remove: (key, version) => request('DELETE', `/endpoints/${encodeKey(key)}`, { version }),
  // Fork: generic download of a file answered by a POST with a JSON body
  download: (path, document) => requestBlob('POST', path, jsonPayload(document)),
}

const encodeSlug = (slug) => encodeURIComponent(slug)

// Client of the administration API of the public status pages (/api/v1/admin/status-pages)
export const statusPagesApi = {
  list: () => request('GET', '/status-pages'),
  options: () => request('GET', '/status-pages/options'),
  exposure: ({ group, key }) => {
    const params = new URLSearchParams()
    if (group) {
      params.set('group', group)
    }
    if (key) {
      params.set('key', key)
    }
    return request('GET', `/status-pages/exposure?${params}`)
  },
  validate: (document, slug) => request('POST', slug ? `/status-pages/validate?slug=${encodeSlug(slug)}` : '/status-pages/validate', jsonPayload(document)),
  get: (slug) => request('GET', `/status-pages/${encodeSlug(slug)}`),
  create: (document) => request('POST', '/status-pages', jsonPayload(document)),
  update: (slug, document, version) => request('PUT', `/status-pages/${encodeSlug(slug)}`, { ...jsonPayload(document), version }),
  setEnabled: (slug, enabled, version) => request('POST', `/status-pages/${encodeSlug(slug)}/${enabled ? 'enable' : 'disable'}`, { version }),
  remove: (slug, version) => request('DELETE', `/status-pages/${encodeSlug(slug)}`, { version }),
  preview: (slug) => request('GET', `/status-pages/${encodeSlug(slug)}/preview`),
}

// Client of the administration API of the global push keys (/api/v1/admin/push-keys, fork)
export const pushKeysApi = {
  list: () => request('GET', '/push-keys'),
  create: (name) => request('POST', '/push-keys', jsonPayload({ name })),
  remove: (id) => request('DELETE', `/push-keys/${encodeURIComponent(id)}`),
}

// Backup and restore of the administration (/api/v1/admin/backup and /api/v1/admin/restore, fork)
export const backupApi = {
  // Without a password the body is {}, and the file is in plain text
  download: (password) => adminApi.download('/backup', password ? { password } : {}),
  preview: (body) => request('POST', '/restore/preview', jsonPayload(body)),
  restore: (body) => request('POST', '/restore', jsonPayload(body)),
}

export function describePushKeyError(error) {
  switch (error && error.status) {
    case 400:
      return 'The name of the key must have between 1 and 64 characters.'
    case 404:
      return 'The push key no longer exists.'
    case 409:
      return 'A push key with this name already exists.'
    case 501:
      return 'The configured storage does not support push keys managed through the web.'
    default:
      return describeAdminError(error)
  }
}

export function describeStatusPageError(error) {
  switch (error && error.status) {
    case 409:
      return error.message && error.message.includes('configuration file and cannot be changed')
        ? 'This status page is defined in the configuration file and cannot be changed through the web.'
        : 'A status page with this slug already exists.'
    case 412:
      return 'The status page was changed by someone else since you opened it. Reload it to see the current version.'
    case 501:
      return 'The configured storage does not support status pages managed through the web.'
    default:
      return describeAdminError(error)
  }
}

export function describeAdminError(error) {
  switch (error && error.status) {
    case 401:
      return 'Authentication required.'
    case 403:
      return 'Administrator permission required.'
    case 409:
      if (error.message && error.message.includes('already has a managed endpoint or stored data')) {
        return 'The new name and group are already used by another endpoint or by the history of a removed one. Choose another name or group.'
      }
      if (error.message && error.message.includes('managed status page version does not match')) {
        return 'A status page that selects this endpoint was changed at the same time. Try again.'
      }
      if (error.message && error.message.includes('push token is already used')) {
        return 'This push token is already used by another endpoint or by a push key. Generate another token.'
      }
      return error.message || 'Conflict.'
    case 412:
      return 'The endpoint was changed by someone else since you opened it.'
    case 429:
      return 'Too many endpoint tests in progress. Try again in a moment.'
    case 503:
      return 'Status is starting or reloading its configuration. Try again in a moment.'
    default:
      return (error && error.message) || 'Unexpected error.'
  }
}
