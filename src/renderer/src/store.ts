import { create } from 'zustand'
import type { Agent, JobRun, MissedRun, SystemStatus } from '@shared/scheduler'
import type { Theme, ThemeSummary } from '@shared/theme'
import { applyTheme } from './theme'

type AppState = {
  agents: Agent[]
  runs: JobRun[]
  missed: MissedRun[]
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
  status: null,
  theme: null,
  themes: [],
  loading: true,

  refresh: async () => {
    const [agents, runs, missed, status] = await Promise.all([
      window.api.agents.list(),
      window.api.scheduler.listRuns(),
      window.api.scheduler.listMissed(),
      window.api.scheduler.status()
    ])
    set({ agents, runs, missed, status, loading: false })
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
    // Re-pull on focus so coming back to the app after sleep/wake is instant
    // instead of waiting for the next aos tick (~10 min) or relying on
    // fs.watch surviving the suspend.
    const onFocus = (): void => {
      void get().refresh()
    }
    window.addEventListener('focus', onFocus)
    return () => {
      offTheme()
      offSched()
      window.removeEventListener('focus', onFocus)
    }
  }
}))

export const SECTION_ORDER = ['Agents', 'Daily', 'Engineering', 'Reflection', 'Dev']
