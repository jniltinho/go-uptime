// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
import { afterEach, test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'
import vm from 'node:vm'
import { THEMES, applyTheme, currentTheme, defaultTheme, defaultThemeIsDark, setTheme, themeFromCookie, wantsDarkMode } from './theme.js'

const here = dirname(fileURLToPath(import.meta.url))
const { themes, cases } = JSON.parse(readFileSync(join(here, 'theme.cases.json'), 'utf8'))

// A fake document with the data-default-theme attribute (absent when null or undefined), the cookie and the theme-color
// meta. It serves both theme.js (dataset) and the inline script of index.html (getAttribute).
const fakeDocument = ({ defaultTheme: value, cookie = '', classes = [] } = {}) => {
  const classSet = new Set(classes)
  const meta = { content: '', setAttribute(name, attribute) { this[name] = attribute } }
  const absent = value === undefined || value === null
  return {
    classSet,
    meta,
    document: {
      cookie,
      documentElement: {
        dataset: absent ? {} : { defaultTheme: value },
        getAttribute: (name) => (name === 'data-default-theme' && !absent ? value : null),
        classList: {
          toggle: (name, force) => (force ? classSet.add(name) : classSet.delete(name)),
          add: (name) => classSet.add(name),
          remove: (...names) => names.forEach((name) => classSet.delete(name))
        }
      },
      querySelector: (selector) => (selector === 'meta[name="theme-color"]' ? meta : null)
    }
  }
}

const classOf = (classSet) => [...classSet].sort().join(' ')
const cookieOf = (value) => (value ? `other=1; theme=${value}` : 'other=1')

afterEach(() => {
  delete global.document
})

test('the table of themes of theme.js is the one of theme.cases.json', () => {
  assert.deepEqual(Object.fromEntries(Object.entries(THEMES).map(([theme, { className, themeColor }]) => [theme, { class: className, themeColor }])), themes)
})

test('theme.js follows every case of theme.cases.json', () => {
  for (const item of cases) {
    // A class of another theme left over, to check that the classes are mutually exclusive
    const fake = fakeDocument({ defaultTheme: item.default, cookie: cookieOf(item.cookie), classes: ['dark', 'theme-bio', 'unrelated'] })
    global.document = fake.document
    assert.equal(currentTheme(), item.theme, JSON.stringify(item))
    assert.equal(wantsDarkMode(), item.theme === 'dark', JSON.stringify(item))
    applyTheme(currentTheme())
    assert.equal(classOf(fake.classSet), ['unrelated', item.class].filter(Boolean).sort().join(' '), JSON.stringify(item))
    assert.equal(fake.meta.content, item.themeColor, JSON.stringify(item))
  }
})

test('the inline script of index.html follows every case of theme.cases.json', () => {
  const html = readFileSync(join(here, '..', '..', 'public', 'index.html'), 'utf8')
  const start = html.indexOf('(function() {', html.indexOf('Initialize theme immediately'))
  const end = html.indexOf('})();', start) + '})();'.length
  assert.ok(start > 0 && end > start, 'the inline theme script was not found in public/index.html')
  const script = new vm.Script(html.slice(start, end))
  for (const item of cases) {
    const fake = fakeDocument({ defaultTheme: item.default, cookie: cookieOf(item.cookie), classes: ['dark', 'theme-bio', 'unrelated'] })
    script.runInNewContext({ document: fake.document })
    assert.equal(classOf(fake.classSet), ['unrelated', item.class].filter(Boolean).sort().join(' '), JSON.stringify(item))
    assert.equal(fake.meta.content, item.themeColor, JSON.stringify(item))
  }
})

test('the default theme: the attribute, its absence, the template not rendered and the HTML of a previous version', () => {
  for (const [value, expected] of [['dark', 'dark'], ['light', 'light'], ['bio', 'bio'], ['', 'light'], [undefined, 'dark'], ['{{ .DefaultTheme }}', 'dark']]) {
    global.document = fakeDocument({ defaultTheme: value }).document
    assert.equal(defaultTheme(), expected, String(value))
    assert.equal(defaultThemeIsDark(), expected === 'dark', String(value))
  }
})

test('an invalid cookie is ignored', () => {
  for (const value of ['blue', 'DARK', 'theme-bio', '']) {
    global.document = fakeDocument({ defaultTheme: 'bio', cookie: cookieOf(value) }).document
    assert.equal(themeFromCookie(), '', value)
    assert.equal(currentTheme(), 'bio', value)
  }
})

test('choosing a theme saves the cookie and moves between every pair of themes with one class at a time', () => {
  const fake = fakeDocument({ defaultTheme: 'dark' })
  global.document = fake.document
  for (const from of Object.keys(THEMES)) {
    for (const to of Object.keys(THEMES)) {
      setTheme(from)
      assert.equal(setTheme(to), to)
      assert.match(fake.document.cookie, new RegExp(`^theme=${to}; path=/; max-age=31536000; samesite=strict$`))
      assert.equal(classOf(fake.classSet), THEMES[to].className, `${from} -> ${to}`)
      assert.equal(fake.meta.content, THEMES[to].themeColor, `${from} -> ${to}`)
    }
  }
  assert.equal(setTheme('blue'), 'light', 'a value that is not a theme falls back to light')
})
