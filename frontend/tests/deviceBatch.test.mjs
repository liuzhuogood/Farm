import assert from 'node:assert/strict'
import test from 'node:test'
import {
  executeBatchAction,
  dedupeSerials,
  filterSelectableDevices,
  useDeviceBatch,
} from '../src/composables/useDeviceBatch.ts'

test('dedupeSerials eliminates duplicate serials, respects max 1024 limit', () => {
  assert.deepEqual(dedupeSerials(['a', 'b', 'a', 'c', 'b']), ['a', 'b', 'c'])
  assert.deepEqual(dedupeSerials([]), [])
  assert.deepEqual(dedupeSerials(null), [])

  const many = Array.from({ length: 1100 }, (_, i) => `dev-${i}`)
  const deduped = dedupeSerials(many)
  assert.equal(deduped.length, 1024)
})

test('executeBatchAction bounded concurrency never exceeds 4 even if requested higher', async () => {
  const serials = ['d1', 'd2', 'd3', 'd4', 'd5', 'd6']
  let active = 0
  let maxActive = 0

  const runner = async () => {
    active++
    maxActive = Math.max(maxActive, active)
    await new Promise(r => setTimeout(r, 20))
    active--
    return { executed: true, status: 'executed' }
  }

  await executeBatchAction({
    batchId: 'concurrency-test',
    generation: 1,
    serials,
    action: 'home',
    concurrency: 8, // Request 8, must be capped at 4
    runner,
  })

  assert.ok(maxActive <= 4, `Observed concurrency ${maxActive} must not exceed 4`)
})

test('executeBatchAction releases all timers promptly upon success (no timer leak)', async () => {
  const start = Date.now()
  const serials = ['dev-quick-1', 'dev-quick-2']

  const quickRunner = async () => {
    return { executed: true, status: 'executed' }
  }

  const { results, summary } = await executeBatchAction({
    batchId: 'quick-batch',
    generation: 1,
    serials,
    action: 'back',
    runner: quickRunner,
  })

  const elapsed = Date.now() - start
  assert.ok(elapsed < 200, `Execution took ${elapsed}ms, should finish well within 200ms without timer leak`)
  assert.equal(summary.success, 2)
  assert.equal(results[0].status, 'success')
  assert.equal(results[1].status, 'success')
})

test('executeBatchAction marks dispatched items as unknown on timeout or abort, undispatched as cancelled', async () => {
  const serials = ['dispatched-timeout', 'undispatched-wait']

  const hangingRunner = async (serial, signal) => {
    if (serial === 'dispatched-timeout') {
      await new Promise((resolve, reject) => {
        signal.addEventListener('abort', () => {
          const err = new Error('设备执行超时 (50ms)')
          err.name = 'TimeoutError'
          reject(err)
        })
      })
    }
  }

  const { results, summary } = await executeBatchAction({
    batchId: 'timeout-unknown-batch',
    generation: 1,
    serials,
    action: 'home',
    concurrency: 1, // serial dispatch
    perDeviceTimeoutMs: 50,
    runner: hangingRunner,
  })

  assert.equal(results.length, 2)
  const d1 = results.find(r => r.serial === 'dispatched-timeout')
  assert.equal(d1?.status, 'unknown')
  assert.ok(d1?.error?.includes('超时'), 'Error should retain timeout reason')

  assert.equal(summary.unknown, 1)
})

test('executeBatchAction checks isTargetStillOperable before dispatching', async () => {
  const serials = ['live-dev', 'offline-later-dev']
  let operable = true

  const runner = async (serial) => {
    if (serial === 'live-dev') {
      // Simulate that the second device drops offline while first was running
      operable = false
      return { executed: true, status: 'executed' }
    }
    return { executed: true, status: 'executed' }
  }

  const { results } = await executeBatchAction({
    batchId: 'dyn-check-batch',
    generation: 1,
    serials,
    action: 'back',
    concurrency: 1,
    isTargetStillOperable: s => (s === 'live-dev' ? true : operable),
    runner,
  })

  const d1 = results.find(r => r.serial === 'live-dev')
  assert.equal(d1?.status, 'success')

  const d2 = results.find(r => r.serial === 'offline-later-dev')
  assert.equal(d2?.status, 'cancelled')
  assert.ok(d2?.error?.includes('不可操作'), 'Should mark as cancelled with reason')
})

