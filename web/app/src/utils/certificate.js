// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Expiration of the TLS certificate of an endpoint (fork), shown discreetly like the "Cert Exp." of the Uptime Kuma
const DAY_MS = 24 * 60 * 60 * 1000
const NANOSECONDS_PER_MILLISECOND = 1000000

// certificateOfResults returns the days until the certificate expires (rounded down, negative once expired) and the
// expiration date, from the most recent result with a certificate of the protected status API, or null without one.
// Pushes and failed connections have no certificate and are skipped.
export const certificateOfResults = (results, now = Date.now()) => {
  const list = results || []
  for (let i = list.length - 1; i >= 0; i--) {
    const result = list[i]
    if (result && result.certificateExpiration) {
      const expiresAt = new Date(result.timestamp).getTime() + result.certificateExpiration / NANOSECONDS_PER_MILLISECOND
      return { days: Math.floor((expiresAt - now) / DAY_MS), expiresAt: new Date(expiresAt) }
    }
  }
  return null
}

// certificateDate formats the expiration date the same way on the dashboard and on the public details page
export const certificateDate = (expiresAt) => {
  const date = expiresAt instanceof Date ? expiresAt : new Date(expiresAt)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleDateString(undefined, { year: 'numeric', month: 'short', day: 'numeric' })
}

export const certificateText = (days) => {
  if (days > 1) {
    return `Certificate expires in ${days} days`
  }
  if (days === 1) {
    return 'Certificate expires in 1 day'
  }
  if (days === 0) {
    return 'Certificate expires today'
  }
  return days === -1 ? 'Certificate expired 1 day ago' : `Certificate expired ${-days} days ago`
}

// Secondary color while the expiration is far, amber from 14 days and red from 7 days or once expired
export const certificateClass = (days) => {
  if (days <= 7) {
    return 'text-red-600 dark:text-red-400'
  }
  if (days <= 14) {
    return 'text-amber-600 dark:text-amber-400'
  }
  return 'text-muted-foreground dark:text-gray-400'
}
