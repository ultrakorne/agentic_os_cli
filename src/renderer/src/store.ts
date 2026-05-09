import { create } from 'zustand'
import type { Agent, CrontabStatus, JobRun, MissedRun, Schedule } from '@shared/scheduler'
import type { Theme } from '@shared/theme'
import { applyTheme } from './theme'

type AppState = {
  agents: Agent[]
  schedules: Map<string, Schedule>
  schedulesById: Map<string, Schedule>
  runs: JobRun[]
  missed: MissedRun[]
  crontabStatus: CrontabStatus | null
  theme: Theme | null
  loading: boolean
  refresh: () => Promise<void>
  rescanAgents: () => Promise<void>
  reconcileCrontab: () => Promise<void>
  init: () => () => void
}

export const useApp = create<AppState>((set, get) => ({
  agents: [],
  schedules: new Map(),
  schedulesById: new Map(),
  runs: [],
  missed: [],
  crontabStatus: null,
  theme: null,
  loading: true,

  refresh: async () => {
    const [agents, schedules, runs, missed, crontabStatus] = await Promise.all([
      window.api.agents.list(),
      window.api.scheduler.listSchedules(),
      window.api.scheduler.listRuns(),
      window.api.scheduler.listMissed(),
      window.api.scheduler.crontabStatus()
    ])
    const byJob = new Map<string, Schedule>()
    const byId = new Map<string, Schedule>()
    for (const s of schedules) {
      byJob.set(s.jobId, s)
      byId.set(s.id, s)
    }
    set({
      agents,
      schedules: byJob,
      schedulesById: byId,
      runs,
      missed,
      crontabStatus,
      loading: false
    })
  },

  rescanAgents: async () => {
    await window.api.agents.rescan()
  },

  reconcileCrontab: async () => {
    await window.api.scheduler.reconcileCrontab()
  },

  init: () => {
    void window.api.theme.get().then((theme) => {
      applyTheme(theme)
      set({ theme })
    })
    const offTheme = window.api.theme.onChange((theme) => {
      applyTheme(theme)
      set({ theme })
    })
    void get().refresh()
    const offSched = window.api.scheduler.onChange(() => {
      void get().refresh()
    })
    return () => {
      offTheme()
      offSched()
    }
  }
}))

export const SECTION_ORDER = ['Agents', 'Daily', 'Engineering', 'Reflection', 'Dev']
