if (!window.__nativeGroupWorkerStarted && document.querySelector('canvas[aria-label="设备投屏画面"]') && document.documentElement.classList.contains('desktop-player')) {
  window.__nativeGroupWorkerStarted = true;
  const serial = new URL(location.href).searchParams.get('serial');
  const control = new BroadcastChannel('mywebscrcpy.native-group-verification');
  const production = new BroadcastChannel('mywebscrcpy.group-control.v1');
  let state = null;
  production.onmessage = ({ data }) => {
    if (data.type === 'state' && data.active === true && Array.isArray(data.serials)) state = data;
  };
  const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
  async function waitFor(check, message) {
    for (let count = 0; count < 300; count++) { if (check()) return; await pause(50); }
    throw new Error(message);
  }
  control.onmessage = async ({ data }) => {
    if (data.type !== 'exercise' || data.serial !== serial) return;
    const sourcePackets = [];
    const nativeSend = WebSocket.prototype.send;
    WebSocket.prototype.send = function (bytes) {
      if (new URL(this.url).searchParams.get('serial') === serial && bytes instanceof Uint8Array && bytes.length === 2 && bytes[0] === 4) {
        sourcePackets.push(Array.from(bytes));
        return;
      }
      return nativeSend.call(this, bytes);
    };
    try {
      // Observe the production coordinator's own state broadcast; do not send a
      // substitute state or request that could conceal a production handshake bug.
      await waitFor(() => state?.serials.includes(data.target), 'new source WebView did not receive active production group-control state');
      await waitFor(() => {
        const canvas = document.querySelector('canvas[aria-label="设备投屏画面"]');
        return canvas && canvas.width > 300 && canvas.height > 300 && !document.body.textContent.includes('连接中') && !document.body.textContent.includes('等待画面');
      }, 'source production player did not obtain a video frame');
      const canvas = document.querySelector('canvas[aria-label="设备投屏画面"]');
      const box = canvas.getBoundingClientRect();
      const event = { button: 2, buttons: 2, bubbles: true, cancelable: true, clientX: box.left + box.width / 2, clientY: box.top + box.height / 2 };
      canvas.dispatchEvent(new MouseEvent('mousedown', event));
      window.dispatchEvent(new MouseEvent('mouseup', { ...event, buttons: 0 }));
      canvas.dispatchEvent(new MouseEvent('contextmenu', { ...event, buttons: 0 }));
      await waitFor(() => sourcePackets.length >= 2, 'production canvas did not emit complete Back input');
      control.postMessage({ type: 'result', serial, ok: true, receivedActiveState: true, canvasDispatched: true, sourcePackets });
    } catch (error) {
      control.postMessage({ type: 'result', serial, ok: false, error: String(error), sourcePackets });
    } finally {
      WebSocket.prototype.send = nativeSend;
    }
  };
  control.postMessage({ type: 'ready', serial });
  window.addEventListener('pagehide', () => { production.close(); control.close(); }, { once: true });
}
