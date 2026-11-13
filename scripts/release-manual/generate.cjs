const fs = require('node:fs');
const path = require('node:path');
const { pathToFileURL } = require('node:url');
const { spawnSync } = require('node:child_process');

const pdfName = 'TvRockGO-Manual.pdf';
const [artifactRoot, ...args] = process.argv.slice(2);

function generateInElectron(root, chapters) {
  const workDir = fs.mkdtempSync(path.join(root, '.release-manual-'));
  try {
    const electron = require('../../rockbar/node_modules/electron');
    const result = spawnSync(electron, [__filename, root, workDir, ...chapters], {
      stdio: 'inherit',
      windowsHide: true,
      timeout: 120_000,
    });
    if (result.error) throw result.error;
    if (result.status !== 0) throw Error('Electron PDF generation failed: ' + (result.signal || result.status));
    fs.renameSync(path.join(workDir, pdfName), path.join(root, pdfName));
  } finally {
    fs.rmSync(workDir, { recursive: true, force: true, maxRetries: 5 });
  }
}

function preparePrintDocument(artifactURL, version) {
  const sourceBase = 'https://manual.invalid/';
  const repository = 'https://github.com/tvrock-master/tvrockgo';
  const gitRef = version && version !== 'dev' ? encodeURIComponent(version) : 'HEAD';
  const slug = text => text.trim().toLowerCase().replace(/[^\p{L}\p{M}\p{N}_\s-]/gu, '').replace(/\s/g, '-');
  const contents = document.querySelector('#contents');
  const chapters = new Map();

  for (const [index, template] of Array.from(document.querySelectorAll('template[data-source]')).entries()) {
    const source = template.dataset.source;
    const section = document.createElement('section');
    section.className = 'chapter';
    section.id = 'chapter-' + (index + 1);
    section.append(template.content);
    const title = section.querySelector('h1');
    if (!title) throw Error('Missing chapter title: ' + source);

    const item = document.createElement('li');
    const link = document.createElement('a');
    link.href = '#' + section.id;
    link.textContent = index === 0 ? '導入と基本操作' : title.textContent;
    item.append(link);
    contents.append(item);

    const headings = new Map();
    for (const [number, heading] of Array.from(section.querySelectorAll('h1, h2, h3, h4, h5, h6')).entries()) {
      const base = slug(heading.textContent);
      let key = base;
      for (let suffix = 1; headings.has(key); suffix++) key = base + '-' + suffix;
      heading.id = section.id + '-' + (number + 1);
      headings.set(key, heading.id);
    }
    chapters.set(source, { section, headings });
    template.replaceWith(section);
  }

  for (const [source, { section }] of chapters) {
    const sourceURL = new URL(source, sourceBase);
    for (const image of section.querySelectorAll('img')) {
      const reference = new URL(image.getAttribute('src'), sourceURL);
      if (reference.origin !== sourceURL.origin || !reference.pathname.startsWith('/docs/images/')) {
        throw Error('Image must be in docs/images/: ' + image.getAttribute('src'));
      }
      image.src = new URL(reference.pathname.slice(1), artifactURL).href;
    }
    for (const link of section.querySelectorAll('a[href]')) {
      const reference = new URL(link.getAttribute('href'), sourceURL);
      if (reference.origin !== sourceURL.origin) {
        if (!['http:', 'https:', 'mailto:'].includes(reference.protocol)) {
          throw Error('Unsupported link: ' + link.getAttribute('href'));
        }
        continue;
      }

      const target = decodeURIComponent(reference.pathname.slice(1));
      const destination = chapters.get(target);
      if (destination) {
        const fragment = decodeURIComponent(reference.hash.slice(1));
        let id = destination.section.id;
        if (fragment) {
          id = destination.headings.get(fragment);
          if (!id) {
            // Some source links use the section title without its leading number or punctuation.
            const matches = [...destination.headings].filter(([key]) => key.replace(/^\d+(?:-\d+)*-/, '') === slug(fragment));
            if (matches.length !== 1) throw Error(source + ': unresolved link ' + link.getAttribute('href'));
            id = matches[0][1];
          }
        }
        link.href = '#' + id;
      } else if (['LICENSE', 'THIRD_PARTY_LICENSES.md', 'docs/custom-command-examples/'].includes(target)) {
        link.replaceWith(link.textContent + '（同梱の ' + target + ' を参照）');
      } else if (['AGENTS.md', 'docs/releases.md', 'rockbar/README.md'].includes(target)) {
        link.href = repository + '/blob/' + gitRef + '/' + target + reference.hash;
      } else {
        throw Error(source + ': unresolved link ' + link.getAttribute('href'));
      }
    }
  }

  return Promise.all([
    document.fonts.load('400 12px "Noto Sans JP Variable"', '利用マニュアル'),
    ...Array.from(document.images, async image => {
      try {
        await image.decode();
      } catch {
        throw Error('Cannot load manual image: ' + image.getAttribute('src'));
      }
    }),
  ]).then(async ([fonts]) => {
    await document.fonts.ready;
    if (!fonts.length || Array.from(document.fonts).some(font => font.status === 'error')) {
      throw Error('Cannot load Noto Sans JP for the release manual');
    }
  });
}

