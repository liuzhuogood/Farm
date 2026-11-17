import { spawn } from 'node:child_process'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import os from 'node:os'
import path from 'node:path'
import { once } from 'node:events'
import assert from 'node:assert/strict'

const directory = await mkdtemp(path.join(os.tmpdir(), 'farm-update-menu-test-'))
const report = path.join(directory, 'report.json')
const child = spawn(path.resolve('desktop/target/release/farm-gui'), [], {
  env: {
    ...process.env,
    FARM_NATIVE_REPORT: report,
    FARM_VERIFY_UPDATE_MENU: '1',
    FARM_DESKTOP_PORT: process.env.FARM_DESKTOP_PORT || '1992',
  },
  stdio: ['ignore', 'ignore', 'pipe'],
})
let errors = ''
child.stderr.on('data', (data) => { errors = (errors + data.toString()).slice(-4096) })
const timer = setTimeout(() => child.kill(), 55000)
try {
  const [code] = await once(child, 'exit')
  assert.equal(code, 0, errors)
  const result = JSON.parse(await readFile(report, 'utf8'))
  assert.equal(result.check_update_dialog, true, JSON.stringify(result))
  console.log('PASS: native check-update menu opens the update dialog')
}
finally {
  clearTimeout(timer)
  if (child.exitCode === null) child.kill()
  await rm(directory, { recursive: true, force: true })
}
