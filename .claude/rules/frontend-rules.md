# Frontend Rules (PART 16, 17)

⚠️ **These rules are NON-NEGOTIABLE. Violations are bugs.** ⚠️

## CRITICAL - NEVER DO
- Never alter AI.md’s shared shell, stylesheet order, breakpoints, component catalog, or accessibility behavior by local convention.
- Never use unlayered CSS, magic z-index values, ID selectors, `!important` outside print, physical directional properties, or unlisted utility classes.
- Never let a fixed drawer live inside a transformed/clipping ancestor or extend the page’s scroll width.
- Never ship a visible missing translation, broken asset, browser console error, overflow, or unlabeled control.

## CRITICAL - ALWAYS DO
- Use Go templates, vanilla JavaScript, and the prescribed shared header, page shell, and admin shell.
- Keep CSS in the four prescribed files and cascade layers: reset/tokens/base/utilities in common.css, shared components in components.css, and page placement in public.css or admin.css.
- Use mobile-first named breakpoints, logical properties, semantic tokens, both themes, RTL, reduced motion, visible focus, and AA contrast.
- Keep the navigation checkbox baseline functional without JavaScript; render the panel and overlay after `<main>` and verify keyboard focus behavior.
- Review all pages across the required viewport, theme, direction, state, and stress-content matrix; verify no horizontal overflow with the backstop disabled.
- Keep server/admin page links, forms, CSRF behavior, docs, and responsive navigation consistent with their routes.

## KEY DECISIONS
| Question | Answer | Spec Reference |
|---|---|---|
| Which CSS breakpoints are standard? | Base, 640, 768, 1024, and 1280 CSS pixels. | PART 16 |
| Where does the mobile drawer render? | As body-level siblings after `<main>`, outside the header. | PART 16 |
| What is the source for admin behavior? | AI.md PART 17; retain shared page shell and defined admin shell. | PART 17 |

For complete details, see AI.md PART 16, 17.
