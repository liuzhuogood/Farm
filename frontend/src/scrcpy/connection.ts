// 投屏连接方式的展示模型。
//
// Farm 的媒体端点始终是本机服务：WS 通道是浏览器经服务端 TCP 转发，
// WebRTC 通道是浏览器与服务端之间的 UDP 直连（配置 TURN 时才是中继）。
// 真正的浏览器↔设备 P2P 目前不存在，因此这里不提供 p2p 状态。
export interface ConnectionInfo {
  channel: 'ws' | 'webrtc'
  mode: 'direct' | 'relay' | 'forward'
  protocol: 'udp' | 'tcp'
  /** WebRTC 通道的往返时延；其它通道不计算 */
  rttMs?: number
}

export const connectionModeLabel: Record<ConnectionInfo['mode'], string> = {
  direct: '直连',
  relay: '中继',
  forward: '转发',
}

const channelLabel: Record<ConnectionInfo['channel'], string> = {
  ws: 'WebSocket',
  webrtc: 'WebRTC',
}

const protocolLabel: Record<ConnectionInfo['protocol'], string> = {
  udp: 'UDP',
  tcp: 'TCP',
}

/** 明细里的一行：通道与协议 */
export function describeChannel(info: ConnectionInfo) {
  return `${channelLabel[info.channel]} · ${protocolLabel[info.protocol]}`
}

/** 明细里的一行：方式说明 */
export function describeMode(info: ConnectionInfo) {
  switch (info.mode) {
    case 'direct':
      return '直连（UDP 直连服务端）'
    case 'relay':
      return '中继（经 TURN 转发）'
    default:
      return '转发（TCP 经服务端）'
  }
}
