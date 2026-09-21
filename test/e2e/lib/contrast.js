// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Measures the contrast of every text of the page against the background it is really drawn on, for
// test/e2e/theme-colors.sh --contrast. It returns a JSON object from a stable identifier of each element that has text
// of its own to { ratio, foreground, background, large }. The background is the first opaque one going up the
// ancestors, with the translucent ones composited over it. Text over an image or a canvas is out of its reach.
(() => {
  const parse = (value) => {
    const match = /rgba?\(([^)]+)\)/.exec(value || '')
    if (!match) return null
    const parts = match[1].split(/[,\s/]+/).filter(Boolean).map(Number)
    return { r: parts[0], g: parts[1], b: parts[2], a: parts.length > 3 ? parts[3] : 1 }
  }
  const over = (top, bottom) => ({ r: top.r * top.a + bottom.r * (1 - top.a), g: top.g * top.a + bottom.g * (1 - top.a), b: top.b * top.a + bottom.b * (1 - top.a), a: 1 })
  const luminance = ({ r, g, b }) => {
    const channel = (value) => { const c = value / 255; return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4 }
    return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b)
  }
  const backgroundOf = (element) => {
    const layers = []
    for (let node = element; node; node = node.parentElement) {
      const colour = parse(getComputedStyle(node).backgroundColor)
      if (colour && colour.a > 0) {
        layers.push(colour)
        if (colour.a === 1) break
      }
    }
    let result = { r: 255, g: 255, b: 255, a: 1 }
    for (const layer of layers.reverse()) result = over(layer, result)
    return result
  }
  const pathOf = (element) => {
    const parts = []
    for (let node = element; node && node.nodeType === 1 && node !== document.documentElement; node = node.parentElement) {
      const testId = node.getAttribute('data-testid')
      if (testId) {
        const same = Array.from(document.querySelectorAll('[data-testid="' + CSS.escape(testId) + '"]'))
        parts.unshift('[' + testId + (same.length > 1 ? '#' + same.indexOf(node) : '') + ']')
        break
      }
      const siblings = Array.from(node.parentElement ? node.parentElement.children : []).filter((sibling) => sibling.tagName === node.tagName)
      parts.unshift(node.tagName.toLowerCase() + (siblings.length > 1 ? ':' + siblings.indexOf(node) : ''))
    }
    return parts.join('>')
  }
  const hex = ({ r, g, b }) => '#' + [r, g, b].map((value) => Math.round(value).toString(16).padStart(2, '0')).join('')
  const result = {}
  for (const element of document.querySelectorAll('body *')) {
    const ownText = Array.from(element.childNodes).some((node) => node.nodeType === 3 && node.textContent.trim().length > 0)
    if (!ownText) continue
    const style = getComputedStyle(element)
    const box = element.getBoundingClientRect()
    if (style.visibility === 'hidden' || style.display === 'none' || box.width === 0 || box.height === 0 || element.closest('.sr-only')) continue
    const foregroundRaw = parse(style.color)
    if (!foregroundRaw) continue
    const background = backgroundOf(element)
    const foreground = over(foregroundRaw, background)
    const [light, dark] = [luminance(foreground), luminance(background)].sort((a, b) => b - a)
    const size = parseFloat(style.fontSize)
    const bold = Number(style.fontWeight) >= 700
    result[pathOf(element)] = {
      ratio: Math.round(((light + 0.05) / (dark + 0.05)) * 100) / 100,
      foreground: hex(foreground),
      background: hex(background),
      large: size >= 24 || (size >= 18.66 && bold),
      text: element.textContent.trim().slice(0, 30)
    }
  }
  return JSON.stringify(result)
})()
