const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { normalizeReleaseVersion, setReleaseVersion } = require('../scripts/set-release-version.cjs');

test('release tags retain prerelease and build metadata', () => {
  for (const [tag, expected] of [
    ['v1.2.3', '1.2.3'], ['2.0.0', '2.0.0'],
    ['v1.2.3-rc.1', '1.2.3-rc.1'], ['v1.2.3-rc.1+build.5', '1.2.3-rc.1+build.5'],
  ]) assert.equal(normalizeReleaseVersion(tag), expected);
  for (const tag of ['dev', 'release/1.2.3', 'v1.2.3garbage', 'v01.2.3', 'v1.2.3-01', 'v1.2', 'v1.2.3+']) {
    assert.throws(() => normalizeReleaseVersion(tag));
  }
});

test('package and lock versions are updated together without changing dependencies', t => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'rockbar-version-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const pkg = { name: 'rockbar', version: '1.0.0', dependencies: { ws: '^8.18.3' } };
  const lock = { version: '1.0.0', packages: { '': { version: '1.0.0' }, 'node_modules/ws': { version: '8.18.3' } } };
  fs.writeFileSync(path.join(directory, 'package.json'), JSON.stringify(pkg));
  fs.writeFileSync(path.join(directory, 'package-lock.json'), JSON.stringify(lock));
  assert.equal(setReleaseVersion('v2.3.4-beta.2', directory), '2.3.4-beta.2');
  const updatedPackage = JSON.parse(fs.readFileSync(path.join(directory, 'package.json')));
  const updatedLock = JSON.parse(fs.readFileSync(path.join(directory, 'package-lock.json')));
  assert.equal(updatedPackage.version, '2.3.4-beta.2');
  assert.equal(updatedLock.version, updatedPackage.version);
  assert.equal(updatedLock.packages[''].version, updatedPackage.version);
  assert.deepEqual(updatedPackage.dependencies, pkg.dependencies);
  assert.deepEqual(updatedLock.packages['node_modules/ws'], lock.packages['node_modules/ws']);
});

test('invalid lockfile or tag does not partially update package.json', t => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'rockbar-version-'));
  t.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const packagePath = path.join(directory, 'package.json');
  const original = '{"version":"1.0.0"}';
  fs.writeFileSync(packagePath, original);
  fs.writeFileSync(path.join(directory, 'package-lock.json'), '{}');
  assert.throws(() => setReleaseVersion('v2.0.0', directory));
  assert.throws(() => setReleaseVersion('invalid', directory));
  assert.equal(fs.readFileSync(packagePath, 'utf8'), original);
});
