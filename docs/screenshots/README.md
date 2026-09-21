# Screenshots

The screens of [jniltinho/go-uptime](https://github.com/jniltinho/go-uptime), captured at 1280×900 by
[capture.sh](capture.sh), which starts a local instance, registers endpoints, a status page and push keys through the
administration and lets the history fill before taking the pictures: run it again whenever a screen changes. Dark mode
is the default; the public pages are shown in light mode, which every visitor can switch with the button in the header.
The logo in the header is optional (`ui.logo`).

## Dashboard

The monitored endpoints, with the bars of the last checks, the average response time and the search, filter and sort
controls.

![Dashboard](dashboard.png)

## Bio theme

The third theme, next to dark and light: teal, blue and navy, with a navy header. Each visitor picks a theme with the
selector in the header (or in the settings bar of the dashboard), and `ui.default-theme: bio` makes it the default.

![Dashboard in the bio theme](dashboard-bio.png)

![Public status page in the bio theme](status-page-bio.png)

## Endpoint details

Bars of the recent checks, the panel of numbers and the **Response Time Trend** chart in the format of the Uptime
Kuma, with the periods Recent, 3h, 6h, 24h and 1w. In the aggregate periods the legend names the three lines, and the
minimum and the maximum are drawn dashed and dotted so that they can be told apart.

![Endpoint details with the response time chart](endpoint-details.png)

## Login screen

`security.basic` uses a login page with logout instead of the dialog of the browser. The session is stored in the
database, failed logins are rate limited, and `curl -u` keeps working for the API and for scripts.

![Login screen](login.png)

## Endpoint administration

Endpoints created through the web and endpoints from the configuration file, side by side. The ones from the file are
read-only.

![Endpoint administration](admin-endpoints.png)

## Status pages

Managed status pages and the ones defined in the configuration file, with their address, state and number of
endpoints. A page that asks for a login of its own is marked with a padlock.

![Administration of the status pages](admin-status-pages.png)

The form fills the window: General and Groups on the left, the list of endpoints on the right, each column scrolling
on its own, with Validate, Preview and Save always visible. **Require login to view this page** asks the visitor for a
username and a password of that page, which the browser requests as in `security.basic`.

![Form of a status page](admin-status-page-form.png)

## Push keys

Global keys for push monitoring, created and revoked through the web, next to the keys defined in the configuration
file.

![Push keys](admin-push-keys.png)

## Backup and restore

A JSON file with the endpoints, status pages and push keys, optionally encrypted with a password, that can be
restored on another installation with any supported database. The restore shows a preview of what it will create,
update or skip before anything is written.

![Backup and restore](admin-backup.png)

## Public status page

Open without login at `/status/<slug>`, with featured endpoints, uptime of 24h, 7d and 30d aligned in columns, and
the days left until the TLS certificate expires. The banner counts how many endpoints are up and down.

![Public status page](status-page.png)

The same page in dark mode:

![Public status page in dark mode](status-page-dark.png)

## Public endpoint details

Every endpoint of a status page links to a public details page with its history, its response time chart and,
when the page allows it, the table of checks with the messages. A check that failed without answering shows why —
`Certificate error`, `DNS error`, `Timeout`, `Connection failed` or `Check failed` — never the error itself.

![Public details of an endpoint](status-page-endpoint.png)

## How these were taken

A local Go Uptime with `admin.enabled`, `security.basic` and a handful of endpoints, captured with
[agent-browser](https://www.npmjs.com/package/agent-browser) at 1280×900. The end-to-end scripts in `test/e2e/` save
their own screenshots under `dist/prints/`, which stays out of git; the ones in this folder are curated by hand and
should be retaken whenever a screen changes shape.
