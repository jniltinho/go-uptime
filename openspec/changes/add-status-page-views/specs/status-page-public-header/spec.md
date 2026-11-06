## ADDED Requirements

### Requirement: Slim public header
The header of the public status pages SHALL be a single slim bar of 40 pixels: the logo at 20 pixels, the header text
in the small size and the compact theme selector (icon only), with no other content. It SHALL keep the link of
`ui.link`, the behaviour of the three themes and the navy header of the bio theme.

#### Scenario: Header height
- **WHEN** a public status page is rendered at 1280 pixels of width
- **THEN** the header is at most 44 pixels high

### Requirement: Compact title and summary
The title of a status page SHALL use a smaller size than before (`text-xl`, `text-2xl` from `sm`), its description a
small muted text, and the summary of the page SHALL be a single line with the status, the counts and the time of the
update. The page container SHALL be wide enough for the matrix view.

#### Scenario: Summary on one line
- **WHEN** a page is shown on a screen 1280 pixels wide
- **THEN** the status, the counts and the update time of the summary are on one line

### Requirement: Large screens
The public pages SHALL use the width of Full HD, QHD and 4K screens: the side margins are small: the grouped and matrix
views and the header use up to 1760 pixels with 48 pixels of inner padding, the list `max-w-6xl` and 1400 pixels from 1536; from 2400 pixels of viewport (QHD at 100%
scaling) the public layout is zoomed by 1.25, and from 3200 pixels (4K at 100%) by 1.6, the same thresholds as ad-manager, so that the content keeps its
physical size instead of becoming a thin strip. The administration and the dashboard are not affected.

#### Scenario: Full HD
- **WHEN** the matrix view is shown on a viewport of 1920 pixels
- **THEN** its container is 1760 pixels wide (128 pixels of space on each side) and the zoom is 1

#### Scenario: 4K at 100% scaling
- **WHEN** a public page is shown on a viewport of 3840 pixels
- **THEN** the public layout has a zoom of 1.6 and the content is centred with the same proportions as in Full HD