test('useDeviceBatch destroy and cancel cleanly reset state', async () => {
  const batch = useDeviceBatch()
  const runner = () => new Promise(r => setTimeout(r, 200))

  batch.runBatch(['dev-1'], 'home', undefined, runner)
  assert.equal(batch.isRunning.value, true)

  batch.destroy()
  assert.equal(batch.isRunning.value, false)
  assert.equal(batch.results.value.length, 0)
  assert.equal(batch.currentBatchId.value, '')
})

// The power button must preserve system power semantics instead of invoking sleep.
test('batch power sends the named power event without the screen-lock command path', async () => {
  const previousFetch = globalThis.fetch
  const calls = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url, body: JSON.parse(init.body) })
    return new Response(JSON.stringify({ request_id: 'power-1', status: 'executed', executed: true, mode: 'sdk', elapsed_ms: 1 }), { status: 200 })
  }
  try {
    const { summary } = await executeBatchAction({ batchId: 'power', generation: 1, serials: ['device'], action: 'power' })
    assert.equal(summary.success, 1)
    assert.equal(calls.length, 1)
    assert.equal(calls[0].url, '/api/v1/actions/events')
    assert.equal(calls[0].body.event, 'power')
  } finally { globalThis.fetch = previousFetch }
})

function mockScreenRequests(t, screenState) {
  const calls = []
  t.mock.method(console, 'info', () => {})
  t.mock.method(globalThis, 'fetch', async (url, init = {}) => {
    const body = init.body ? JSON.parse(init.body) : undefined
    calls.push({ url, body })
    return Response.json(body
      ? { status: 'completed', exit_code: 0 }
      : screenState)
  })
  return calls
}

test('batch lock sends sleep and verifies screen off for each selected device', async t => {
  const calls = mockScreenRequests(t, { screenOn: false })
  const { summary } = await executeBatchAction({ batchId: 'lock', generation: 1, serials: ['a:5555', 'b:5555'], action: 'lock' })
  assert.equal(summary.success, 2)
  assert.deepEqual(calls.filter(call => call.body).map(call => call.body.args), [
    ['shell', 'input', 'keyevent', 'KEYCODE_SLEEP'],
    ['shell', 'input', 'keyevent', 'KEYCODE_SLEEP'],
  ])
  for (const serial of ['a%3A5555', 'b%3A5555'])
    assert.ok(calls.some(call => !call.body && call.url === `/api/screen-state?serial=${serial}`))
})

test('batch unlock wakes and dismisses keyguard, then verifies the unlocked state', async t => {
  const calls = mockScreenRequests(t, { screenOn: true, screenLocked: false })
  const { summary } = await executeBatchAction({ batchId: 'unlock', generation: 1, serials: ['a:5555'], action: 'unlock' })
  assert.equal(summary.success, 1)
  assert.deepEqual(calls.filter(call => call.body).map(call => call.body.args), [
    ['shell', 'input', 'keyevent', 'KEYCODE_WAKEUP'],
    ['shell', 'wm', 'dismiss-keyguard'],
  ])
  assert.equal(calls.at(-1).url, '/api/screen-state?serial=a%3A5555&lock=1')
})

test('batch unlock reports a remaining password lock as failure', async t => {
  mockScreenRequests(t, { screenOn: true, screenLocked: true })
  const { summary, results } = await executeBatchAction({ batchId: 'secure-unlock', generation: 1, serials: ['secure'], action: 'unlock' })
  assert.equal(summary.success, 0)
  assert.equal(summary.failed, 1)
  assert.match(results[0].error, /请在设备上完成解锁/)
})
