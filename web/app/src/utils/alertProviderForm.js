// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Fork: form of an alerting provider of the administration. Converts the definition of a provider (the keys of its
// block in the alerting section, without "type") to the values of the form and back, from the description of the type
// answered by GET /api/v1/admin/alert-providers (info.fields: key, kind, secret, required, default, help).
//
// The conversion is lossless: every key the form does not show (client, overrides, provider-override of the default
// alert, a field unknown to the form) is kept as it was, so that switching between the form and the YAML loses nothing.

// Value of a secret in every answer of the API; sent back, it keeps the stored value
export const MASKED = '********'

// Choices of a value inherited from the default alert of the provider
export const INHERIT = 'default'
export const YES = 'yes'
export const NO = 'no'

const DEFAULT_ALERT = 'default-alert'

const deepCopy = (value) => (value === undefined ? undefined : JSON.parse(JSON.stringify(value)))

const isEmpty = (value) => value === undefined || value === null || value === ''

const triState = (value) => (value === true ? YES : value === false ? NO : INHERIT)

// mapToText writes a map of strings as one "key: value" line per entry
export function mapToText(map) {
  if (!map || typeof map !== 'object') {
    return ''
  }
  return Object.entries(map).map(([key, value]) => `${key}: ${value}`).join('\n')
}

// textToMap reads the lines written by mapToText; a line without ":" is ignored
export function textToMap(text) {
  const map = {}
  for (const line of String(text || '').split('\n')) {
    const separator = line.indexOf(':')
    if (separator <= 0) {
      continue
    }
    map[line.slice(0, separator).trim()] = line.slice(separator + 1).trim()
  }
  return map
}

// formFromDocument returns the values of the form for a definition
export function formFromDocument(info, document) {
  const source = document || {}
  const values = {}
  for (const field of (info && info.fields) || []) {
    const value = source[field.key]
    switch (field.kind) {
      case 'boolean':
        values[field.key] = value === true
        break
      case 'list':
        values[field.key] = Array.isArray(value) ? value.join(', ') : isEmpty(value) ? '' : String(value)
        break
      case 'map':
        values[field.key] = value && typeof value === 'object' && !Array.isArray(value) ? mapToText(value) : ''
        break
      default:
        values[field.key] = isEmpty(value) ? '' : String(value)
    }
  }
  const defaultAlert = source[DEFAULT_ALERT] || {}
  return {
    values,
    defaultAlert: {
      failureThreshold: isEmpty(defaultAlert['failure-threshold']) ? '' : String(defaultAlert['failure-threshold']),
      successThreshold: isEmpty(defaultAlert['success-threshold']) ? '' : String(defaultAlert['success-threshold']),
      sendOnResolved: triState(defaultAlert['send-on-resolved']),
      description: isEmpty(defaultAlert.description) ? '' : String(defaultAlert.description),
      minimumReminderInterval: isEmpty(defaultAlert['minimum-reminder-interval']) ? '' : String(defaultAlert['minimum-reminder-interval']),
    },
  }
}

// documentFromForm returns the definition of the form, starting from original so that the keys the form does not
// show are kept. An empty field removes its key; an unchecked boolean is false only when the original had the key.
export function documentFromForm(info, form, original) {
  const document = deepCopy(original) || {}
  delete document.type
  for (const field of (info && info.fields) || []) {
    const value = form.values[field.key]
    switch (field.kind) {
      case 'boolean':
        if (value) {
          document[field.key] = true
        } else if (field.key in document) {
          document[field.key] = false
        }
        break
      case 'list': {
        const items = String(value || '').split(/[,\n]/).map((item) => item.trim()).filter(Boolean)
        if (items.length) {
          document[field.key] = items
        } else {
          delete document[field.key]
        }
        break
      }
      case 'map': {
        const map = textToMap(value)
        if (Object.keys(map).length) {
          document[field.key] = map
        } else {
          delete document[field.key]
        }
        break
      }
      case 'integer': {
        const text = String(value ?? '').trim()
        if (text === '') {
          delete document[field.key]
        } else {
          document[field.key] = /^-?\d+$/.test(text) ? Number(text) : text
        }
        break
      }
      default: {
        const text = String(value ?? '')
        if (text.trim() === '') {
          delete document[field.key]
        } else {
          document[field.key] = text
        }
      }
    }
  }
  const defaultAlert = { ...(document[DEFAULT_ALERT] || {}) }
  const setOrDelete = (key, value) => {
    if (isEmpty(value)) {
      delete defaultAlert[key]
    } else {
      defaultAlert[key] = value
    }
  }
  const number = (text) => (/^\d+$/.test(String(text).trim()) ? Number(String(text).trim()) : (String(text).trim() || null))
  setOrDelete('failure-threshold', number(form.defaultAlert.failureThreshold))
  setOrDelete('success-threshold', number(form.defaultAlert.successThreshold))
  setOrDelete('send-on-resolved', form.defaultAlert.sendOnResolved === YES ? true : form.defaultAlert.sendOnResolved === NO ? false : null)
  setOrDelete('description', String(form.defaultAlert.description || '').trim() || null)
  setOrDelete('minimum-reminder-interval', String(form.defaultAlert.minimumReminderInterval || '').trim() || null)
  if (Object.keys(defaultAlert).length) {
    document[DEFAULT_ALERT] = defaultAlert
  } else {
    delete document[DEFAULT_ALERT]
  }
  return document
}

// missingRequired returns the keys of the required fields left empty; a stored secret (MASKED) counts as filled
export function missingRequired(info, form) {
  return ((info && info.fields) || [])
    .filter((field) => field.required && field.kind !== 'boolean')
    .filter((field) => String(form.values[field.key] ?? '').trim() === '')
    .map((field) => field.key)
}

// providerState returns the label and the tone of the state of an item of the list
export function providerState(item) {
  if (!item || item.source === 'none') {
    return { label: 'Not configured', tone: 'muted' }
  }
  if (item.state === 'invalid') {
    return { label: 'Invalid', tone: 'error' }
  }
  return { label: 'Valid', tone: 'success' }
}

// usageCount returns how many endpoints use a provider
export function usageCount(item) {
  const usage = (item && item.usage) || {}
  return ((usage.managed || []).length) + ((usage.file || []).length)
}

// groupByCategory groups the descriptions of the types by category, in the order of CATEGORIES, each sorted by name
export const CATEGORIES = ['Chat', 'Email', 'SMS', 'Push', 'Incidents', 'Observability', 'Issues', 'Automation']

export function groupByCategory(types, query = '') {
  const needle = String(query).trim().toLowerCase()
  const matches = (type) => !needle || type.name.toLowerCase().includes(needle) || type.type.includes(needle)
  return CATEGORIES
    .map((category) => ({
      category,
      types: (types || []).filter((type) => type.category === category && matches(type)).sort((a, b) => a.name.localeCompare(b.name)),
    }))
    .filter((group) => group.types.length > 0)
}
