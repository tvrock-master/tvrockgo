// Rebuild after changing dependencies:
// npm run build --prefix app -- --dest ../.tmp/license-app-build --report-json
// node scripts/generate-third-party-licenses.cjs --app-report .tmp/license-app-build/report.json
// --check verifies the committed outputs and dependency manifests without rebuilding.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const { execFileSync } = require('node:child_process');
const root = path.resolve(__dirname, '..');
const read = file => fs.readFileSync(path.resolve(root, file), 'utf8').replace(/\r\n/g, '\n');
const hash = text => crypto.createHash('sha256').update(text).digest('hex');
const json = file => JSON.parse(read(file));
const relative = file => path.relative(root, file).replace(/\\/g, '/');
const fontDirectory = 'licenses/external-fonts';
const pluginDirectory = 'licenses/nsis-plugins';
const fontLicenseFiles = item => [item.file, ...(item.additional || [])];
// Normalize presentation whitespace only; retain all copyright and license wording.
const normalize = text => text.replace(/\r\n/g, '\n').replace(/[ \t]+$/gm, '')
  .replace(/^[ \t]+/gm, indent => indent.replace(/\t/g, '    ')).trim();
const notePattern = /^(?:licen[sc]e|copying|notice|copyright|authors|contributors)(?:s|[._-].*)?$/i;
function filesBelow(dir) {
  if (!fs.existsSync(dir)) return [];
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    if (['node_modules', '.git', 'testdata', 'tests', 'test', 'examples', '.github'].includes(entry.name)) return [];
    const file = path.join(dir, entry.name);
    return entry.isDirectory() ? filesBelow(file) : entry.isFile() ? [file] : [];
  });
}
function noticeFiles(dir) {
  return filesBelow(dir).filter(file => notePattern.test(path.basename(file)) ||
    /[\\/](?:licenses|licences)[\\/]/i.test(path.relative(dir, file)));
}
function parseJSONStream(text) {
  const result = []; let start = -1, depth = 0, quoted = false, escaped = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (quoted) { if (escaped) escaped = false; else if (c === '\\') escaped = true; else if (c === '"') quoted = false; }
    else if (c === '"') quoted = true;
    else if (c === '{') { if (depth++ === 0) start = i; }
    else if (c === '}' && --depth === 0) result.push(JSON.parse(text.slice(start, i + 1)));
  }
  return result;
}
function go(args, overrides = {}) {
  return execFileSync('go', args, { cwd: root, encoding: 'utf8', maxBuffer: 100 * 1024 * 1024,
    env: { ...process.env, GOCACHE: path.join(root, '.tmp/gocache'), CGO_ENABLED: '0', ...overrides } });
}
function dependencyInputHashes() {
  const files = [
    'go.sum', 'LICENSE', 'licenses/NSIS-COPYING.txt', 'rockbar/src/assets/playback-icons/LICENSE',
    `${fontDirectory}/sources.json`, `${pluginDirectory}/sources.json`,
    ...json(`${fontDirectory}/sources.json`).flatMap(item => fontLicenseFiles(item).map(file => `${fontDirectory}/${file}`)),
    ...json(`${pluginDirectory}/sources.json`).flatMap(item => item.files.map(file => `${pluginDirectory}/${file}`)),
  ];
  const hashes = new Map([...new Set(files)].map(file => [file, hash(read(file))]));
  // A mirror may rename the main module without changing any external dependencies.
  const goMod = read('go.mod').replace(/^([ \t]*module[ \t]+)\S+/m, '$1<main-module>');
  hashes.set('go.mod', hash(goMod));
  for (const area of ['app', 'rockbar']) {
    const pkg = json(`${area}/package.json`);
    const dependencies = { dependencies: pkg.dependencies, devDependencies: pkg.devDependencies, optionalDependencies: pkg.optionalDependencies };
    hashes.set(`${area}/package.json`, hash(JSON.stringify(dependencies)));
    const lock = json(`${area}/package-lock.json`);
    delete lock.version;
    if (lock.packages?.['']) delete lock.packages[''].version;
    hashes.set(`${area}/package-lock.json`, hash(JSON.stringify(lock)));
  }
  return Object.fromEntries([...hashes.keys()].sort().map(file => [file, hashes.get(file)]));
}
const registry = [], byText = new Map(), components = [];
function add(component, text, origin) {
  text = normalize(text);
  if (!text) throw Error(`Empty notice: ${component.name} / ${origin}`);
  const key = hash(text);
  let notice = byText.get(key);
  if (!notice) { notice = { id: `L${String(registry.length + 1).padStart(4, '0')}`, text, origins: [] }; byText.set(key, notice); registry.push(notice); }
  const label = `${component.name}${component.version ? ' ' + component.version : ''} / ${origin}`;
  if (!notice.origins.includes(label)) notice.origins.push(label);
  if (!component.notices.includes(notice.id)) component.notices.push(notice.id);
}
function component(name, version, kind, source) {
  const c = { name, version, kind, source, notices: [] }; components.push(c); return c;
}
function addFiles(c, dir) {
  for (const file of noticeFiles(dir).sort()) {
    const text = read(file);
    if (text.trim()) add(c, text, path.relative(dir, file).replace(/\\/g, '/'));
  }
}
function goNotices() {
  const modules = new Map(), targets = [['windows', 'amd64'], ['linux', 'amd64'], ['linux', 'arm64'], ['darwin', 'amd64'], ['darwin', 'arm64']];
  for (const [GOOS, GOARCH] of targets) {
    const packages = parseJSONStream(go(['list', '-deps', '-json', '-buildvcs=false', '.'], { GOOS, GOARCH }));
    for (const p of packages) if (p.Module && !p.Module.Main) {
      const m = p.Module.Replace || p.Module;
      if (!modules.has(m.Path)) modules.set(m.Path, { ...m, packages: [] });
      if (!modules.get(m.Path).packages.includes(p.ImportPath)) modules.get(m.Path).packages.push(p.ImportPath);
    }
  }
  for (const m of [...modules.values()].sort((a, b) => a.Path.localeCompare(b.Path, 'en'))) {
    const c = component(m.Path, m.Version, 'Go（Windows / Linux / macOS の実行時依存）', `https://pkg.go.dev/${m.Path}@${m.Version}`);
    addFiles(c, m.Dir);
    if (m.Path === 'github.com/benburkert/openpgp') {
      // Only aes/keywrap is linked. Its own full BSD notice is in the source header.
      const text = read(path.join(m.Dir, 'aes/keywrap/keywrap.go')).split('\n\npackage keywrap')[0]
        .split('\n').map(line => line.replace(/^\/\/ ?/, '')).join('\n');
      add(c, text, 'aes/keywrap/keywrap.go: source license header');
    }
    if (!c.notices.length) throw Error(`No license found for Go module ${m.Path}`);
    c.packages = m.packages.sort();
  }
  const goroot = go(['env', 'GOROOT']).trim(), version = go(['env', 'GOVERSION']).trim();
  const c = component('Go runtime / standard library', version, 'Go実行環境', 'https://go.dev/LICENSE');
  for (const filename of ['LICENSE', 'PATENTS']) if (fs.existsSync(path.join(goroot, filename))) add(c, read(path.join(goroot, filename)), filename);
  for (const file of noticeFiles(path.join(goroot, 'src/vendor')).sort()) add(c, read(file), 'src/vendor/' + path.relative(path.join(goroot, 'src/vendor'), file).replace(/\\/g, '/'));
  return modules.size;
}
function npmNotice(area, installName, kind) {
  const dir = path.join(root, area, 'node_modules', installName);
  const pkg = json(path.join(dir, 'package.json'));
  const c = component(pkg.name, pkg.version, kind, typeof pkg.repository === 'string' ? pkg.repository : pkg.repository?.url || pkg.homepage || `https://www.npmjs.com/package/${pkg.name}`);
  c.license = pkg.license;
  addFiles(c, dir);
  if (!c.notices.length) throw Error(`No license found for npm package ${pkg.name}`);
  return { c, pkg, dir };
}
function appNotices(statsPath) {
  const stats = json(statsPath), names = new Set();
  function walk(m, active) {
    active = active || !!m.chunks?.length;
    if (active && m.identifier) {
      // Loader code before ! executes during the build; only the final resource is emitted.
      const resource = m.identifier.split('!').at(-1);
      const matches = [...resource.matchAll(/node_modules[\\/]((?:@[^\\/!?]+[\\/])?[^\\/!?]+)/g)];
      if (matches.length) names.add(matches.at(-1)[1].replace(/\\/g, '/'));
    }
    for (const child of m.modules || []) walk(child, active);
  }
  for (const m of stats.modules || []) walk(m, false);
  if (!names.has('vue') || !names.has('vuetify') || !names.has('axios')) throw Error('Incomplete app production stats');
  for (const name of [...names].sort()) npmNotice('app', name, 'Webアプリに出力されるコード');
  // Webpack's bootstrap is emitted even though it is not a normal npm resource module.
  npmNotice('app', 'webpack', 'Webアプリのバンドル実行時コード');
  return [...names].sort();
}
function rockbarNotices() {
  const seen = new Set();
  function visit(name) {
    if (seen.has(name)) return; seen.add(name);
    const { pkg } = npmNotice('rockbar', name, 'RockbarのNode.js実行時依存');
    for (const child of Object.keys(pkg.dependencies || {})) visit(child);
    for (const child of Object.keys(pkg.optionalDependencies || {})) {
      if (fs.existsSync(path.join(root, 'rockbar/node_modules', child, 'package.json'))) visit(child);
    }
  }
  for (const name of Object.keys(json('rockbar/package.json').dependencies || {})) visit(name);
  const electron = json('rockbar/node_modules/electron/package.json');
  const c = component('Electron', electron.version, 'RockbarのWindows実行環境', 'https://github.com/electron/electron');
  add(c, read('rockbar/node_modules/electron/dist/LICENSE'), 'Electron distribution LICENSE');
  // Keep Electron's own complete, readable credits file intact instead of expanding
  // its hundreds of components in the main document and the login page.
  const credits = read('rockbar/node_modules/electron/dist/LICENSES.chromium.html');
  const count = (credits.match(/class="product"/g) || []).length;
  if (count < 100) throw Error('Incomplete Chromium credits');
  c.bundledNotices = 'licenses/electron/LICENSES.chromium.html';
  c.bundledNoticeHash = hash(credits);
  npmNotice('rockbar', 'app-builder-lib', 'Rockbarインストーラーに使われるNSISテンプレート');
  return { version: electron.version, chromiumComponents: count };
}
function otherNotices() {
  for (const item of json(`${fontDirectory}/sources.json`)) {
    const c = component(item.name, item.version, 'CDNから読み込むフォント・アイコン', item.source);
    for (const file of fontLicenseFiles(item)) add(c, read(`${fontDirectory}/${file}`), file);
  }
  const lucide = component('Lucide / Feather-derived icons', '0.468.0', 'Rockbarの再生操作アイコン（派生アイコンを含む）', 'https://github.com/lucide-icons/lucide/blob/0.468.0/LICENSE');
  add(lucide, read('rockbar/src/assets/playback-icons/LICENSE'), 'assets/playback-icons/LICENSE');
  const nsis = component('NSIS installer / compression modules', '3.0.4.1', 'RockbarのWindowsインストーラー', 'https://github.com/electron-userland/electron-builder-binaries/releases/tag/nsis-3.0.4.1');
  add(nsis, read('licenses/NSIS-COPYING.txt'), 'NSIS distribution COPYING');
  for (const item of json(`${pluginDirectory}/sources.json`)) {
    const c = component(item.name, item.version, 'RockbarのWindowsインストーラー用プラグイン', item.source);
    for (const file of item.files) add(c, read(`${pluginDirectory}/${file}`), file);
  }
}
function render() {
  const cleanCell = value => String(value || '').replace(/\|/g, '\\|').replace(/[\r\n]/g, ' ');
  const intro = `# サードパーティーライセンス\n\nTvRockGOのGo実行時依存、Webアプリの本番バンドル、Rockbarの実行時依存・Electron付属コンポーネント、配信フォント・アイコン、インストーラーに関する著作権表示とライセンス本文です。\n\nGoはWindows・Linux・macOS（CGO無効）の依存を合わせています。npmは本番バンドルに出力されるコードとRockbarの実行時依存を対象とし、出力されないビルド・テスト専用ツールは含めていません。CDNのフォントは取得時点の配布元表記です。各コンポーネントのNOTICE、辞書の利用条件、同梱された第三者表記も保持しています。同一本文はまとめ、下の一覧と本文冒頭の「対象」で対応を示します。\n\nTvRockGO自身のライセンスは[LICENSE](LICENSE)を参照してください。別途導入するTVTest・BonDriver・TvRockDTV・TvRockOnTVTest等は、この配布物に含めず、それぞれの配布元の表記を参照してください。\n\nこのファイルとログイン画面のLicenseは、同じ生成元から更新します。更新手順はソースリポジトリのdocs/releases.mdを参照してください。\n\n## コンポーネント一覧\n\n| コンポーネント | バージョン | 配布箇所 | 本文 |\n| --- | --- | --- | --- |\n`;
  const table = components.map(c => {
    const source = (c.source || '').replace(/^git\+/, '').replace(/^git:/, 'https:');
    const name = /^https?:/.test(source) ? `[${cleanCell(c.name)}](${source})` : cleanCell(c.name);
    return `| ${name} | ${cleanCell(c.version)} | ${cleanCell(c.kind)} | ${c.notices.map(id => `[${id}](#${id.toLowerCase()})`).join(', ')}${c.bundledNotices ? ' ＋付属ファイル（下記）' : ''} |`;
  }).join('\n');
  const electron = '\n\n## Electronの内包部品\n\nElectronはChromium・Node.js等を含む1項目として扱います。これらの内包部品の著作権表示・ライセンス全文は、Electronに付属する `LICENSES.chromium.html` を改変せず同梱します。\n\n- TvRockGO配布ZIP: `licenses/electron/LICENSES.chromium.html`\n- Windows版Rockbar単体: インストール先の `LICENSES.chromium.html`\n\nElectron本体のMITライセンスはこの文書にも収録しています。内包部品の全文は上記ファイルをブラウザで開いて確認してください。\n';
  return intro + table + electron + '\n' + registry.map(n => {
    const ticks = '`'.repeat(Math.max(3, ...[...n.text.matchAll(/`+/g)].map(m => m[0].length + 1)));
    return `\n## ${n.id}\n\n対象:\n\n${n.origins.map(s => '- ' + s).join('\n')}\n\n${ticks}text\n${n.text}\n${ticks}\n`;
  }).join('');
}
function embeddedText(html) {
  const match = html.match(/<textarea hidden id="licenseSource">\n([\s\S]*?)\n    <\/textarea>/);
  if (!match) throw Error('Generated license payload missing from login.html');
  return require(require.resolve('he', { paths: [path.join(root, 'app')] })).decode(match[1]);
}
function check() {
  const manifest = json('licenses/manifest.json');
  const expected = manifest.inputHashes;
  if (!expected) throw Error('License manifest lacks input hashes: regenerate third-party licenses.');
  const missing = Object.keys(expected).filter(file => !fs.existsSync(path.join(root, file)));
  if (missing.length) throw Error('Missing third-party license inputs:\n' + missing.map(file => '  ' + file).join('\n'));
  const actual = dependencyInputHashes();
  const changed = Object.keys({ ...expected, ...actual }).sort().filter(file => expected[file] !== actual[file]);
  if (changed.length) {
    throw Error('Dependencies or upstream license inputs changed: regenerate third-party licenses.\n' +
      changed.map(file => '  ' + file).join('\n'));
  }
  const md = read('THIRD_PARTY_LICENSES.md');
  if (hash(md) !== manifest.documentHash) throw Error('THIRD_PARTY_LICENSES.md differs from the generated manifest');
  const electron = manifest.components.find(c => c.name === 'Electron');
  if (fs.existsSync(path.join(root, 'rockbar/node_modules/electron/dist/LICENSES.chromium.html')) &&
      electron.bundledNoticeHash !== hash(read('rockbar/node_modules/electron/dist/LICENSES.chromium.html'))) {
    throw Error('Electron credits changed: regenerate third-party licenses.');
  }
  const expectedText = 'TvRockGO\n\n' + read('LICENSE').trim() + '\n\n' + md;
  if (embeddedText(read('template/login.html')) !== expectedText) throw Error('Login license text differs from THIRD_PARTY_LICENSES.md / LICENSE');
  console.log(`PASS: ${manifest.components.length} components, ${manifest.noticeCount} complete notice texts; login and distribution document match`);
}
function main() {
  if (process.argv.includes('--check')) return check();
  const at = process.argv.indexOf('--app-report');
  if (at < 0 || !process.argv[at + 1]) throw Error('Pass --app-report <production webpack report.json>');
  const goModules = goNotices(); console.log(`Go: ${goModules} modules`);
  const appPackages = appNotices(process.argv[at + 1]); console.log(`App: ${appPackages.join(', ')}`);
  const electron = rockbarNotices(); console.log(`Electron: ${electron.chromiumComponents} credited components`);
  otherNotices();
  const document = render(); fs.writeFileSync(path.join(root, 'THIRD_PARTY_LICENSES.md'), document);
  const plain = 'TvRockGO\n\n' + read('LICENSE').trim() + '\n\n' + document;
  // A hidden textarea keeps all notices readable in the source and decodes HTML
  // entities natively. Escaping braces also protects Go html/template delimiters.
  const payload = plain.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/{/g, '&#123;');
  const html = read('template/login.html').replace(/(?:<script type="text\/plain" id="licenseSource"[^>]*>[\s\S]*?<\/script>|<textarea hidden id="licenseSource">[\s\S]*?<\/textarea>)/,
    () => `<textarea hidden id="licenseSource">\n${payload}\n    </textarea>`);
  if (embeddedText(html) !== plain) throw Error('License payload verification failed');
  fs.writeFileSync(path.join(root, 'template/login.html'), html);
  fs.writeFileSync(path.join(root, 'licenses/manifest.json'), JSON.stringify({
    inputHashes: dependencyInputHashes(), documentHash: hash(document), noticeCount: registry.length,
    goModules, appPackages, electron, components,
  }, null, 2) + '\n');
  console.log(`Wrote ${components.length} components / ${registry.length} texts (${Buffer.byteLength(document)} bytes)`);
  check();
}
if (require.main === module) { try { main(); } catch (error) { console.error(error.message); process.exitCode = 1; } }
module.exports = { parseJSONStream, embeddedText };
