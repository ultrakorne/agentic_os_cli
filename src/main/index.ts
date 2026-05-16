import { app, shell, BrowserWindow } from 'electron'
import { join } from 'path'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import icon from '../../resources/icon.png?asset'
import { AppService } from './service'
import { RunsStore } from './scheduler/runs-store'
import { broadcastChange, registerIpc, type ServiceHandle } from './ipc'
import { createThemeStore, type ThemeStore } from './theme/loader'
import { resolveAosBin, readAosHome } from './cli'
import type { SystemStatus } from '../shared/scheduler'

function createWindow(): void {
  const mainWindow = new BrowserWindow({
    width: 1280,
    height: 820,
    minWidth: 960,
    minHeight: 640,
    show: false,
    autoHideMenuBar: true,
    backgroundColor: '#16161e',
    titleBarStyle: process.platform === 'darwin' ? 'hiddenInset' : 'default',
    ...(process.platform === 'linux' ? { icon } : {}),
    webPreferences: {
      preload: join(__dirname, '../preload/index.js'),
      sandbox: false
    }
  })

  mainWindow.on('ready-to-show', () => mainWindow.show())
  mainWindow.webContents.setWindowOpenHandler((details) => {
    shell.openExternal(details.url)
    return { action: 'deny' }
  })

  if (is.dev && process.env['ELECTRON_RENDERER_URL']) {
    mainWindow.loadURL(process.env['ELECTRON_RENDERER_URL'])
  } else {
    mainWindow.loadFile(join(__dirname, '../renderer/index.html'))
  }
}

let service: AppService | null = null

app.whenReady().then(async () => {
  electronApp.setAppUserModelId('com.agentic-os')

  app.on('browser-window-created', (_, window) => {
    optimizer.watchWindowShortcuts(window)
  })

  // Theme is independent of the aos CLI — it lives in userData and works even
  // when aos is missing, so we can paint the "install aos" banner correctly.
  const themePath = join(app.getPath('userData'), 'theme.json')
  const themeStore = createThemeStore(themePath)

  const handle = await initService(themeStore)
  registerIpc(handle)

  createWindow()

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

async function initService(themeStore: ThemeStore): Promise<ServiceHandle> {
  const aosBin = await resolveAosBin()
  if (!aosBin) {
    return cliMissingHandle(themeStore, {
      cliMissing: true,
      aosBin: null,
      aosHome: null,
      lastRefresh: null,
      lastRefreshError: 'aos not found on PATH — run `scripts/install.sh` in agentic_os_cli/'
    })
  }
  const homeRes = await readAosHome(aosBin)
  if ('error' in homeRes) {
    return cliMissingHandle(themeStore, {
      cliMissing: true,
      aosBin,
      aosHome: null,
      lastRefresh: null,
      lastRefreshError: homeRes.error
    })
  }

  const aosHome = homeRes.home
  const runs = new RunsStore(aosBin, join(aosHome, 'runs'))

  service = new AppService({
    aosBin,
    aosHome,
    runs,
    onChange: () => broadcastChange()
  })
  await service.start()
  // Reconcile cron on boot so the renderer always sees the current crontab
  // state (and so a fresh checkout converges without the user clicking
  // anything). Errors surface in the SystemStatus.lastRefreshError field.
  void service.refresh()

  return { service, themeStore }
}

function cliMissingHandle(themeStore: ThemeStore, status: SystemStatus): ServiceHandle {
  return { service: null, themeStore, cliMissingStatus: status }
}

app.on('window-all-closed', () => {
  service?.stop()
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
