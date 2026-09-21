// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Preferences that the interface keeps in the browser (localStorage), under the prefix of the project.
//
// While the project was called Gatus the prefix was "gatus:". A preference stored under it is migrated the first time
// it is read: copied to the current key, then removed. Nothing here is required for the interface to work, so every
// access is guarded: storage can be missing, blocked or full.

export const PREFIX = 'go-uptime:'
export const LEGACY_PREFIX = 'gatus:'

export const preferenceKey = (name) => `${PREFIX}${name}`
export const legacyPreferenceKey = (name) => `${LEGACY_PREFIX}${name}`

const browserStorage = () => {
  try {
    return typeof localStorage !== 'undefined' ? localStorage : null
  } catch {
    return null
  }
}

const resolve = (storage) => (storage === undefined ? browserStorage() : storage)

// readPreference returns the stored value of a preference, or null. The current key wins; a value found only under
// the legacy key is migrated. An unreadable storage reads as null.
export const readPreference = (name, storage) => {
  const store = resolve(storage)
  if (!store) {
    return null
  }
  try {
    const current = store.getItem(preferenceKey(name))
    if (current !== null && current !== undefined) {
      return current
    }
    const legacy = store.getItem(legacyPreferenceKey(name))
    if (legacy === null || legacy === undefined) {
      return null
    }
    try {
      // Another tab may have stored the preference since the read above: what it chose is newer than the legacy value,
      // so it is kept, and only the legacy key goes
      const stored = store.getItem(preferenceKey(name))
      if (stored !== null && stored !== undefined) {
        store.removeItem(legacyPreferenceKey(name))
        return stored
      }
      store.setItem(preferenceKey(name), legacy)
      store.removeItem(legacyPreferenceKey(name))
    } catch {
      // The value is still returned: the migration is retried on the next read
    }
    return legacy
  } catch {
    return null
  }
}

// writePreference stores a preference under the current key and drops the legacy one. It returns whether it was stored.
export const writePreference = (name, value, storage) => {
  const store = resolve(storage)
  if (!store) {
    return false
  }
  try {
    store.setItem(preferenceKey(name), value)
  } catch {
    return false
  }
  try {
    store.removeItem(legacyPreferenceKey(name))
  } catch {
    // The preference is stored; a legacy key that cannot be removed loses to the current one on every read
  }
  return true
}

// removePreference removes a preference under both keys
export const removePreference = (name, storage) => {
  const store = resolve(storage)
  if (!store) {
    return
  }
  try {
    store.removeItem(preferenceKey(name))
    store.removeItem(legacyPreferenceKey(name))
  } catch {
    // Nothing to remove from a storage that cannot be reached
  }
}
