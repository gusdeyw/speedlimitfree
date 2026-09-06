# SpeedLimitFree: compact desktop redesign

Design exploration and implementation, 2026-09-06. The approved compact table is implemented in the production Svelte interface for 0.2.0. The original standalone concept remains available for comparison. Service management and traffic behavior retain their existing APIs.

The production app follows the Windows app theme with CSS `prefers-color-scheme`; the native Wails frame uses `windows.SystemDefault`. There is no manual theme preference. Go extracts 32 px executable icons through the Windows shell, releases native icon/bitmap resources, and returns PNG data URIs. Successful and failed results are cached in bounded memory. Visible rows load icons lazily, with at most two requests in flight; no process icon files are written or downloaded. A generic icon covers missing/inaccessible executables. Hosted applications may share their executable's icon.

## Recommendation

Make the application a compact desktop utility centered on one application table. The main task is **find the application → inspect its speed → change its download or upload limit**. The table should occupy most of the window immediately after opening.

Use NetLimiter's application hierarchy and directional limit controls as the interaction reference. Use a restrained Windows-style surface, system typography, clear column labels, and a collapsible details strip. The proposed light and dark versions share the same structure.

Open [the clickable concept](design/table-concept.html) directly in a browser. It works offline with sample data; no service or network calls are made. Try searching, sorting column headers, expanding Chrome, editing a limit, pausing limits, showing details, and changing the theme. Reload to reset the example values.

- [Light overview](design/table-light.png)
- [Limit editing](design/table-edit-limit.png)
- [Dark overview](design/table-dark.png)
- [920 × 640 layout](design/table-small.png)

## Reference analysis

