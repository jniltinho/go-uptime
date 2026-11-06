// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.
// Views of a public status page (fork): the "grouped" and "matrix" layouts, computed in the browser from the payload.
// The module is pure, so that the unit tests load it with Node (npm run test:unit).
//
// The payload is already cut by status-pages.maximum-endpoints-per-page: the views only see the published endpoints and
// never widen the selection. Featured endpoints carry their group and are placed back in it, so that the featured
// section of the list view does not leave holes in the other views.

export const VIEW_LIST = 'list'
export const VIEW_GROUPED = 'grouped'
export const VIEW_MATRIX = 'matrix'
export const ALL = ''

// Labels of the views, for the switcher; the noun of a group comes from the page (views.groupLabel)
export const viewLabel = (view, groupLabel = 'Group') =>
  ({
    [VIEW_LIST]: 'List',
    [VIEW_GROUPED]: `By ${groupLabel.toLowerCase()}`,
    [VIEW_MATRIX]: 'Matrix',
  })[view] || view

// Kinds of an empty cell of the matrix
export const CELL_NOT_CONFIGURED = 'not-configured'
export const CELL_NOT_SHOWN = 'not-shown'
export const CELL_NOT_IN_ENVIRONMENT = 'not-in-environment'

// normalizeViews returns the views of a payload, with the defaults of a payload without them
export const normalizeViews = (views) => {
  const enabled = Array.isArray(views?.enabled) && views.enabled.length > 0 ? views.enabled : [VIEW_LIST]
  return {
    enabled,
    default: enabled.includes(views?.default) ? views.default : enabled[0],
    groupLabel: views?.groupLabel || 'Group',
    environments: Array.isArray(views?.environments) ? views.environments : [],
    applications: Array.isArray(views?.applications) ? views.applications : [],
  }
}

// resolveView returns the view to show: the requested one when the page enables it, else the default of the page
export const resolveView = (views, requested) => {
  const normalized = normalizeViews(views)
  return normalized.enabled.includes(requested) ? requested : normalized.default
}

// aggregateStatus mirrors aggregateStatus of the backend: unknown endpoints are ignored; all up is operational, all down
// is down, any other mix (pending included) is degraded, and nothing with results is unknown
export const aggregateStatus = (statuses) => {
  let up = 0
  let down = 0
  let pending = 0
  for (const status of statuses) {
    if (status === 'up') up++
    else if (status === 'down') down++
    else if (status === 'pending') pending++
  }
  if (up === 0 && down === 0 && pending === 0) return 'unknown'
  if (down === 0 && pending === 0) return 'operational'
  if (up === 0 && pending === 0) return 'down'
  return 'degraded'
}

// isProblem is whether an endpoint counts as a problem: down or pending
export const isProblem = (endpoint) => endpoint.status === 'down' || endpoint.status === 'pending'

// groupStatuses returns the status of every group of the views, featured endpoints included, by group name: the status
// that decides whether a group of the grouped view may stay collapsed
export const groupStatuses = (payload) =>
  new Map(collectGroups(payload).map((group) => [group.name, aggregateStatus(group.endpoints.map((endpoint) => endpoint.status))]))

// collectGroups returns the groups of the payload with their endpoints, the featured ones placed back in their group
// (a group reached only through featured endpoints is added after the others), each endpoint with its group name
export const collectGroups = (payload) => {
  const groups = []
  const byName = new Map()
  for (const group of payload?.groups || []) {
    const entry = { name: group.name || '', description: group.description || '', tags: group.tags || [], endpoints: [] }
    for (const endpoint of group.endpoints || []) {
      entry.endpoints.push({ ...endpoint, group: entry.name })
    }
    groups.push(entry)
    byName.set(entry.name, entry)
  }
  for (const featured of payload?.featured || []) {
    const name = featured.group || ''
    let entry = byName.get(name)
    if (!entry) {
      // A group reached only through featured endpoints: its details come in views.groups
      const details = (payload?.views?.groups || []).find((group) => group.name === name)
      entry = { name, description: details?.description || '', tags: details?.tags || [], endpoints: [] }
      groups.push(entry)
      byName.set(name, entry)
    }
    entry.endpoints.push({ ...featured, group: name })
  }
  for (const entry of groups) {
    entry.endpoints.sort((a, b) => a.name.toLowerCase().localeCompare(b.name.toLowerCase()))
  }
  return groups
}

