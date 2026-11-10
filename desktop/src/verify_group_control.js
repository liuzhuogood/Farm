(async () => {
  if (window.__nativeGroupControlStarted) return;
  window.__nativeGroupControlStarted = true;
  const config = window.__nativeGroupControlConfig;
  const invoke = (command, args) => window.__TAURI_INTERNALS__.invoke(command, args);
  const assert = (value, message) => { if (!value) throw new Error(message); };
  const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
  async function waitFor(check, message, timeout = 15000) {
    const deadline = Date.now() + timeout;
    while (Date.now() < deadline) { if (check()) return; await pause(50); }
    throw new Error(message);
  }
  const report = async value => {
    const response = await fetch('/api/desktop/native-verification', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: 'Bearer ' + window.__FARM_DESKTOP_TOKEN__ },
      body: JSON.stringify({ group_control: value }),
    });
    assert(response.ok, 'native group-control report failed: ' + response.status);
  };
  const control = new BroadcastChannel('farm.native-group-verification');
  const production = new BroadcastChannel('farm.group-control.v1');
  const NativeSocket = window.WebSocket;
  const nativeSend = NativeSocket.prototype.send;
  const targetPackets = [];
  const inputPackets = [];
  let targetReady = false;
  let readySource = null;
  let workerResult = null;
  let source;
  let target;
  const result = { testCommandsSuppressed: true };
  control.onmessage = ({ data }) => {
    if (data.serial !== source) return;
    if (data.type === 'ready') readySource = data.serial;
    if (data.type === 'result') workerResult = data;
  };
  production.onmessage = ({ data }) => {
    if (data.type === 'input' && data.sourceSerial === source && data.bytes instanceof Uint8Array)
      inputPackets.push(Array.from(data.bytes));
  };
  // Keep real production connections and metadata. Only the two test Back
  // packets are observed instead of being delivered to the actual devices.
  NativeSocket.prototype.send = function (bytes) {
    const url = new URL(this.url);
    if (url.searchParams.get('serial') === target && bytes instanceof Uint8Array && bytes.length === 2 && bytes[0] === 4) {
      targetPackets.push(Array.from(bytes));
      return;
    }
    return nativeSend.call(this, bytes);
  };
  window.WebSocket = new Proxy(NativeSocket, {
    construct(Constructor, args) {
      const socket = Reflect.construct(Constructor, args);
      const url = new URL(socket.url);
      if (url.searchParams.get('serial') === target && url.searchParams.get('control_only') === '1') {
        socket.addEventListener('message', event => {
          if (typeof event.data !== 'string') return;
          try {
            const meta = JSON.parse(event.data);
            if ((meta.type === 'control.meta' || meta.type === 'meta') && Number.isInteger(meta.width) && Number.isInteger(meta.height) && meta.width > 0 && meta.height > 0)
              targetReady = true;
          } catch { /* Production validates and reports malformed metadata. */ }
        });
      }
      return socket;
    },
  });
  try {
    assert(location.pathname === '/', 'verification must start on the production homepage');
    const response = await fetch('/api/devices');
    assert(response.ok, 'device list request failed');
    const devices = (await response.json()).filter(device => device.state === 'device');
    source = config.source ?? devices[0]?.serial;
    target = config.target ?? devices.find(device => device.serial !== source)?.serial;
    assert(source && target && source !== target, 'two distinct online devices are required');
    assert(devices.some(device => device.serial === source) && devices.some(device => device.serial === target), 'configured source and target must both be online');
    result.source = source;
    result.target = target;
    await waitFor(() => document.querySelector('[data-tab="view"]'), 'production view dock missing');
    document.querySelector('[data-tab="view"]').click();
    await waitFor(() => [...document.querySelectorAll('button')].some(button => button.textContent.trim() === '卡片视图'), 'production card-view selector missing');
    [...document.querySelectorAll('button')].find(button => button.textContent.trim() === '卡片视图').click();
    await waitFor(() => localStorage.getItem('farm.view_mode') === 'deck', 'card view was not selected through UI');
    result.cardViewSelectedThroughUi = true;
    await waitFor(() => document.querySelector('[data-tab="devices"]'), 'production device dock missing');
    document.querySelector('[data-tab="devices"]').click();
    await waitFor(() => document.querySelector('[data-device-grid]'), 'production device selection grid missing');
    const buttons = [...document.querySelectorAll('[data-device-grid] button[data-device-serial]')];
    for (const button of buttons) if (button.getAttribute('aria-pressed') === 'true') button.click();
    await pause(50);
    const targetButton = buttons.find(button => button.dataset.deviceSerial === target);
    assert(targetButton, 'configured target missing from production selection UI');
    targetButton.click();
    await waitFor(() => targetButton.getAttribute('aria-pressed') === 'true', 'target was not selected through UI');
    result.targetSelectedBeforeOpen = true;
    document.querySelector('[data-tab="actions"]').click();
    await waitFor(() => document.querySelector('#group-control-mode'), 'production group-control switch missing');
    const toggle = document.querySelector('#group-control-mode');
    assert(toggle.getAttribute('aria-checked') === 'false', 'verification requires group control initially off');
    toggle.click();
    await waitFor(() => toggle.getAttribute('aria-checked') === 'true', 'production group-control switch did not enable');
    result.enabledBeforeOpen = true;
    await waitFor(() => targetReady, 'production target control-only WebSocket never obtained metadata');
    result.targetConnectionReady = true;
    await report({ ...result, stage: 'opening_source' });
    await invoke('open_device_window', { serial: source, width: 380, height: 700 });
    await waitFor(() => readySource === source, 'new native source WebView did not mount', 20000);
    result.nativeWindowOpenedAfterEnable = true;
    control.postMessage({ type: 'exercise', serial: source, target });
    await waitFor(() => workerResult !== null, 'source canvas verification did not finish', 20000);
    assert(workerResult.ok, workerResult.error);
    result.sourceReceivedActiveState = workerResult.receivedActiveState;
    result.sourceCanvasDispatched = workerResult.canvasDispatched;
    result.sourcePackets = workerResult.sourcePackets;
    await waitFor(() => targetPackets.length >= 2 && inputPackets.length >= 2, 'production input did not return to homepage and dispatch to selected target');
    await pause(150);
    const expected = JSON.stringify([[4, 0], [4, 1]]);
    assert(JSON.stringify(workerResult.sourcePackets) === expected, 'source did not emit exactly one complete Back key');
    assert(JSON.stringify(inputPackets) === expected, 'production bus duplicated or lost source Back packets');
    assert(JSON.stringify(targetPackets) === expected, 'homepage duplicated or lost target Back packets');
    result.homepageReceivedInput = true;
    result.targetDispatchedOnce = true;
    result.inputPackets = inputPackets;
    result.targetPackets = targetPackets;
    result.ok = true;
  } catch (error) {
    result.ok = false;
    result.error = String(error);
  } finally {
    const toggle = document.querySelector('#group-control-mode');
    if (toggle?.getAttribute('aria-checked') === 'true') toggle.click();
    NativeSocket.prototype.send = nativeSend;
    window.WebSocket = NativeSocket;
    production.close();
    control.close();
  }
  await report(result);
})();
