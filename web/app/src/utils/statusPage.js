// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Helpers of the public status pages (fork)

// Relative import with the extension, so that the unit tests can load this module with Node
import { generatePrettyTimeAgo, generatePrettyTimeDifference } from './time.js'

export const SLUG_PATTERN = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/

const sanitizeKeyPart = (value) => (value || '').toLowerCase().trim().replace(/[/_., #+&]/g, '-')

// endpointKey mirrors key.ConvertGroupAndNameToKey of the backend
export const endpointKey = (group, name) => `${sanitizeKeyPart(group)}_${sanitizeKeyPart(name)}`

// Labels of the statuses of the payload, for a page ("page"), a group ("group") and an endpoint ("endpoint")
export const STATUS_LABELS = {
  operational: { page: 'All systems operational', group: 'Operational' },
  degraded: { page: 'Partial outage', group: 'Partial outage' },
  down: { page: 'Major outage', group: 'Major outage', endpoint: 'Down' },
  up: { endpoint: 'Up' },
  pending: { endpoint: 'Pending' },
  unknown: { page: 'No data', group: 'No data', endpoint: 'No data' }
}

// Like the dashboard, numbers and dates use the locale of the browser
const percentFormat = new Intl.NumberFormat(undefined, { style: 'percent', maximumFractionDigits: 2 })
const dateTimeFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'short', timeStyle: 'medium' })

// formatUptime formats an uptime between 0 and 1, or a dash without execution during the period
export const formatUptime = (uptime) => (uptime === null || uptime === undefined ? '—' : percentFormat.format(uptime))

export const formatDateTime = (timestamp) => dateTimeFormat.format(new Date(timestamp))

// relativeTimeLabel describes how long ago the timestamp was, never in the future when the clocks differ
export const relativeTimeLabel = (timestamp, now) => {
  const seconds = Math.max(0, Math.round((now - Date.parse(timestamp)) / 1000))
  if (seconds < 10) {
    return 'just now'
  }
  if (seconds < 60) {
    return `${seconds} seconds ago`
  }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) {
    return minutes === 1 ? '1 minute ago' : `${minutes} minutes ago`
  }
  const hours = Math.floor(minutes / 60)
  return hours === 1 ? '1 hour ago' : `${hours} hours ago`
}

// formatMilliseconds formats a response time, or a dash without execution
export const formatMilliseconds = (milliseconds) => (milliseconds === null || milliseconds === undefined ? '—' : `${milliseconds} ms`)

// describeEvents returns the events, from the most recent to the oldest, with the texts of the endpoint details page of
// the dashboard
export const describeEvents = (events) => {
  const described = []
  for (let i = events.length - 1; i >= 0; i--) {
    const event = events[i]
    const nextEvent = events[i + 1]
    let text
    if (event.type === 'START') {
      text = 'Monitoring started'
    } else if (event.type === 'HEALTHY') {
      text = nextEvent ? 'Endpoint became healthy' : 'Endpoint is healthy'
    } else if (event.type === 'UNHEALTHY') {
      text = nextEvent ? `Endpoint was unhealthy for ${generatePrettyTimeDifference(nextEvent.timestamp, event.timestamp)}` : 'Endpoint is unhealthy'
    } else {
      continue
    }
    described.push({ ...event, text, timeAgo: generatePrettyTimeAgo(event.timestamp) })
  }
  return described
}
