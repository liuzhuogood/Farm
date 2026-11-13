import assert from 'node:assert/strict'
import { afterEach, mock, test } from 'node:test'
import { defaultAutomationTask, listAutomationTasks, stopAutomationTask, taskDirectoryURL } from '../src/api/automationTasks.ts'

const task = (overrides = {}) => ({
  id: 'task-1', serial: 'phone', title: '核对打卡', source: 'automation', state: 'running', summary: '',
  started_at: '2026-11-13T09:00:00Z', updated_at: '2026-11-13T09:00:00Z', active: false,
  steps: [], artifacts: [], ...overrides,
})
afterEach(() => mock.restoreAll())

test('default selection prefers active runtime, then open runtime, and never stale MCP running', () => {
  const mcp = task({ id: 'mcp-old', source: 'mcp', started_at: '2026-01-01T00:00:00Z' })
  const done = task({ id: 'recent-done', state: 'done', started_at: '2026-11-13T12:00:00Z' })
  assert.equal(defaultAutomationTask([mcp, done]).id, 'recent-done')
  const waiting = task({ id: 'waiting', started_at: '2026-11-13T10:00:00Z' })
  const active = task({ id: 'active', active: true })
  assert.equal(defaultAutomationTask([mcp, waiting, done, active]).id, 'active')
  assert.equal(defaultAutomationTask([mcp, waiting, done]).id, 'waiting')
  assert.equal(defaultAutomationTask([]), undefined)
})

test('task client uses exact schema and preserves cancellation and removed artifact references', async () => {
  const controller = new AbortController()
  const value = task({ artifacts: [{ id: 'deleted', serial: 'phone', relative_path: 'tasks/task-1/image.png', kind: 'image', download_url: '/download', removed: true }] })
  mock.method(globalThis, 'fetch', async (path, init) => {
    assert.equal(new URL(path, 'http://farm.test').searchParams.get('serial'), 'phone')
    assert.equal(init.signal, controller.signal)
    return Response.json({ tasks: [value] })
  })
  assert.deepEqual(await listAutomationTasks('phone', controller.signal), [value])
})

test('task client rejects another device and missing or incompatible fields instead of guessing aliases', async () => {
  for (const payload of [{ tasks: [task({ serial: 'other-phone' })] }, { tasks: null }, { tasks: [{ id: 'old', title: 'legacy', device_id: 'phone' }] }]) {
    const fetch = mock.method(globalThis, 'fetch', async () => Response.json(payload))
    await assert.rejects(listAutomationTasks('phone', new AbortController().signal), /任务数据格式不正确/)
    fetch.mock.restore()
  }
})

test('stop sends the supported stopped outcome and checks returned task identity', async () => {
  const controller = new AbortController()
  const finished = task({ state: 'stopped', summary: '用户停止任务', ended_at: '2026-11-13T09:01:00Z' })
  mock.method(globalThis, 'fetch', async (path, init) => {
    assert.equal(path, '/api/automation/tasks/task-1/finish?serial=phone')
    assert.equal(init.method, 'POST')
    assert.equal(init.signal, controller.signal)
    assert.deepEqual(JSON.parse(init.body), { outcome: 'stopped', summary: '用户停止任务' })
    return Response.json({ task: finished })
  })
  assert.equal((await stopAutomationTask('phone', 'task-1', controller.signal)).state, 'stopped')
})

test('request errors remain errors and directory URLs bind serial and relative directory', async () => {
  mock.method(globalThis, 'fetch', async () => Response.json({ message: '任务服务不可用' }, { status: 503 }))
  await assert.rejects(listAutomationTasks('phone', new AbortController().signal), /任务服务不可用/)
  const url = new URL(taskDirectoryURL('10.0.0.104:5555', '任务 空格/导出'), 'http://farm.test')
  assert.equal(url.searchParams.get('serial'), '10.0.0.104:5555')
  assert.equal(url.searchParams.get('directory'), '任务 空格/导出')
})

test('task client rejects artifacts belonging to another device at either nesting level', async () => {
  const media = { id: 'foreign', serial: 'other-phone', relative_path: 'image.png', kind: 'image', download_url: '/download' }
  const action = { index: 1, title: '截图', command: 'screenshot work', status: 'done', elapsed_ms: 5, artifacts: [media] }
  for (const value of [task({ artifacts: [media] }), task({ steps: [action] })]) {
    const fetch = mock.method(globalThis, 'fetch', async () => Response.json({ tasks: [value] }))
    await assert.rejects(listAutomationTasks('phone', new AbortController().signal), /任务数据格式不正确/)
    fetch.mock.restore()
  }
})