async function renderPDF(root, workDir, chapters) {
  const { app, BrowserWindow } = require('electron');
  const { marked } = require('marked');
  const version = process.env.APP_VERSION;
  const css = fs.readFileSync(path.join(__dirname, 'print.css'), 'utf8');
  const fontCSS = pathToFileURL(require.resolve('@fontsource-variable/noto-sans-jp/index.css')).href;
  const escape = text => text.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;');
  const content = ['README.md', ...chapters].map(source => {
    const markdown = fs.readFileSync(path.join(root, source), 'utf8').replace(/^\uFEFF/, '');
    return '<template data-source="' + escape(source) + '">' + marked.parse(markdown, { gfm: true }) + '</template>';
  }).join('\n');
  const edition = version && version !== 'dev' ? '<p>' + escape(version) + '</p>' : '';
  const documentPath = path.join(workDir, 'manual.html');
  fs.writeFileSync(documentPath, '<!doctype html><html lang="ja"><head><meta charset="utf-8">' +
    '<meta http-equiv="Content-Security-Policy" content="default-src \'none\'; style-src \'unsafe-inline\' file:; img-src file:; font-src file:">' +
    '<title>TvRockGO 利用マニュアル</title><link rel="stylesheet" href="' + escape(fontCSS) + '">' +
    '<style>' + css + '</style></head><body><main><section class="cover">' +
    '<p class="product">TvRockGO</p><h1>利用マニュアル</h1>' + edition +
    '<h2>目次</h2><ol id="contents"></ol></section>' + content + '</main></body></html>');

  app.setPath('userData', workDir);
  app.disableHardwareAcceleration();
  await app.whenReady();
  const window = new BrowserWindow({ show: false, webPreferences: { sandbox: true, backgroundThrottling: false } });
  await window.loadFile(documentPath);
  const artifactURL = pathToFileURL(root + path.sep).href;
  await window.webContents.executeJavaScript(
    '(' + preparePrintDocument.toString() + ')(' + JSON.stringify(artifactURL) + ', ' + JSON.stringify(version || '') + ')'
  );
  const pdf = await window.webContents.printToPDF({
    pageSize: 'A4',
    preferCSSPageSize: true,
    printBackground: true,
    generateTaggedPDF: true,
    generateDocumentOutline: true,
    displayHeaderFooter: true,
    headerTemplate: '<span></span>',
    footerTemplate: '<div style="width:100%;text-align:center;font:8px Arial;color:#4a5565">' +
      '<span class="pageNumber"></span> / <span class="totalPages"></span></div>',
  });
  if (!pdf.subarray(0, 5).equals(Buffer.from('%PDF-'))) throw Error('Electron did not produce a PDF');
  fs.writeFileSync(path.join(workDir, pdfName), pdf);
}

if (process.versions.electron) {
  const { app } = require('electron');
  const [workDir, ...chapters] = args;
  renderPDF(artifactRoot, workDir, chapters).then(() => app.exit(0), error => {
    console.error(error);
    app.exit(1);
  });
} else if (!artifactRoot || !args.length) {
  console.error('Usage: node generate.cjs <release-directory> <docs/chapter.md> [...]');
  process.exitCode = 1;
} else {
  generateInElectron(path.resolve(artifactRoot), args);
  console.log('Release manual: ' + path.join(artifactRoot, pdfName));
}
