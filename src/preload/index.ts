import { contextBridge, ipcRenderer } from 'electron'
import { electronAPI } from '@electron-toolkit/preload'
import type { Agent, JobRun, RefreshSummary, ScheduleSpec, SystemStatus } from '../shared/scheduler'
import type { Theme, ThemeSummary } from '../shared/theme'

const C = {
  agentsList: 'agents:list',
  agentsRevealDir: 'agents:reveal-dir',
  agentsSetSchedule: 'agents:set-schedule',
  agentsSetDescription: 'agents:set-description',
  schedListRuns: 'scheduler:list-runs',
  schedReadOutput: 'scheduler:read-run-output',
  schedRunNow: 'scheduler:run-now',
  schedRefresh: 'scheduler:refresh',
  schedStatus: 'scheduler:status',
  schedChanged: 'scheduler:changed',
  themeGet: 'theme:get',
  themeList: 'theme:list',
  themeSet: 'theme:set',
  themeChanged: 'theme:changed'
} as const

const api = {
  agents: {
    list: (): Promise<Agent[]> => ipcRenderer.invoke(C.agentsList),
    revealDir: (): Promise<string> => ipcRenderer.invoke(C.agentsRevealDir),
    setSchedule: (agentId: string, spec: ScheduleSpec | null): Promise<void> =>
      ipcRenderer.invoke(C.agentsSetSchedule, agentId, spec),
    setDescription: (agentId: string, description: string): Promise<void> =>
      ipcRenderer.invoke(C.agentsSetDescription, agentId, description)
  },
  scheduler: {
    listRuns: (jobId?: string): Promise<JobRun[]> => ipcRenderer.invoke(C.schedListRuns, jobId),
    readOutput: (runId: string): Promise<string | null> =>
      ipcRenderer.invoke(C.schedReadOutput, runId),
    runNow: (agentId: string): Promise<JobRun> => ipcRenderer.invoke(C.schedRunNow, agentId),
    refresh: (): Promise<RefreshSummary | null> => ipcRenderer.invoke(C.schedRefresh),
    status: (): Promise<SystemStatus> => ipcRenderer.invoke(C.schedStatus),
    onChange: (cb: () => void): (() => void) => {
      const listener = (): void => cb()
      ipcRenderer.on(C.schedChanged, listener)
      return () => ipcRenderer.off(C.schedChanged, listener)
    }
  },
  theme: {
    get: (): Promise<Theme> => ipcRenderer.invoke(C.themeGet),
    list: (): Promise<ThemeSummary[]> => ipcRenderer.invoke(C.themeList),
    set: (id: string): Promise<Theme> => ipcRenderer.invoke(C.themeSet, id),
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
