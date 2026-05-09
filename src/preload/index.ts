import { contextBridge, ipcRenderer } from 'electron'
import { electronAPI } from '@electron-toolkit/preload'
import type {
  Agent,
  CrontabStatus,
  JobRun,
  MissedRun,
  Schedule,
  ScheduleSpec
} from '../shared/scheduler'
import type { Theme } from '../shared/theme'

const C = {
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

const api = {
  agents: {
    list: (): Promise<Agent[]> => ipcRenderer.invoke(C.agentsList),
    rescan: (): Promise<Agent[]> => ipcRenderer.invoke(C.agentsRescan),
    revealDir: (): Promise<string> => ipcRenderer.invoke(C.agentsRevealDir)
  },
  scheduler: {
    listSchedules: (): Promise<Schedule[]> => ipcRenderer.invoke(C.schedListSchedules),
    listRuns: (jobId?: string): Promise<JobRun[]> => ipcRenderer.invoke(C.schedListRuns, jobId),
    listMissed: (): Promise<MissedRun[]> => ipcRenderer.invoke(C.schedListMissed),
    readOutput: (runId: string): Promise<string | null> =>
      ipcRenderer.invoke(C.schedReadOutput, runId),
    crontabStatus: (): Promise<CrontabStatus> => ipcRenderer.invoke(C.schedCrontabStatus),
    reconcileCrontab: (): Promise<{ wrote: boolean; conflict: boolean; reason?: string }> =>
      ipcRenderer.invoke(C.schedReconcileCrontab),
    runNow: (jobId: string): Promise<JobRun> => ipcRenderer.invoke(C.schedRunNow, jobId),
    upsert: (schedule: Schedule): Promise<void> => ipcRenderer.invoke(C.schedUpsert, schedule),
    remove: (id: string): Promise<void> => ipcRenderer.invoke(C.schedRemove, id),
    nextRun: (spec: ScheduleSpec): Promise<string | null> =>
      ipcRenderer.invoke(C.schedNextRun, spec),
    onChange: (cb: () => void): (() => void) => {
      const listener = (): void => cb()
      ipcRenderer.on(C.schedChanged, listener)
      return () => ipcRenderer.off(C.schedChanged, listener)
    }
  },
  theme: {
    get: (): Promise<Theme> => ipcRenderer.invoke(C.themeGet),
    reload: (): Promise<Theme> => ipcRenderer.invoke(C.themeReload),
    onChange: (cb: (theme: Theme) => void): (() => void) => {
      const listener = (_e: unknown, theme: Theme): void => cb(theme)
      ipcRenderer.on(C.themeChanged, listener)
      return () => ipcRenderer.off(C.themeChanged, listener)
    }
  }
}

export type AppAPI = typeof api

if (process.contextIsolated) {
  try {
    contextBridge.exposeInMainWorld('electron', electronAPI)
    contextBridge.exposeInMainWorld('api', api)
  } catch (error) {
    console.error(error)
  }
} else {
  // @ts-ignore (define in dts)
  window.electron = electronAPI
  // @ts-ignore (define in dts)
  window.api = api
}
