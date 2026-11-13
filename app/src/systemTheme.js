// Keep Vuetify, custom CSS and native controls on the selected or OS theme.
function followSystemTheme(theme, getPreference = () => 0, browser = window, root = document.documentElement) {
  const query = browser.matchMedia('(prefers-color-scheme: dark)');
  const apply = () => {
    const preference = Number(getPreference());
    const dark = preference === 2 || (preference !== 1 && query.matches);
    theme.dark = dark;
    root.dataset.colorScheme = dark ? 'dark' : 'light';
    root.style.colorScheme = dark ? 'dark' : 'light';
  };
  apply();
  if (query.addEventListener) query.addEventListener('change', apply);
  else query.addListener(apply);
  return {
    refresh: apply,
    dispose() {
      if (query.removeEventListener) query.removeEventListener('change', apply);
      else query.removeListener(apply);
    },
  };
}

module.exports = { followSystemTheme };
