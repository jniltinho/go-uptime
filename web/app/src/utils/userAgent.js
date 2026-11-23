// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Choice of the user-agent preset of an endpoint in the endpoint form of the administration. The server applies the
// same rules (internal/config/endpoint/useragent.go); the form only avoids building a definition the server refuses.

// The presets of the user-agent field. The empty value is the default (go-uptime/1.0): the form never sends it.
export const userAgentOptions = [
  { label: 'Go Uptime (default)', value: '', testid: 'admin-user-agent-default' },
  { label: 'Chrome', value: 'chrome', testid: 'admin-user-agent-chrome' },
  { label: 'Firefox', value: 'firefox', testid: 'admin-user-agent-firefox' },
  { label: 'Edge', value: 'edge', testid: 'admin-user-agent-edge' },
]

// Whether the check of an endpoint sends its headers, and so accepts a preset: HTTP(S) and WebSocket URLs, without a
// DNS check (which wins over the URL on the server).
export const supportsUserAgent = (url, isDns = false) => !isDns && /^(https?|wss?):\/\//i.test(String(url || '').trim())

// Whether the header rows of the form set the User-Agent, compared like the serialization does (trimmed name) and like
// the server does (any letter case).
export const hasUserAgentHeader = (headers) =>
  Array.isArray(headers) &&
  headers.some(
    (header) =>
      String(header.name || '')
        .trim()
        .toLowerCase() === 'user-agent',
  )

// The value of the user-agent field to send, or null to remove the field from the document.
export const userAgentToSend = (preset, url, isDns, headers) => {
  if (!preset || preset === 'go-uptime' || !supportsUserAgent(url, isDns) || hasUserAgentHeader(headers)) {
    return null
  }
  return preset
}
