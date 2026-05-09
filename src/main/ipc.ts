import { ipcMain, BrowserWindow, shell } from 'electron'
import type { Schedule, ScheduleSpec } from '../shared/scheduler'
import type { Theme } from '../shared/theme'
import type { SchedulerEngine } from './scheduler/engine'
import { loadTheme } from './theme/loader'

export const IPC = {
  agentsList: 'agents:list',
  agentsRescan: 'agents:rescan',
  agentsRevealDir: 'agents:reveal-dir',
  schedListSchedules: 'scheduler:list-schedules',
  schedListRuns: 'scheduler:list-runs',
  schedListMissed: 'scheduler:list-missed',
  schedReadOutput: 'scheduler:read-run-output',
  schedCrontabStatus: 'scheduler:crontab-status',
  schedReconcileCrontab: 'scheduler:reconcile-crontab',
  schedRunNow: 'scheduler:run-now',
  schedUpsert: 'scheduler:upsert',
  schedRemove: 'scheduler:remove',
  schedNextRun: 'scheduler:next-run',
  schedChanged: 'scheduler:changed',
  themeGet: 'theme:get',
  themeReload: 'theme:reload',
  themeChanged: 'theme:changed'
} as const

export function registerIpc(engine: SchedulerEngine, agentsDir: string): void {
  ipcMain.handle(IPC.agentsList, () => engine.listAgents())
  ipcMain.handle(IPC.agentsRescan, async () => {
    const agents = await engine.refreshAgents()
    broadcastChange()
    return agents
  })
  ipcMain.handle(IPC.agentsRevealDir, () => shell.openPath(agentsDir))

  ipcMain.handle(IPC.schedListSchedules, () => engine.listSchedules())
  ipcMain.handle(IPC.schedListRuns, (_e, jobId?: string) => engine.listRuns(jobId))
  ipcMain.handle(IPC.schedListMissed, () => engine.listMissed())
  ipcMain.handle(IPC.schedReadOutput, (_e, runId: string) => engine.readOutput(runId))
  ipcMain.handle(IPC.schedCrontabStatus, () => engine.crontabStatus())
  ipcMain.handle(IPC.schedReconcileCrontab, () => engine.reconcileCrontab())
  ipcMain.handle(IPC.schedRunNow, (_e, jobId: string) => engine.runManually(jobId))
  ipcMain.handle(IPC.schedUpsert, (_e, sched: Schedule) => engine.upsertSchedule(sched))
  ipcMain.handle(IPC.schedRemove, (_e, id: string) => engine.removeSchedule(id))
  ipcMain.handle(IPC.schedNextRun, (_e, spec: ScheduleSpec) => {
    const next = engine.nextRunFor(spec)
    return next ? next.toISOString() : null
  })

  ipcMain.handle(IPC.themeGet, () => loadTheme())
  ipcMain.handle(IPC.themeReload, async () => {
    const theme = await loadTheme()
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
