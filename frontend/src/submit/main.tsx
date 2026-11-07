// Public submission site. Built from the same codebase as the staff app
// (RALLY_APP=submit) but signs in against the attendee realm; its
// /config.json carries that realm and client id.
import { loadRuntimeConfig, renderStartupError } from '@/runtime-config';

const rootEl = document.getElementById('root');
if (!rootEl) {
  throw new Error('#root element missing from index.html');
}

// The app is imported only after /config.json is in, so nothing (OIDC
// client, API client, router) can read a setting before it is known.
loadRuntimeConfig()
  .then(() => import('./app'))
  .then(({ startApp }) => startApp(rootEl))
  .catch((err: unknown) => renderStartupError(rootEl, err));