// orderedApplications returns the applications in display order: the ones listed by the page first, in its order, then
// the others alphabetically. Only applications with an endpoint are returned.
export const orderedApplications = (views, groups) => {
  const present = new Set()
  for (const group of groups) {
    for (const endpoint of group.endpoints) {
      present.add(endpoint.application || endpoint.name)
    }
  }
  const listed = normalizeViews(views).applications.filter((application) => present.has(application))
  const others = [...present]
    .filter((application) => !listed.includes(application))
    .sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase()))
  return [...listed, ...others]
}

// environmentOrder returns the position of an environment id in the order of the page; endpoints without environment
// (or with an id that the payload does not declare) come last
const environmentOrder = (environments, id) => {
  const index = environments.findIndex((environment) => environment.id === id)
  return index < 0 ? environments.length : index
}

// matchesFilters is whether an endpoint passes the filters of environment and application (ALL means any)
const matchesFilters = (endpoint, filters) =>
  (!filters?.environment || endpoint.environment === filters.environment) &&
  (!filters?.application || (endpoint.application || endpoint.name) === filters.application)

// counters returns the counters of the cards and of the banner, over the whole published page (never filtered): the
// groups, the monitors, the down endpoints in the primary environment and elsewhere (endpoints without environment count
// as elsewhere), the groups with a problem, and the counts by status
export const counters = (payload) => {
  const views = normalizeViews(payload?.views)
  const primary = views.environments[0]?.id
  const groups = collectGroups(payload)
  const result = {
    groups: groups.filter((group) => group.endpoints.length > 0).length,
    monitors: 0,
    primaryDown: 0,
    otherDown: 0,
    affectedGroups: 0,
    up: 0,
    down: 0,
    pending: 0,
    unknown: 0,
    hasEnvironments: views.environments.length > 0,
  }
  for (const group of groups) {
    let affected = false
    for (const endpoint of group.endpoints) {
      result.monitors++
      result[['up', 'down', 'pending'].includes(endpoint.status) ? endpoint.status : 'unknown']++
      if (endpoint.status === 'down') {
        if (primary && endpoint.environment === primary) result.primaryDown++
        else result.otherDown++
      }
      if (isProblem(endpoint)) affected = true
    }
    if (affected) result.affectedGroups++
  }
  return result
}

// groupedModel returns the groups of the grouped view after the filters: each group with its status (over its
// filtered endpoints), the short names of its environments, its applications with their status, and its endpoints split
// by environment in the order of the page (endpoints without environment last). Groups left without endpoint are not
// returned.
export const groupedModel = (payload, filters = {}) => {
  const views = normalizeViews(payload?.views)
  const environments = views.environments
  const model = []
  for (const group of collectGroups(payload)) {
    const endpoints = group.endpoints.filter((endpoint) => matchesFilters(endpoint, filters))
    if (endpoints.length === 0) continue
    if (filters.onlyProblems && !endpoints.some(isProblem)) continue
    const sections = new Map()
    for (const endpoint of endpoints) {
      const id = environments.some((environment) => environment.id === endpoint.environment) ? endpoint.environment : ''
      if (!sections.has(id)) sections.set(id, [])
      sections.get(id).push(endpoint)
    }
    const orderedSections = [...sections.entries()]
      .sort(([a], [b]) => environmentOrder(environments, a) - environmentOrder(environments, b))
      .map(([id, sectionEndpoints]) => {
        const environment = environments.find((candidate) => candidate.id === id) || null
        const statuses = sectionEndpoints.map((endpoint) => endpoint.status)
        return {
          id,
          environment,
          endpoints: [...sectionEndpoints].sort((a, b) => applicationCompare(views, a, b)),
          up: statuses.filter((status) => status === 'up').length,
          down: statuses.filter((status) => status === 'down').length,
          pending: statuses.filter((status) => status === 'pending').length,
        }
      })
    const applications = new Map()
    for (const endpoint of endpoints) {
      const application = endpoint.application || endpoint.name
      if (!applications.has(application)) applications.set(application, [])
      applications.get(application).push(endpoint.status)
    }
    const order = orderedApplications(payload?.views, [{ endpoints }])
    const statuses = endpoints.map((endpoint) => endpoint.status)
    model.push({
      name: group.name,
      description: group.description,
      tags: group.tags,
      status: aggregateStatus(statuses),
      up: statuses.filter((status) => status === 'up').length,
      down: statuses.filter((status) => status === 'down').length,
      pending: statuses.filter((status) => status === 'pending').length,
      environments: orderedSections.filter((section) => section.environment).map((section) => section.environment.short),
      applications: order.map((name) => ({ name, status: aggregateStatus(applications.get(name)) })),
      sections: orderedSections,
    })
  }
  return model
}

