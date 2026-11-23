import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, readdirSync, rmSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const directory = path.join(root, '.cache/services/desktop-dev')
const recordFile = path.join(directory, 'process.json')
const marker = path.join(directory, 'window-ready')
const lock = path.join(directory, 'launch.lock')
const dataDirectory = path.join(os.homedir(), 'Library/Application Support/com.farm.desktop')
const endpointFile = path.join(dataDirectory, 'desktop-endpoint.json')
const appBundle = path.join(root, 'desktop/target/release/bundle/macos/Farm.app')
const guiExecutable = path.join(appBundle, 'Contents/MacOS/farm-gui')
const serviceExecutable = path.join(appBundle, 'Contents/MacOS/farm')
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms))

function identity(pid) {
  if (!Number.isInteger(pid) || pid <= 1) return ''
  try {
    return execFileSync('ps', ['-p', String(pid), '-o', 'lstart=', '-o', 'args='], {
      encoding: 'utf8', env: { ...process.env, LC_ALL: 'C' }, stdio: ['ignore', 'pipe', 'ignore'],
    }).trim()
  } catch { return '' }
}

function executable(pid) {
  if (!Number.isInteger(pid) || pid <= 1) return ''
  try {
    return execFileSync('ps', ['-ww', '-p', String(pid), '-o', 'comm='], {
      encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'],
    }).trim()
  } catch { return '' }
}

function parentPID(pid) {
  try { return Number(execFileSync('ps', ['-p', String(pid), '-o', 'ppid='], { encoding: 'utf8' }).trim()) }
  catch { return 0 }
}

function readJSON(file) {
  if (!file || !existsSync(file)) return undefined
  try { return JSON.parse(readFileSync(file, 'utf8')) }
  catch { return undefined }
}

function validGUI(pid) {
  return executable(pid) === guiExecutable
}

function validService(pid) {
  return executable(pid) === serviceExecutable
}

function addCandidate(candidates, pid, source, ready) {
  if (!validGUI(pid)) return
  const servicePID = Number(ready?.pid ?? 0)
  const hasService = Number.isInteger(servicePID) && servicePID > 1 && validService(servicePID) && parentPID(servicePID) === pid
  const previous = candidates.get(pid)
  candidates.set(pid, {
    pid,
    identity: identity(pid),
    servicePID: hasService ? servicePID : previous?.servicePID,
    ready: hasService ? ready : previous?.ready,
    readyFile: hasService ? ready?.file ?? previous?.readyFile : previous?.readyFile,
    sources: [...(previous?.sources ?? []), source],
  })
}

function collectCandidates() {
  const candidates = new Map()
  const record = readJSON(recordFile)
  if (record && identity(record.pid) === record.identity) addCandidate(candidates, record.pid, 'process.json')

  const endpoint = readJSON(endpointFile)
  if (endpoint && validService(endpoint.pid)) {
    const guiPID = parentPID(endpoint.pid)
    addCandidate(candidates, guiPID, 'desktop-endpoint.json', endpoint)
  }

  if (existsSync(dataDirectory)) {
    for (const name of readdirSync(dataDirectory)) {
      const match = /^desktop-ready-(\d+)\.json$/.exec(name)
      if (!match) continue
      const file = path.join(dataDirectory, name)
      const ready = readJSON(file)
      if (!ready) continue
      addCandidate(candidates, Number(match[1]), 'desktop-ready', { ...ready, file })
    }
  }
  return [...candidates.values()]
}

async function waitUntil(predicate, timeout, message) {
  const deadline = Date.now() + timeout
  while (Date.now() < deadline) {
    if (await predicate()) return
    await sleep(100)
  }
  if (!(await predicate())) throw new Error(message)
}

function cleanStaleState(candidate) {
  rmSync(recordFile, { force: true })
  rmSync(marker, { force: true })
  if (candidate?.readyFile || candidate?.ready?.file) {
    rmSync(candidate.readyFile ?? candidate.ready.file, { force: true })
  }
  const endpoint = readJSON(endpointFile)
  if (candidate?.servicePID && endpoint?.pid === candidate.servicePID && !validService(candidate.servicePID)) {
    rmSync(endpointFile, { force: true })
  }
}

async function main() {
  if (process.platform !== 'darwin' || process.arch !== 'arm64') throw new Error('此快捷脚本用于 macOS Apple Silicon 开发环境')
  if (process.argv.length !== 2) throw new Error('用法: node scripts/stop-desktop.mjs')
  const owner = readJSON(path.join(lock, 'owner.json'))
  if (owner && identity(owner.pid) === owner.identity) throw new Error('桌面启动命令正在执行，请等待完成后再停止')
  rmSync(lock, { recursive: true, force: true })

  const candidates = collectCandidates()
  if (candidates.length === 0) {
    cleanStaleState()
    console.log('[未运行] 当前源码桌面开发版')
    return
  }
  if (candidates.length > 1) {
    throw new Error(`检测到多个当前源码桌面实例（PID ${candidates.map(item => item.pid).join('、')}），请确认后手动停止`)
  }

  const candidate = candidates[0]
  console.log(`[停止] 当前源码桌面开发版（PID ${candidate.pid}）`)
  try { process.kill(candidate.pid, 'SIGTERM') }
  catch (error) { if (error.code !== 'ESRCH') throw error }
  await waitUntil(() => !validGUI(candidate.pid), 10000, '桌面进程未按时退出，请从 Farm 菜单退出后重试')

  if (candidate.servicePID && validService(candidate.servicePID)) {
    const ready = candidate.ready?.url && candidate.ready?.token ? candidate.ready : readJSON(candidate.readyFile)
    if (ready?.url && ready?.token) {
      try {
        await fetch(ready.url + '/api/desktop/shutdown', {
          method: 'POST',
          headers: { Authorization: `Bearer ${ready.token}` },
          signal: AbortSignal.timeout(2000),
        })
      } catch {}
    }
    await waitUntil(() => !validService(candidate.servicePID), 10000, '配套服务未按时退出，请查看 desktop.log')
  }

  cleanStaleState(candidate)
  console.log('[已停止] 当前源码桌面开发版')
}

main().catch(error => { console.error(`[桌面停止失败] ${error.message}`); process.exitCode = 1 })
