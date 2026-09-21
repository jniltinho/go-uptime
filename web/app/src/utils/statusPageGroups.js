// Collapsed groups of a public status page: which state a group is shown in, and how the choice of a visitor is
// remembered between visits without leaving the name of a group in the browser.
//
// A page can require a login of its own, and its answer goes out with "private, no-store" so that it leaves no trace.
// The name of a group in localStorage would be one. So the key of a choice is the SHA-256 of the slug and of the raw
// name of the group, for every page: nothing readable is stored. crypto.subtle only exists in a secure context (HTTPS
// or localhost); without it, or without storage, the choice is kept for the visit and not remembered.

import { preferenceKey, readPreference, writePreference } from './storage.js'

// The preference of the choices, and its key in the browser (see storage.js, which also migrates the key of v6)
export const PREFERENCE = 'status-page-groups'
export const STORAGE_KEY = preferenceKey(PREFERENCE)
export const MAXIMUM_CHOICES = 500
export const COLLAPSED = 'c'
export const EXPANDED = 'e'

const HASH_PATTERN = /^[0-9a-f]{64}$/

// isCollapsed decides the state of a group, in this order: a group that is not operational is expanded, unless the
// visitor collapsed it since the last payload (incidentCollapsed); otherwise the choice of the visitor, the one of this
// visit before the one remembered from another visit; otherwise the default of the page. A problem never starts
// hidden, and forcing a group open does not erase the choice: it applies again when the group recovers.
export const isCollapsed = ({ status, visitChoice, rememberedChoice, pageDefault, incidentCollapsed }) => {
  if (status !== 'operational') {
    return incidentCollapsed === true
  }
  const choice = visitChoice || rememberedChoice
  if (choice === COLLAPSED || choice === EXPANDED) {
    return choice === COLLAPSED
  }
  return pageDefault === true
}

// groupCounts is the text of the counts of a group, without the statuses that have none: "2 up · 1 down"
export const groupCounts = (summary) => {
  if (!summary) {
    return ''
  }
  return [
    [summary.up, 'up'],
    [summary.down, 'down'],
    [summary.pending, 'pending'],
    [summary.unknown, 'no data']
  ].filter(([value]) => value > 0).map(([value, label]) => `${value} ${label}`).join(' · ')
}

// crypto and localStorage are read through typeof, so that the module also loads where one of them does not exist
const defaultSubtle = () => (typeof crypto !== 'undefined' && crypto ? crypto.subtle : undefined)

const toHex = (buffer) => Array.from(new Uint8Array(buffer), (byte) => byte.toString(16).padStart(2, '0')).join('')

// groupHashes derives the storage key of each group. It returns a Map from the raw name of the group (an empty string
// for the group without name, never its label) to the key, or null when the keys cannot be derived.
export const groupHashes = async (slug, names, subtle = defaultSubtle()) => {
  if (!subtle || typeof subtle.digest !== 'function') {
    return null
  }
  try {
    const encoder = new TextEncoder()
    const hashes = new Map()
    for (const name of names) {
      hashes.set(name, toHex(await subtle.digest('SHA-256', encoder.encode(`${slug}\n${name}`))))
    }
    return hashes
  } catch {
    return null
  }
}

const defaultStorage = () => {
  try {
    return typeof localStorage !== 'undefined' ? localStorage : null
  } catch {
    return null
  }
}

// readChoices returns the stored choices as a Map from key to choice, in insertion order, the oldest first. Anything
// that is not an object of hexadecimal keys with the expected values is ignored.
export const readChoices = (storage = defaultStorage()) => {
  const choices = new Map()
  try {
    const parsed = JSON.parse((storage ? readPreference(PREFERENCE, storage) : null) || 'null')
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return choices
    }
    for (const [key, value] of Object.entries(parsed)) {
      if (HASH_PATTERN.test(key) && (value === COLLAPSED || value === EXPANDED)) {
        choices.set(key, value)
      }
    }
  } catch {
    return new Map()
  }
  return choices
}

// writeChoice stores the choice for a key, as the newest one, and discards the oldest ones above MAXIMUM_CHOICES. It
// returns whether the choice was stored.
export const writeChoice = (key, choice, storage = defaultStorage()) => {
  if (!storage || !HASH_PATTERN.test(key) || (choice !== COLLAPSED && choice !== EXPANDED)) {
    return false
  }
  try {
    const choices = readChoices(storage)
    choices.delete(key)
    choices.set(key, choice)
    while (choices.size > MAXIMUM_CHOICES) {
      choices.delete(choices.keys().next().value)
    }
    return writePreference(PREFERENCE, JSON.stringify(Object.fromEntries(choices)), storage)
  } catch {
    return false
  }
}

// rememberedChoices resolves, before a payload is shown, the remembered choice of each of its groups: a Map from the
// raw name of the group to its choice. It also returns the keys, to store a later choice without deriving them again.
export const rememberedChoices = async (slug, groups, { subtle, storage } = {}) => {
  const names = (groups || []).map((group) => group.name || '')
  const hashes = await groupHashes(slug, names, subtle === undefined ? defaultSubtle() : subtle)
  const remembered = new Map()
  if (!hashes) {
    return { hashes: null, remembered }
  }
  const stored = readChoices(storage === undefined ? defaultStorage() : storage)
  for (const [name, hash] of hashes) {
    if (stored.has(hash)) {
      remembered.set(name, stored.get(hash))
    }
  }
  return { hashes, remembered }
}
