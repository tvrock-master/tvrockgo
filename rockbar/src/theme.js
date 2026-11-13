(() => {
  const storageKey = 'rockbar.colorTheme';
  const system = window.matchMedia('(prefers-color-scheme: dark)');
  const valid = value => ['0', '1', '2'].includes(value);
  let mode = '0';
  try {
    const saved = localStorage.getItem(storageKey);
    if (valid(saved)) mode = saved;
  } catch {}

  function apply() {
    document.documentElement.dataset.theme = mode === '2' || (mode === '0' && system.matches) ? 'dark' : 'light';
  }
  window.rockbarTheme = {
    get mode() { return mode; },
    setMode(value) {
      if (!valid(value)) return;
      // Keep the current selection if the browser cannot persist it.
      localStorage.setItem(storageKey, value);
      mode = value;
      apply();
    },
  };
  system.addEventListener('change', apply);
  apply();
})();
