const fs = require('node:fs');
const path = require('node:path');

function normalizeReleaseVersion(tag) {
  const version = tag.replace(/^v/, '');
  const number = '(?:0|[1-9][0-9]*)';
  const identifier = '(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)';
  const pattern = new RegExp(`^${number}\\.${number}\\.${number}(?:-${identifier}(?:\\.${identifier})*)?(?:\\+[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?$`);
  if (!pattern.test(version)) throw new Error(`Release tag must be a semantic version, e.g. v1.2.3 or v1.2.3-rc.1: ${tag}`);
  return version;
}

function setReleaseVersion(tag, directory = path.resolve(__dirname, '..')) {
  const version = normalizeReleaseVersion(tag);
  const packagePath = path.join(directory, 'package.json');
  const lockPath = path.join(directory, 'package-lock.json');
  const pkg = JSON.parse(fs.readFileSync(packagePath, 'utf8'));
  const lock = JSON.parse(fs.readFileSync(lockPath, 'utf8'));
  if (!lock.packages?.['']) throw new Error('package-lock.json is missing its root package');
  // Parse and validate both files before updating either of them.
  for (const [file, data, changed] of [
    [packagePath, pkg, pkg.version !== version],
    [lockPath, lock, lock.version !== version || lock.packages[''].version !== version],
  ]) {
    if (!changed) continue;
    data.version = version;
    if (file === lockPath) data.packages[''].version = version;
    fs.writeFileSync(file, JSON.stringify(data, null, 2) + '\n');
  }
  return version;
}

if (require.main === module) {
  try {
    if (!process.argv[2]) throw new Error('A release tag is required');
    console.log(setReleaseVersion(process.argv[2]));
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
module.exports = { normalizeReleaseVersion, setReleaseVersion };
