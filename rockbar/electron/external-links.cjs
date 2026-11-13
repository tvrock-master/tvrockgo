// Program content may only open web pages in the user's default browser.
async function openProgramLink(shell, value) {
  try {
    const url = new URL(value);
    if (!['http:', 'https:'].includes(url.protocol) || url.username || url.password) return false;
    await shell.openExternal(url.href);
    return true;
  } catch { return false; }
}

module.exports = { openProgramLink };
