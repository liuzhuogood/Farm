import assert from 'node:assert/strict'
import test from 'node:test'
import { UPDATE_SETTINGS_KEY, loadUpdateSettings, normalizeUpdateSettings, updateActionFor } from '../src/composables/useUpdateSettings.ts'

test('update settings default to startup check with manual install', () => {
  assert.deepEqual(normalizeUpdateSettings(undefined), { checkOnStartup: true, autoUpdate: false })
  assert.deepEqual(loadUpdateSettings(), { checkOnStartup: true, autoUpdate: false })
})

test('update settings reject malformed storage without guessing', () => {
  const storage = { getItem: key => key === UPDATE_SETTINGS_KEY ? '{bad json' : null }
  assert.deepEqual(loadUpdateSettings(storage), { checkOnStartup: true, autoUpdate: false })
  assert.deepEqual(normalizeUpdateSettings({ checkOnStartup: 'yes', autoUpdate: true }), { checkOnStartup: true, autoUpdate: false })
})

test('automatic install requires both startup checks and an installable update', () => {
  assert.equal(updateActionFor({ has_update: true, can_upgrade: true }, { checkOnStartup: true, autoUpdate: true }), 'install')
  assert.equal(updateActionFor({ has_update: true, can_upgrade: false }, { checkOnStartup: true, autoUpdate: true }), 'prompt')
  assert.equal(updateActionFor({ has_update: true, can_upgrade: true }, { checkOnStartup: false, autoUpdate: false }), 'prompt')
  assert.equal(updateActionFor({ has_update: false, can_upgrade: false }, { checkOnStartup: true, autoUpdate: true }), 'none')
})

test('invalid stored settings cannot leave auto update enabled without checks', () => {
  assert.deepEqual(normalizeUpdateSettings({ checkOnStartup: false, autoUpdate: true }), { checkOnStartup: false, autoUpdate: false })
})
