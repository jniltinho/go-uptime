import { test } from 'vitest'
import assert from 'node:assert/strict'
import { hasUserAgentHeader, supportsUserAgent, userAgentToSend } from './userAgent.js'

test('supports the URLs whose checks send the headers', () => {
  assert.equal(supportsUserAgent('https://example.org'), true)
  assert.equal(supportsUserAgent('HTTP://example.org'), true)
  assert.equal(supportsUserAgent(' wss://example.org/ws'), true)
  assert.equal(supportsUserAgent('ws://example.org'), true)
  assert.equal(supportsUserAgent('grpcs://example.org:443'), false)
  assert.equal(supportsUserAgent('tcp://example.org:443'), false)
  assert.equal(supportsUserAgent('tls://example.org:443'), false)
  assert.equal(supportsUserAgent('8.8.8.8'), false)
  assert.equal(supportsUserAgent(''), false)
  assert.equal(supportsUserAgent('https://1.1.1.1', true), false)
})

test('finds a User-Agent header row in any case, trimmed', () => {
  assert.equal(hasUserAgentHeader([{ name: ' User-agent ', value: 'x' }]), true)
  assert.equal(hasUserAgentHeader([{ name: 'USER-AGENT', value: '' }]), true)
  assert.equal(hasUserAgentHeader([{ name: 'Authorization', value: 'x' }]), false)
  assert.equal(hasUserAgentHeader([]), false)
  assert.equal(hasUserAgentHeader(undefined), false)
})

test('sends a preset only when the server accepts it', () => {
  assert.equal(userAgentToSend('chrome', 'https://example.org', false, []), 'chrome')
  assert.equal(userAgentToSend('edge', 'wss://example.org', false, []), 'edge')
  assert.equal(userAgentToSend('', 'https://example.org', false, []), null)
  assert.equal(userAgentToSend('go-uptime', 'https://example.org', false, []), null)
  assert.equal(userAgentToSend('firefox', 'grpcs://example.org:443', false, []), null)
  assert.equal(userAgentToSend('firefox', 'https://1.1.1.1', true, []), null)
  assert.equal(userAgentToSend('chrome', 'https://example.org', false, [{ name: 'user-agent', value: 'Mine/1.0' }]), null)
})
