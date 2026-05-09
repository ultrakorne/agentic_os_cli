import { ElectronAPI } from '@electron-toolkit/preload'
import type { AppAPI } from './index'

declare global {
  interface Window {
    electron: ElectronAPI
    api: AppAPI
  }
}
