import { ipcMain, BrowserWindow, shell } from 'electron'
import type { ScheduleSpec } from '../shared/scheduler'
import type { Theme } from '../shared/theme'
import type { SchedulerEngine } from './scheduler/engine'
import type { ThemeStore } from './theme/loader'

export const IPC = {
  agentsList: 'agents:list',
  agentsRescan: 'agents:rescan',
  agentsRevealDir: 'agents:reveal-dir',
  agentsSetSchedule: 'agents:set-schedule',
  agentsSetDescription: 'agents:set-description',
  agentsListIssues: 'agents:list-issues',
  schedListRuns: 'scheduler:list-runs',
  schedListMissed: 'scheduler:list-missed',
  schedReadOutput: 'scheduler:read-run-output',
  schedCrontabStatus: 'scheduler:crontab-status',
  schedReconcileCrontab: 'scheduler:reconcile-crontab',
  schedRunNow: 'scheduler:run-now',
  schedNextRun: 'scheduler:next-run',
  schedChanged: 'scheduler:changed',
  themeGet: 'theme:get',
  themeList: 'theme:list',
  themeSet: 'theme:set',
  themeChanged: 'theme:changed'
} as const

export function registerIpc(
  engine: SchedulerEngine,
  agentsDir: string,
  themeStore: ThemeStore
): void {
  ipcMain.handle(IPC.agentsList, () => engine.listAgents())
  ipcMain.handle(IPC.agentsRescan, async () => {
    const agents = await engine.refreshScripts()
    broadcastChange()
    return agents
  })
  ipcMain.handle(IPC.agentsRevealDir, () => shell.openPath(agentsDir))
  ipcMain.handle(
    IPC.agentsSetSchedule,
    (_e, agentId: string, spec: ScheduleSpec | null) => engine.setSchedule(agentId, spec)
  )
  ipcMain.handle(
    IPC.agentsSetDescription,
    (_e, agentId: string, description: string) => engine.setDescription(agentId, description)
  )
  ipcMain.handle(IPC.agentsListIssues, () => engine.listScanIssues())

  ipcMain.handle(IPC.schedListRuns, (_e, jobId?: string) => engine.listRuns(jobId))
  ipcMain.handle(IPC.schedListMissed, () => engine.listMissed())
  ipcMain.handle(IPC.schedReadOutput, (_e, runId: string) => engine.readOutput(runId))
  ipcMain.handle(IPC.schedCrontabStatus, () => engine.crontabStatus())
  ipcMain.handle(IPC.schedReconcileCrontab, () => engine.reconcileCrontab())
  ipcMain.handle(IPC.schedRunNow, (_e, agentId: string) => engine.runManually(agentId))
  ipcMain.handle(IPC.schedNextRun, (_e, spec: ScheduleSpec) => {
    const next = engine.nextRunFor(spec)
    return next ? next.toISOString() : null
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
