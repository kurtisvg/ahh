# Browser Terminal Assets

The following files are vendored for the browser PTY terminal so the Ahh server
can serve the UI without depending on a CDN or frontend build pipeline:

- `xterm.css` from `@xterm/xterm` version `5.5.0`
- `xterm.js` from `@xterm/xterm` version `5.5.0`
- `addon-fit.js` from `@xterm/addon-fit` version `0.10.0`

These upstream packages are MIT licensed. Keep the upstream copyright and
license headers in copied files.

Planned replacement: remove these checked-in vendored assets and this notice
once Ahh has a frontend dependency pipeline, such as `package.json` plus a
repeatable copy/build step that pins and copies the needed files from
`node_modules`. At that point, dependency metadata should live in the frontend
package files instead of this notice.