| Reference | What fits this app | What should stay out of the default view |
| --- | --- | --- |
| [NetLimiter Activity](https://www.netlimiter.com/docs/user-gui-client/child-windows/activity) | Application/process hierarchy, sortable rates, compact filtering and row actions | Connection-level detail, multiple network zones and dense collections of firewall/status icons |
| [NetLimiter Limits](https://www.netlimiter.com/docs/basic-concepts/limits) | Separate download and upload columns; clicking a limit opens its editor | A large advanced editor for every basic speed change |
| [NetBalancer main window](https://netbalancer.com/download) | A process list as the central working area; toolbar access to traffic controls | Persistent graph space, priorities, adapters and network-rule tools that this app does not currently expose |
| [Microsoft TCPView](https://learn.microsoft.com/en-us/sysinternals/downloads/tcpview) | Familiar desktop table, compact toolbar, ownership information and contextual operations | Endpoint/IP/protocol columns on the main application list |

NetLimiter's [Info View](https://www.netlimiter.com/docs/user-gui-client/child-windows/info-view) changes with the selected entity. Our equivalent is a details strip that opens on demand, preserving width for the main table.

These are interaction and layout references, not claims of feature parity. NetLimiter's documentation is older and the screenshot currently served on its homepage is labeled NetLimiter 4 Pro. The local reference screenshots are identified accordingly; they are not presented as verified NetLimiter 5 screens.

Official screenshots inspected:

- [NetLimiter main screenshot](https://www.netlimiter.com/img/hero.png), [local reference](design/references/netlimiter-main.png).
- [NetLimiter Activity screenshot](https://www.netlimiter.com/img/docs/Activity-4.0.56.PNG), [local reference](design/references/netlimiter-activity.png).

Screenshots and extracted application icons belong to their respective owners and are included only in this local design exploration. Production icon loading should use each application's installed icon, with a generic fallback.

## Current interface audit

The current implementation uses Svelte 5, TypeScript, plain CSS, Phosphor icons and Wails. This stack can support the redesign directly.

| Current pattern | Consequence | Proposed change |
| --- | --- | --- |
| Persistent left sidebar, workspace breadcrumb, branding and explanatory copy | Consumes horizontal space without helping identify or limit an application | Small top bar: Applications, Saved limits, service control and Settings |
| Large heading and subtitle | Pushes the process list down | Open directly into search, filters and the table |
| Always-visible global metrics and traffic chart | Occupies substantial space before the main task | Compact download/upload totals in the bottom status bar; optional chart later |
| 64 px rows with large initials tiles and two lines of labels | Too few applications visible at once | 32 px rows, small installed application icons, process count in its own column |
| Download/upload limits stacked in a single column | Harder to compare one direction across multiple applications | Two aligned, editable limit columns |
| Generic action icon opens a full rule drawer | Extra indirection for a common value change | Click the desired limit cell; edit its value and unit in an anchored popover |
| Separate app/process modes | A user loses context when investigating one application's processes | Expand the application group in place |
| Large repeated explanatory banners | Creates visual noise after setup is complete | Compact status; prominent inline messages only for actionable errors |
| Large status badges for unrestricted applications | Adds visual weight to rows needing no attention | Subtle status text; emphasize limited, paused, overridden, unavailable or failed states |

Keep the behavior already implemented: service lifecycle controls, tray behavior, persisted application rules, temporary process overrides, filtering, named-pipe errors, virtualization and suspended hidden updates.

## Proposed main window

1. **Top bar, 44 px:** identity, Applications / Saved limits, compact service status and Settings. Appearance follows Windows automatically.
2. **Toolbar, 50 px:** persistent search, All running / Network active / Limited, Pause limits, Add application, toggle details.
3. **Table:** Application, Processes/PID, Download, Upload, Download limit, Upload limit, Status. Sticky header, sortable columns, independently scrolling body.
4. **Optional details strip:** executable path, application/process scope, enable/disable and remove the selected rule.
5. **Status bar, 28 px:** matching application/process counts and aggregate traffic. Technical accounting details belong in a tooltip or diagnostics.

The measured concept shows 20 complete application rows at 1280 × 820, with 85% of the window allocated to the table region. This is a layout measurement, not a runtime performance result. At 920 × 640, all default columns fit without page overflow.

## Interaction rules for implementation

- Search names, executable paths and PID. Ctrl+K or / focuses search; Escape clears or cancels the active editor.
- Keep alphabetical order as the initial stable list. Clicking a rate header sorts by that rate. Avoid moving an actively edited row as new measurements arrive.
- Group by executable path, not display name. Expand one application to inspect its running process instances.
- Application limits share their budget across matching instances. A child process edit creates a temporary override, clearly identified in the editor and row.
- Clicking a download/upload limit edits only that direction. Preserve the other direction and rule identity when calling `SaveRule`.
- Show a value and unit selector, a few useful presets, Apply, and Unlimited. Zero is a blocking value in some systems, so never silently treat zero as unlimited; use the explicit Unlimited action.
- The current contract has one enabled state per rule. Enable/disable therefore applies to both configured directions. Do not introduce independent directional checkboxes that imply unsupported backend behavior.
- Inherited application values in process rows must be distinguishable from a process override. Parent rows must expose the presence of overrides.
- Saved limits must include applications that are not running and rules on individual processes. Adding an executable creates a rule without claiming that process is running.
- Preserve the input while measurements refresh. Show pending save and failures at the edited control. Update the displayed value only from a successful backend response.
- Offline/monitor mode must show unavailable rates. A saved pending rule must not look actively enforced. Paused limits, disabled rules and a stopped service remain distinct states.
- Keep Start, Stop, Restart, Install / repair and recent logs in service settings. Always await the existing checked management API. The status indicator can open these controls directly.
- No connection count, historical totals, priorities, firewall controls or adapter filters until their underlying data and behavior exist.

## Visual and performance choices

- Windows system font stack, 12–13 px table text, tabular numerals, right-aligned numeric columns.
- Neutral gray surfaces and one blue selection/action accent. Small application icons provide recognition.
- Thin dividers, 3–4 px control corners, light hover and keyboard focus states. No decorative animation, gradients or dashboard cards.
- Light and dark themes use the same tokens and hierarchy. Support the Windows preference in the production implementation.
- Continue using Svelte and plain CSS. No new UI framework, component library or font download is needed.
- Preserve virtualization, changing its row measurement to match the selected density. Keep stable keys based on executable path or PID plus creation time.
- Cache executable icons in Go and load them lazily; never extract them on every traffic update. Use bounded caches and a generic icon when inaccessible.
- Keep measured data events bounded and suspend updates when hidden. A graph, if added to the details area, should exist only while opened.

## Applying the concept to the app

1. Extract the current service controls and rule operations into reusable components without changing their API behavior.
2. Replace the sidebar/overview composition with the compact top bar and table.
3. Split the two limit columns and add the anchored directional editor using `SaveRule`, preserving the other direction.
4. Flatten expanded application/process groups for the existing virtualized list; retain process identity and override precedence.
5. Add details/settings panels, keyboard navigation, native icon caching and automatic system appearance. Custom column preferences remain a future option.
6. Re-run the existing rule, failure, service and native tray tests; add meaningful checks for inline edits, inherited limits, grouping and virtualization at the new density.

The standalone concept demonstrates the design and primary interactions. It is not the production implementation: it uses sample data, simplified in-memory rules and a small unvirtualized fixture list. Its simulated service buttons never affect Windows. See [preview checks](design/preview-checks.json) for the tested interactions.
