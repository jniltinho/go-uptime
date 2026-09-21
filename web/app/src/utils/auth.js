// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Calls to the protected API with the login screen of security.basic (fork)

// Headers of the calls to the protected API: with them, a 401 does not open the native credentials dialog of the browser
export const PROTECTED_API_HEADERS = Object.freeze({ 'X-Requested-With': 'XMLHttpRequest' })

// Event dispatched on window when the protected API answers 401: App.vue reloads the configuration and, without a
// session, opens the login screen
export const UNAUTHORIZED_EVENT = 'go-uptime:unauthorized'

export const notifyUnauthorized = () => {
  window.dispatchEvent(new CustomEvent(UNAUTHORIZED_EVENT))
}
