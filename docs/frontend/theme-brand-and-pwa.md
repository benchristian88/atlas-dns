# Theme, Brand, and Browser Install Metadata

## Current scope

Release 1.0 completes both the visual and technical Atlas DNS Controller
identity. Repository, module, binary, service, image, environment, config-path,
manifest, and release-artifact identifiers use the documented Atlas forms.
Stable `/api/v1` paths and domain terms that correctly describe managed AdGuard
Home nodes are deliberately unchanged.

This work is frontend-only. Theme preference is browser-local presentation
state; it does not create a controller setting, database record, API contract,
audit event, revision, or deployment.

## Theme architecture

`web/src/theme/ThemeProvider.tsx` is the runtime source of truth. It exposes:

- `System`, the default, resolved through `prefers-color-scheme`;
- explicit `Light` and `Dark` preferences;
- the resolved light/dark theme used by theme-specific artwork; and
- one setter used by the header control.

The preference is stored under `atlas-dns.theme` in browser
`localStorage`. Invalid, unavailable, or unwritable storage falls back to
System without blocking the application. System registers a media-query
change listener; explicit Light or Dark does not react to OS changes.

The small blocking same-origin `web/public/theme-init.js` script, loaded from
the document head, mirrors the provider's validated preference resolution
before React and CSS load. It complies with the production `script-src 'self'`
policy without enabling inline script. It sets `data-theme`,
`data-theme-preference`, `color-scheme`, and browser `theme-color` early to
avoid an avoidable wrong-theme flash. React then owns subsequent changes.
Components must not read or write the theme storage key themselves.

Theme values remain semantic CSS custom properties in
`web/src/styles/design-tokens.css`. Atlas Blue is the primary interaction
colour, while success, information, warning, and danger retain separate
semantic colours. Atlas Teal is a brand colour and is not a replacement for
success.

The shared surface hierarchy is page canvas → primary card → subtle/inset
surface, with popup as the elevated overlay surface. Light uses `#F3F5F7` →
`#FFFFFF` → `#E9EEF3`; Dark uses `#151A20` → `#1D242D` → `#252E38`.
Shared border and semantic foreground/soft/border mappings are documented in
`design-system.md`. Pages and feature components must not introduce local
theme colours.

## Theme control and artwork

The compact theme icon button is in the utility top bar beside notification and
account actions and remains available beside the mobile drawer button. Its
icon and accessible name reflect the selected preference. Activating it opens a
three-option menu for Light, Dark, and System with checked state, arrow/Home/End
navigation, Escape close/focus return, outside-click close, and ordinary touch
activation. System is always explicit; there is no hidden gesture.

The header and authentication layout use the supplied light/dark Atlas DNS
lockup SVGs. The header uses the approved symbol-only fallback at phone widths.
The desktop lockup is 200 px wide and 52 px high, the login lockup is constrained
to 340 px, and the approved symbol-only asset is used at phone widths. The
link's accessible name remains `Atlas DNS Controller dashboard`; decorative
artwork is hidden from the accessibility tree.

## Approved assets

Canonical design/source files stay outside the browser public root:

```text
assets/branding/source/Atlas_Brand_Assets_v3/
assets/branding/source/final-lockups/
```

Runtime SVGs are intentionally limited to:

```text
web/public/branding/atlas-mark-white.svg
web/public/branding/atlas-mark-black.svg
web/public/branding/atlas-mark-color-light.svg
web/public/branding/atlas-mark-color-dark.svg
web/public/branding/atlas-dns-lockup-light.svg
web/public/branding/atlas-dns-lockup-dark.svg
```

The four mark files are byte-identical to the V3 `PRIMARY_ANGLED_GAP` source.
The final supplied PNG lockups are source masters, not public runtime files.
CSS filters or inversion are not used to create theme variants.

## Favicon, Apple, and PWA behavior

The sole runtime icon system is:

```text
web/public/favicon.ico
web/public/favicon.svg
web/public/favicon-16x16.png
web/public/favicon-32x32.png
web/public/apple-touch-icon.png
web/public/android-chrome-192x192.png
web/public/android-chrome-512x512.png
web/public/manifest.webmanifest
```

`index.html` advertises ICO, SVG, 32px, 16px, and Apple variants. The manifest
retains the Atlas DNS Controller name, root identity/scope, standalone display,
and only the approved opaque 192px and 512px icons. It does not claim maskable
safe-area behavior that the source manifest does not specify. No service
worker, offline cache, or background update mechanism is introduced.
The viewport uses `viewport-fit=cover`; the authenticated shell applies safe-area
insets in Safari and standalone mode while leaving pinch zoom enabled.

Run `npm run test:assets` in `web/` to validate manifest JSON and naming,
runtime/source equality, exact PNG dimensions, HTML references, the approved
runtime branding set, and removal of the legacy neutral icon references.

## Accessibility and browser validation

- Theme selection uses a labelled button/menu, checked preference state, visible
  focus styling, keyboard navigation, and a 44px touch target.
- Menus remain available through click/touch and keyboard without hover.
- Theme-specific SVGs are selected as assets rather than transformed.
- Status meaning remains text-backed and independent from brand colour.
- Header and login layouts retain max-width constraints and symbol fallback.

Automated DOM, Axe, asset, type, lint, and production-build checks do not prove
real-browser contrast, iOS saved-app masking, standalone launch, or browser
chrome behavior. Those checks remain explicit external release evidence.
