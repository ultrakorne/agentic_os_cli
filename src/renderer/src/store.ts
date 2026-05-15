import { create } from 'zustand'
import type {
  Agent,
  AgentScanIssue,
  JobRun,
  MissedRun,
  SystemStatus
} from '@shared/scheduler'
import type { Theme, ThemeSummary } from '@shared/theme'
import { applyTheme } from './theme'

type AppState = {
  agents: Agent[]
  runs: JobRun[]
  missed: MissedRun[]
  scanIssues: AgentScanIssue[]
  status: SystemStatus | null
  theme: Theme | null
  themes: ThemeSummary[]
  loading: boolean
  refresh: () => Promise<void>
  rescanAgents: () => Promise<void>
  setTheme: (id: string) => Promise<void>
  init: () => () => void
}

export const useApp = create<AppState>((set, get) => ({
  agents: [],
  runs: [],
  missed: [],
  scanIssues: [],
  status: null,
  theme: null,
  themes: [],
  loading: true,

  refresh: async () => {
    const [agents, runs, missed, status, scanIssues] = await Promise.all([
      window.api.agents.list(),
      window.api.scheduler.listRuns(),
      window.api.scheduler.listMissed(),
      window.api.scheduler.status(),
      window.api.agents.listIssues()
    ])
    set({ agents, runs, missed, status, scanIssues, loading: false })
  },

  // Both the top-bar "rescan" and the SystemBanner "reconcile" route through
  // the CLI: scanning agents and reconciling cron are the same action now.
  rescanAgents: async () => {
    await window.api.scheduler.refresh()
  },

  setTheme: async (id: string) => {
    const theme = await window.api.theme.set(id)
    applyTheme(theme)
    set({ theme })
  },

  init: () => {
    void window.api.theme.get().then((theme) => {
      applyTheme(theme)
      set({ theme })
    })
    void window.api.theme.list().then((themes) => set({ themes }))
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
