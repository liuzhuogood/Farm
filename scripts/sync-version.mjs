import { readFileSync, writeFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const check = process.argv.includes('--check')
const versionFlag = process.argv.indexOf('--version')
const override = versionFlag >= 0 ? process.argv[versionFlag + 1] : undefined
if (versionFlag >= 0 && (!override || override.startsWith('--'))) throw new Error('--version 需要版本号')
let version = override ? override.replace(/^v/, '') : readFileSync(path.join(root, 'VERSION'), 'utf8').trim().replace(/^v/, '')
if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-([0-9A-Za-z.-]+))?$/.test(version)) throw new Error('VERSION 必须是正式 SemVer 版本')
if (override && !check) {
  const file = path.join(root, 'VERSION')
  const newline = readFileSync(file, 'utf8').includes('\r\n') ? '\r\n' : '\n'
  writeFileSync(file, `v${version}${newline}`)
}
function update(file, transform) {
 const full = path.join(root,file), old = readFileSync(full,'utf8')
 // Windows checkout uses CRLF; compare values while preserving its line endings.
 const newline = old.includes('\r\n') ? '\r\n' : '\n'
 const next = transform(old.replace(/\r\n/g, '\n')).replace(/\n/g, newline)
 if (old !== next) { if (check) throw new Error(`${file} 与 VERSION 不一致`); writeFileSync(full,next) }
}
for (const file of ['frontend/package.json','desktop/package.json','desktop/tauri.conf.json']) {
 update(file, text => text.replace(/("version"\s*:\s*")[^"]+("\s*[,}])/, `$1${version}$2`))
}
update('desktop/Cargo.toml', text => text.replace(/(\[package\][\s\S]*?\nversion = ")[^"]+("\n)/, `$1${version}$2`))
update('desktop/Cargo.lock', text => text.replace(/(name = "farm-gui"\nversion = ")[^"]+("\n)/, `$1${version}$2`))
update('packages/python-sdk/pyproject.toml', text => text.replace(/(\[project\][\s\S]*?\nversion = ")[^"]+("\n)/, `$1${version}$2`))
update('packages/python-sdk/src/farm/__init__.py', text => text.replace(/^(__version__ = ")[^"]+(")$/m, `$1${version}$2`))
update('packages/python-sdk/version.go', text => text.replace(/^(const Version = ")[^"]+(")$/m, `$1${version}$2`))
update('desktop/tauri.conf.json', text => { const config=JSON.parse(text); config.plugins.updater.pubkey=readFileSync(path.join(root,'desktop/updater.pub'),'utf8').trim(); return JSON.stringify(config,null,2)+'\n' })
console.log(`版本一致: ${version}`)