// applicationCompare orders two endpoints by application (in the order of the page), then by name
const applicationCompare = (views, a, b) => {
  const order = views.applications
  const applicationA = a.application || a.name
  const applicationB = b.application || b.name
  const indexA = order.indexOf(applicationA)
  const indexB = order.indexOf(applicationB)
  if (applicationA !== applicationB) {
    if (indexA >= 0 && indexB >= 0) return indexA - indexB
    if (indexA >= 0) return -1
    if (indexB >= 0) return 1
    return applicationA.toLowerCase().localeCompare(applicationB.toLowerCase())
  }
  return a.name.toLowerCase().localeCompare(b.name.toLowerCase())
}

// cellUptime returns the 30-day uptime of a cell: the lowest non-null value of its endpoints in the primary environment,
// else in the first environment (in the order of the page, then without environment) that has a non-null value, else null
const cellUptime = (environments, endpoints) => {
  const byEnvironment = new Map()
  for (const endpoint of endpoints) {
    const value = endpoint.uptime?.['30d']
    if (value === null || value === undefined) continue
    const id = endpoint.environment || ''
    byEnvironment.set(id, Math.min(byEnvironment.has(id) ? byEnvironment.get(id) : Infinity, value))
  }
  const ids = [...byEnvironment.keys()].sort((a, b) => environmentOrder(environments, a) - environmentOrder(environments, b))
  return ids.length > 0 ? byEnvironment.get(ids[0]) : null
}

// matrixModel returns the rows (groups) and the columns (applications) of the matrix after the filters. A cell has the
// aggregated status of the endpoints of its pair in the filtered environments, the 30-day uptime and the short names of
// the environments present; an empty cell says why: not configured, not shown (truncated page) or not in the filtered
// environment. The only-problems filter keeps the rows with a problem in the filtered environments.
export const matrixModel = (payload, filters = {}) => {
  const views = normalizeViews(payload?.views)
  const environments = views.environments
  const groups = collectGroups(payload)
  const applicationFilter = filters.application
  const columns = orderedApplications(payload?.views, groups).filter(
    (application) => !applicationFilter || application === applicationFilter,
  )
  const rows = []
  for (const group of groups) {
    const filtered = group.endpoints.filter((endpoint) => matchesFilters(endpoint, filters))
    if (filters.onlyProblems && !filtered.some(isProblem)) continue
    if (group.endpoints.length === 0) continue
    const cells = columns.map((application) => {
      const all = group.endpoints.filter((endpoint) => (endpoint.application || endpoint.name) === application)
      const endpoints = all.filter((endpoint) => !filters.environment || endpoint.environment === filters.environment)
      if (endpoints.length === 0) {
        let kind = CELL_NOT_CONFIGURED
        if (all.length > 0) kind = CELL_NOT_IN_ENVIRONMENT
        else if (payload?.truncated) kind = CELL_NOT_SHOWN
        return { application, kind, endpoints: [] }
      }
      const shorts = []
      for (const environment of environments) {
        if (endpoints.some((endpoint) => endpoint.environment === environment.id)) shorts.push(environment.short)
      }
      return {
        application,
        kind: 'cell',
        status: aggregateStatus(endpoints.map((endpoint) => endpoint.status)),
        uptime: cellUptime(environments, endpoints),
        environments: shorts,
        endpoints,
      }
    })
    rows.push({ name: group.name, description: group.description, tags: group.tags, cells })
  }
  return { columns, rows }
}

// filterOptions returns the environments and the applications that the chips offer: the declared environments that have
// an endpoint on the page, and the applications in display order
export const filterOptions = (payload) => {
  const views = normalizeViews(payload?.views)
  const groups = collectGroups(payload)
  const used = new Set(groups.flatMap((group) => group.endpoints.map((endpoint) => endpoint.environment)))
  return {
    environments: views.environments.filter((environment) => used.has(environment.id)),
    applications: orderedApplications(payload?.views, groups),
  }
}

// sanitizeFilters drops a filter value that the payload does not offer anymore, after a refresh
export const sanitizeFilters = (payload, filters) => {
  const options = filterOptions(payload)
  return {
    environment: options.environments.some((environment) => environment.id === filters.environment) ? filters.environment : ALL,
    application: options.applications.includes(filters.application) ? filters.application : ALL,
    onlyProblems: filters.onlyProblems === true,
  }
}
