// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Numbers of the panel of the endpoint details pages, of the dashboard and of the public status pages (fork)

import { formatMilliseconds, formatUptime } from './statusPage.js'

// formatCurrentResponseTime formats the duration of the last result, or a dash without result or with a duration of
// zero (e.g. a push without ping)
export const formatCurrentResponseTime = (milliseconds) => (Number.isFinite(milliseconds) && milliseconds > 0 ? formatMilliseconds(Math.round(milliseconds)) : '—')

// detailsSummaryItems returns the five numbers of the panel, in the order of the page. Push endpoints are labeled
// "Ping", like the Uptime Kuma, and the averages include the pushes without ping, like the public payload
export const detailsSummaryItems = ({ currentResponseTime, uptime, responseTime, push } = {}) => {
  const uptimes = uptime || {}
  const responseTimes = responseTime || {}
  const averageResponseTime = responseTimes['24h']
  return [
    { label: push ? 'Ping (Current)' : 'Response (Current)', value: formatCurrentResponseTime(currentResponseTime) },
    { label: push ? 'Avg. Ping (24h)' : 'Avg. Response (24h)', value: Number.isFinite(averageResponseTime) ? formatMilliseconds(Math.round(averageResponseTime)) : '—' },
    { label: 'Uptime (24h)', value: formatUptime(uptimes['24h']) },
    { label: 'Uptime (7d)', value: formatUptime(uptimes['7d']) },
    { label: 'Uptime (30d)', value: formatUptime(uptimes['30d']) }
  ]
}
