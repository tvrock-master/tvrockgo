// Match the server's embedded version, preserving dev for untagged builds.
function clientVersion(env = process.env) {
  return (env.APP_VERSION || env.ROCKBAR_VERSION || env.CI_COMMIT_TAG ||
    (env.GITHUB_REF_TYPE === 'tag' ? env.GITHUB_REF_NAME : '') || 'dev').replace(/^v/, '');
}
module.exports = { clientVersion };
