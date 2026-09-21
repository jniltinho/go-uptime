// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Collects the computed colours of every element of the page, for test/e2e/theme-colors.sh. It returns a JSON object
// from a stable identifier of each element (its data-testid when it has one, otherwise its path in the DOM) to the
// values that a theme can change. The pixels of images and of a canvas are out of its reach.
(() => {
  const PROPERTIES = ['color', 'background-color', 'border-top-color', 'border-right-color', 'border-bottom-color',
    'border-left-color', 'outline-color', 'fill', 'stroke', 'box-shadow', 'opacity', 'background-image',
    'text-decoration-color', 'caret-color', 'accent-color']
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
  const valuesOf = (element, pseudo) => {
    const style = getComputedStyle(element, pseudo)
    if (pseudo && (style.content === 'none' || style.content === 'normal')) {
      return null
    }
    return PROPERTIES.map((property) => style.getPropertyValue(property)).join('|')
  }
  const colours = {}
  for (const element of document.querySelectorAll('html, html *')) {
    if (['SCRIPT', 'STYLE', 'META', 'LINK', 'TITLE', 'HEAD'].includes(element.tagName)) {
      continue
    }
    const path = pathOf(element)
    colours[path] = valuesOf(element)
    for (const pseudo of ['::before', '::after', '::placeholder']) {
      const values = valuesOf(element, pseudo)
      if (values) {
        colours[path + pseudo] = values
      }
    }
  }
  return JSON.stringify(colours)
})()
