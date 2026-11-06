// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// The components use the role utilities of src/themes/roles.css, never a pair of a light class and a dark: class (fork)
import { test } from 'vitest'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, dirname, relative } from 'node:path'
import { fileURLToPath } from 'node:url'

const src = join(dirname(fileURLToPath(import.meta.url)), '..')

const sources = (dir) =>
  readdirSync(dir).flatMap((name) => {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) return sources(path)
    return /\.(vue|js)$/.test(name) && !name.endsWith('.test.js') ? [path] : []
  })

test('no component uses a dark: class: the colours come from the role utilities', () => {
  const found = []
  for (const path of sources(src)) {
    readFileSync(path, 'utf8')
      .split('\n')
      .forEach((line, index) => {
        // a dark: utility in a class list; "dark: Moon" (an object key followed by a space) is not one
        if (/(^|[\s'"`[])dark:[a-z[]/.test(line)) found.push(`${relative(src, path)}:${index + 1}`)
      })
  }
  assert.deepEqual(found, [], 'use a role of src/themes/roles.css instead of a dark: class')
})

test('every role used by the components is defined in roles.css', () => {
  const roles = new Set([...readFileSync(join(src, 'themes', 'roles.css'), 'utf8').matchAll(/@utility ([a-z0-9-]+)/g)].map((m) => m[1]))
  assert.ok(roles.size > 0)
  for (const role of ['text-fg-muted', 'border-line', 'border-field', 'bg-surface', 'bg-danger-soft', 'text-on-inverse', 'divide-line']) {
    assert.ok(roles.has(role), role)
  }
})
