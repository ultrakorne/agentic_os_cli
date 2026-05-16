import { ipcMain, BrowserWindow, shell } from 'electron'
import { join } from 'path'
import type { ScheduleSpec, SystemStatus } from '../shared/scheduler'
import type { Theme } from '../shared/theme'
import type { AppService } from './service'
import type { ThemeStore } from './theme/loader'

export const IPC = {
  agentsList: 'agents:list',
  agentsRevealDir: 'agents:reveal-dir',
  agentsSetSchedule: 'agents:set-schedule',
  agentsSetDescription: 'agents:set-description',
  schedListRuns: 'scheduler:list-runs',
  schedListMissed: 'scheduler:list-missed',
  schedReadOutput: 'scheduler:read-run-output',
  schedRunNow: 'scheduler:run-now',
  schedNextRun: 'scheduler:next-run',
  schedRefresh: 'scheduler:refresh',
  schedStatus: 'scheduler:status',
  schedChanged: 'scheduler:changed',
  themeGet: 'theme:get',
  themeList: 'theme:list',
  themeSet: 'theme:set',
  themeChanged: 'theme:changed'
} as const

export type ServiceHandle = {
  service: AppService | null
  themeStore: ThemeStore
  // Populated only when service is null (CLI missing / not initialized).
  cliMissingStatus?: SystemStatus
}

// Renderer calls into a null service return either empty data (lists) or
// throw with this message — the renderer should always check status first
// and gate the UI off cliMissing=true.
const NO_CLI = 'aos CLI not configured'

export function registerIpc(handle: ServiceHandle): void {
  const { service, themeStore } = handle

  ipcMain.handle(IPC.agentsList, () => service?.listAgents() ?? [])
  ipcMain.handle(IPC.agentsRevealDir, () => {
    if (!service) return ''
    return shell.openPath(join(service.aosHome, 'agents'))
  })
  ipcMain.handle(IPC.agentsSetSchedule, (_e, agentId: string, spec: ScheduleSpec | null) => {
    if (!service) throw new Error(NO_CLI)
    return service.setSchedule(agentId, spec)
  })
  ipcMain.handle(IPC.agentsSetDescription, (_e, agentId: string, description: string) => {
    if (!service) throw new Error(NO_CLI)
    return service.setDescription(agentId, description)
  })

  ipcMain.handle(IPC.schedListRuns, (_e, jobId?: string) => service?.listRuns(jobId) ?? [])
  ipcMain.handle(IPC.schedListMissed, () => service?.listMissed() ?? [])
  ipcMain.handle(IPC.schedReadOutput, (_e, runId: string) => service?.readOutput(runId) ?? null)
  ipcMain.handle(IPC.schedRunNow, (_e, agentId: string) => {
    if (!service) throw new Error(NO_CLI)
    return service.runManually(agentId)
  })
  ipcMain.handle(IPC.schedNextRun, (_e, spec: ScheduleSpec) => {
    if (!service) return null
    const next = service.nextRunFor(spec)
    return next ? next.toISOString() : null
  })
  ipcMain.handle(IPC.schedRefresh, () => {
    if (!service) return null
    return service.refresh()
  })
  ipcMain.handle(IPC.schedStatus, (): SystemStatus => {
    if (service) return service.status()
    return (
      handle.cliMissingStatus ?? {
        cliMissing: true,
        aosBin: null,
        aosHome: null,
        lastRefresh: null,
        lastRefreshError: null
      }
    )
  })

  ipcMain.handle(IPC.themeGet, () => themeStore.load())
  ipcMain.handle(IPC.themeList, () => themeStore.list())
  ipcMain.handle(IPC.themeSet, async (_e, id: string) => {
    const theme = await themeStore.set(id)
    broadcastTheme(theme)
    return theme
  })
}

export function broadcastChange(): void {
  for (const win of BrowserWindow.getAllWindows()) {
    win.webContents.send(IPC.schedChanged)
  }
}

export function broadcastTheme(theme: Theme): void {
  for (const win of BrowserWindow.getAllWindows()) {
    win.webContents.send(IPC.themeChanged, theme)
  }
}
