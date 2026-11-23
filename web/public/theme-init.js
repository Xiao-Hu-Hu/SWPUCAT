// Run before the application and its styles to avoid a light flash on reload.
;(function () {
  var theme
  try { theme = localStorage.getItem('swpucat-theme') } catch (_) { /* Storage may be blocked. */ }
  var dark = theme === 'dark' || (theme !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.dataset.theme = dark ? 'dark' : 'light'
  document.documentElement.classList.toggle('dark', dark)
  document.documentElement.style.colorScheme = dark ? 'dark' : 'light'
})()
