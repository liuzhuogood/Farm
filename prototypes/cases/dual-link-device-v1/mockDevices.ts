export type LinkKind = 'usb' | 'wifi'
export type LinkState = 'online' | 'offline'

export interface DeviceLink {
  kind: LinkKind
  // USB 通道为硬件序列号，无线通道为 IP:端口
  serial: string
  state: LinkState
}

export interface MockLinkDevice {
  identity: string
  identifier: string
  brand: string
  model: string
  androidRelease: string
  battery: number
  batteryTemp: number
  app: { name: string; pkg: string; icon: 'wechat' | 'douyin' | 'standby' }
  links: DeviceLink[]
  active: LinkKind
}

export const MOCK_LINK_DEVICES: MockLinkDevice[] = [
  {
    identity: 'hw:31040209940025Z',
    identifier: '小米11',
    brand: 'Xiaomi',
    model: '2201123C',
    androidRelease: '13',
    battery: 82,
    batteryTemp: 34.5,
    app: { name: '微信', pkg: 'com.tencent.mm', icon: 'wechat' },
    links: [
      { kind: 'usb', serial: '31040209940025Z', state: 'online' },
      { kind: 'wifi', serial: '10.0.0.104:5555', state: 'online' },
    ],
    active: 'usb',
  },
  {
    identity: 'hw:91m897l1vkzprnf',
    identifier: 'Pixel 7',
    brand: 'Google',
    model: 'GVU6C',
    androidRelease: '14',
    battery: 56,
    batteryTemp: 38.2,
    app: { name: '抖音', pkg: 'com.ss.android.ugc.aweme', icon: 'douyin' },
    links: [
      { kind: 'usb', serial: '91m897l1vkzprnf', state: 'offline' },
      { kind: 'wifi', serial: '10.0.0.30:5555', state: 'online' },
    ],
    active: 'wifi',
  },
  {
    identity: 'hw:R58M1234ABC',
    identifier: 'OPPO Reno',
    brand: 'OPPO',
    model: 'PEGM00',
    androidRelease: '13',
    battery: 64,
    batteryTemp: 33.1,
    app: { name: '待机', pkg: '系统待机中', icon: 'standby' },
    links: [
      { kind: 'wifi', serial: '10.0.0.51:5555', state: 'online' },
    ],
    active: 'wifi',
  },
  {
    identity: 'hw:9A221FDH200XYZ',
    identifier: '华为 Mate',
    brand: 'HUAWEI',
    model: 'NOH-AL10',
    androidRelease: '12',
    battery: 91,
    batteryTemp: 31.4,
    app: { name: '微信', pkg: 'com.tencent.mm', icon: 'wechat' },
    links: [
      { kind: 'usb', serial: '9A221FDH200XYZ', state: 'online' },
    ],
    active: 'usb',
  },
  {
    identity: 'hw:31040209940028Z',
    identifier: '小米11 备用',
    brand: 'Xiaomi',
    model: '2201123C',
    androidRelease: '13',
    battery: 47,
    batteryTemp: 40.6,
    app: { name: '待机', pkg: '系统待机中', icon: 'standby' },
    links: [
      { kind: 'usb', serial: '31040209940028Z', state: 'online' },
      { kind: 'wifi', serial: '10.0.0.66:5555', state: 'offline' },
    ],
    active: 'usb',
  },
]
