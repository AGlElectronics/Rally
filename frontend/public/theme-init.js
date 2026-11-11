// Apply the cached light/dark choice before first paint (see src/lib/theme.ts).
// A file rather than an inline script: the Content-Security-Policy allows
// scripts from the app's own origin only.
(function () {
  var p = 'system';
  try {
    p = localStorage.getItem('cs-theme') || localStorage.getItem('rally-theme') || 'system';
  } catch (e) {}
  var dark = p === 'dark' || (p !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches);
  document.documentElement.classList.toggle('dark', dark);
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light';
})();
