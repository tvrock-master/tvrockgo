const fs = require('node:fs');
const path = require('node:path');
const { clientVersion } = require('./client-version.cjs');

function buildWeb(output = path.resolve(__dirname, '../../static/rockbar')) {
  const source = path.resolve(__dirname, '../src');
  fs.mkdirSync(output, { recursive: true });
  // Copy only browser assets; never clean the parent's static directory.
  for (const file of ['style.css', 'theme.js', 'programs.js', 'clock.js', 'playback-status.js', 'playback-files.js', 'schedules.js', 'captions.js', 'caption-scroll.js', 'program-scroll.js', 'long-press.js', 'protocol.js', 'client.js', 'browser.js', 'renderer.js']) {
    fs.copyFileSync(path.join(source, file), path.join(output, file));
  }
  fs.cpSync(path.join(source, 'assets'), path.join(output, 'assets'), { recursive: true });
  fs.copyFileSync(path.join(source, 'playback-boundaries.js'), path.join(output, 'playback-boundaries.js'));
  fs.writeFileSync(path.join(output, 'client-identity.js'),
    fs.readFileSync(path.join(source, 'client-identity.js'), 'utf8')
      .replace("'TvRockRockbar/' + 'dev'", JSON.stringify('TvRockRockbar/' + clientVersion())));
  fs.cpSync(path.resolve(__dirname, '../web'), output, { recursive: true });
  const html = fs.readFileSync(path.join(source, 'index.html'), 'utf8')
    .replace("connect-src 'none'", "connect-src 'self' ws: wss:")
    .replace('</head>', '  <link rel="manifest" href="manifest.json" crossorigin="use-credentials">\n  <link rel="icon" type="image/png" sizes="192x192" href="icons/rockbar192x192.png">\n  <link rel="apple-touch-icon" href="icons/rockbar180x180.png">\n  <meta name="theme-color" content="#38434e">\n</head>')
    .replace('<!-- runtime-scripts -->', '<script src="protocol.js"></script>\n  <script src="client.js"></script>\n  <script src="client-identity.js"></script>\n  <script src="browser.js"></script>');
  fs.writeFileSync(path.join(output, 'index.html'), html);
  return output;
}

if (require.main === module) console.log('Browser build: ' + buildWeb());
module.exports = { buildWeb };
