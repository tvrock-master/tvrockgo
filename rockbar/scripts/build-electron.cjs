const { spawnSync } = require('node:child_process');
const { clientVersion } = require('./client-version.cjs');

const { setReleaseVersion } = require('./set-release-version.cjs');
const tag = process.env.ROCKBAR_VERSION || process.env.APP_VERSION || process.env.CI_COMMIT_TAG ||
  (process.env.GITHUB_REF_TYPE === 'tag' ? process.env.GITHUB_REF_NAME : '');
if (tag && tag !== 'dev') setReleaseVersion(tag);
const executable = process.platform === 'win32' ? 'electron-builder.cmd' : 'electron-builder';
const result = spawnSync(executable, [
  '--win', 'nsis', '--x64', '--publish', 'never',
  '--config.extraMetadata.tvrockVersion=' + clientVersion(),
], { stdio: 'inherit', shell: process.platform === 'win32' });

if (result.error) {
  console.error(result.error);
  process.exit(1);
}
process.exit(result.status ?? 1);
