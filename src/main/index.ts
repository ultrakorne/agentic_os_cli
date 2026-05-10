import { app, shell, BrowserWindow } from 'electron'
import { join, resolve } from 'path'
import { electronApp, optimizer, is } from '@electron-toolkit/utils'
import icon from '../../resources/icon.png?asset'
import { SchedulerEngine } from './scheduler/engine'
import { AgentMetaStore } from './scheduler/agent-meta-store'
import { RunsStore } from './scheduler/runs-store'
import { computeDevTickCommand } from './scheduler/tick-command'
import { broadcastChange, registerIpc } from './ipc'
import { createThemeStore } from './theme/loader'

let engine: SchedulerEngine | null = null

function getResourcesDir(): string {
  if (is.dev) {
    return resolve(__dirname, '../../resources')
  }
  return join(app.getAppPath().replace(/\.asar$/, '.asar.unpacked'), 'resources')
}

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

  mainWindow.on('ready-to-show', () => {
    mainWindow.show()
  })

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

app.whenReady().then(async () => {
  electronApp.setAppUserModelId('com.agentic-os')

  app.on('browser-window-created', (_, window) => {
    optimizer.watchWindowShortcuts(window)
  })

  const userData = app.getPath('userData')
  const dataDir = join(userData, 'data')
  const agentsDir = join(dataDir, 'agents')
  const resourcesDir = getResourcesDir()

  const meta = new AgentMetaStore()
  const runs = new RunsStore(join(dataDir, 'runs'))
  const tickCommand = is.dev
    ? computeDevTickCommand({
        appPath: app.getAppPath(),
        tickLogPath: join(dataDir, 'tick.log')
      })
    : null
  engine = new SchedulerEngine({
    meta,
    runs,
    dataDir,
    agentsDir,
    resourcesDir,
    tickCommand,
    onChange: () => broadcastChange()
  })
  await engine.start()

  const themeStore = createThemeStore(join(dataDir, 'theme.json'))

  registerIpc(engine, agentsDir, themeStore)

  createWindow()

  app.on('activate', () => {
    if (BrowserWindow.getAllWindows().length === 0) createWindow()
  })
})

app.on('window-all-closed', () => {
  engine?.stop()
  if (process.platform !== 'darwin') {
    app.quit()
  }
})
