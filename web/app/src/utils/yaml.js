// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Minimal YAML serializer for endpoint definitions (objects, arrays, strings, numbers and booleans)

const RESERVED_SCALAR = /^(true|false|yes|no|on|off|null|~|[-+]?(\d+(\.\d*)?|\.\d+)([eE][-+]?\d+)?)$/i

function needsQuotes(value) {
  return value === '' ||
    value !== value.trim() ||
    RESERVED_SCALAR.test(value) ||
    /^[-?:,[\]{}#&*!|>'"%@`]/.test(value) ||
    /: |\s#|\n/.test(value) ||
    value.endsWith(':')
}

function scalar(value) {
  if (value === null || value === undefined) {
    return 'null'
  }
  if (typeof value === 'boolean' || typeof value === 'number') {
    return String(value)
  }
  const text = String(value)
  return needsQuotes(text) ? `'${text.replace(/'/g, "''")}'` : text
}

const isNonEmptyCollection = (value) =>
  value !== null && typeof value === 'object' && (Array.isArray(value) ? value.length > 0 : Object.keys(value).length > 0)

const emptyCollection = (value) => (Array.isArray(value) ? '[]' : '{}')

export function toYaml(value, indent = 0) {
  const padding = '  '.repeat(indent)
  if (Array.isArray(value)) {
    if (value.length === 0) {
      return `${padding}[]`
    }
    return value.map((item) => {
      if (isNonEmptyCollection(item)) {
        if (Array.isArray(item)) {
          return `${padding}-\n${toYaml(item, indent + 1)}`
        }
        return `${padding}- ${toYaml(item, indent + 1).trimStart()}`
      }
      return `${padding}- ${item !== null && typeof item === 'object' ? emptyCollection(item) : scalar(item)}`
    }).join('\n')
  }
  if (value !== null && typeof value === 'object') {
    const keys = Object.keys(value)
    if (keys.length === 0) {
      return `${padding}{}`
    }
    return keys.map((key) => {
      const item = value[key]
      if (isNonEmptyCollection(item)) {
        return `${padding}${scalar(key)}:\n${toYaml(item, indent + 1)}`
      }
      return `${padding}${scalar(key)}: ${item !== null && typeof item === 'object' ? emptyCollection(item) : scalar(item)}`
    }).join('\n')
  }
  return `${padding}${scalar(value)}`
}
