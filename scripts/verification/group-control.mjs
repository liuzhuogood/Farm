import { spawn } from 'node:child_process'
import { once } from 'node:events'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import assert from 'node:assert/strict'

const [binary, source, target, reportPath] = process.argv.slice(2)
assert.ok(binary && source && target, 'Usage: node scripts/verification/group-control.mjs <native-verify GUI> <source serial> <target serial> [report path]')
assert.notEqual(source, target, 'Source and target must be distinct online devices')
const directory = await mkdtemp(path.join(os.tmpdir(), 'mywebscrcpy-group-'))
const report = reportPath ? path.resolve(reportPath) : path.join(directory, 'report.json')
await rm(report, { force: true })
const child = spawn(path.resolve(binary), [], {
  env: {
    ...process.env,
    MYWEBSCRCPY_NATIVE_REPORT: report,
    MYWEBSCRCPY_VERIFY_GROUP_CONTROL: '1',
    MYWEBSCRCPY_VERIFY_GROUP_SOURCE: source,
    MYWEBSCRCPY_VERIFY_GROUP_TARGET: target,
    MYWEBSCRCPY_DESKTOP_PORT: '21989',
  },
  stdio: ['ignore', 'ignore', 'pipe'],
})
let errors = ''
child.stderr.on('data', data => { errors = (errors + data).slice(-8192) })
const timer = setTimeout(() => child.kill(), 80000)
try {
  const [code] = await once(child, 'exit')
  const result = JSON.parse(await readFile(report, 'utf8')).group_control
  assert.equal(code, 0, `${JSON.stringify(result)}\n${errors}`)
  for (const key of ['ok', 'cardViewSelectedThroughUi', 'targetSelectedBeforeOpen', 'enabledBeforeOpen', 'targetConnectionReady', 'nativeWindowOpenedAfterEnable', 'sourceReceivedActiveState', 'sourceCanvasDispatched', 'homepageReceivedInput', 'targetDispatchedOnce'])
    assert.equal(result[key], true, `${key}: ${JSON.stringify(result)}\n${errors}`)
  for (const key of ['sourcePackets', 'inputPackets', 'targetPackets'])
    assert.deepEqual(result[key], [[4, 0], [4, 1]], `${key}: input must be delivered exactly once`)
  console.log('PASS: enable group control before opening a native player; production state, canvas input and single target dispatch')
  console.log(JSON.stringify(result))
}
finally {
  clearTimeout(timer)
  if (child.exitCode === null) child.kill()
  await rm(directory, { recursive: true, force: true })
}
