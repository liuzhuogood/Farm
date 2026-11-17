import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { spawnSync } from 'node:child_process'
import path from 'node:path'

const bundle = path.resolve(process.argv[2] ?? 'desktop/target/release/bundle')
function files(directory) {
  return readdirSync(directory, { withFileTypes: true }).flatMap(item =>
    item.isDirectory() ? files(path.join(directory, item.name)) : [path.join(directory, item.name)])
}

const archives = files(bundle).filter(file => file.endsWith('.app.tar.gz'))
assert.equal(archives.length, 1, 'macOS 更新构建必须恰好生成一个 .app.tar.gz')
const archive = archives[0]
const signature = `${archive}.sig`
assert(files(bundle).includes(signature), 'macOS 更新包缺少签名文件')
assert(readFileSync(signature, 'utf8').trim(), 'macOS 更新包签名为空')

const listing = spawnSync('tar', ['-tzf', archive], { encoding: 'utf8' })
assert.equal(listing.status, 0, `无法读取 macOS 更新包: ${listing.stderr || listing.stdout}`)
const entries = listing.stdout.trim().split('\n').filter(Boolean)
assert(entries.some(entry => /\.app\/Contents\/Info\.plist$/.test(entry)), 'macOS 更新包缺少 App Info.plist')
assert(entries.some(entry => /\.app\/Contents\/MacOS\/farm-gui$/.test(entry)), 'macOS 更新包缺少 GUI 可执行文件')
assert(entries.some(entry => /\.app\/Contents\/MacOS\/farm$/.test(entry)), 'macOS 更新包缺少服务可执行文件')

console.log(`PASS: macOS updater archive and signature (${path.basename(archive)})`)
