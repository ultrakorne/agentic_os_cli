import { useEffect, useMemo, type JSX } from 'react'
import { useApp } from './store'
import { Dashboard } from './dashboard/Dashboard'

function App(): JSX.Element {
  const init = useApp((s) => s.init)
  const theme = useApp((s) => s.theme)
  const runs = useApp((s) => s.runs)
  const agents = useApp((s) => s.agents)
  const schedules = useApp((s) => s.schedules)

  useEffect(() => {
    const off = init()
    return () => {
      off()
    }
  }, [init])

  const stats = useMemo(() => {
    const running = runs.filter((r) => r.status === 'running').length
    const lastSuccess = runs.find((r) => r.status === 'success')
    return {
      running,
      scheduled: schedules.size,
      lastRun: lastSuccess?.endedAt ?? lastSuccess?.startedAt ?? null
    }
  }, [runs, schedules])

  return (
    <div className="flex min-h-screen flex-col">
      <TopBar themeName={theme?.name ?? '...'} agentCount={agents.length} />
      <Dashboard />
      <BottomBar
        running={stats.running}
        scheduled={stats.scheduled}
        lastRun={stats.lastRun}
      />
    </div>
  )
}

function TopBar({
  themeName,
  agentCount
}: {
  themeName: string
  agentCount: number
}): JSX.Element {
  return (
    <header className="bg-frame relative z-10 flex items-center gap-5 border-b border-[var(--color-rule-bright)] px-5 py-3">
      <div className="flex items-baseline gap-2">
        <span
          className="font-display text-[18px] font-extrabold uppercase text-[var(--color-hot)] neon-text"
          style={{ letterSpacing: '0.34em' }}
        >
          AGENTIC
        </span>
        <span
          className="font-display text-[18px] font-extrabold uppercase text-[var(--color-cool)] neon-text"
          style={{ letterSpacing: '0.34em' }}
        >
          OS
        </span>
        <span className="ml-1 font-mono text-[10px] tracking-[0.2em] text-[var(--color-fg-faint)]">
          v0.1
        </span>
      </div>

      <span className="h-5 w-px bg-[var(--color-rule)]" />

      <div className="flex items-baseline gap-2 text-xs">
        <span className="font-display tabular text-base text-[var(--color-cool)] neon-text-soft">
          {agentCount.toString().padStart(2, '0')}
        </span>
        <span className="uppercase tracking-[0.22em] text-[var(--color-fg-dim)]">
          agents online
        </span>
      </div>

      <div className="ml-auto flex items-baseline gap-3 text-xs">
        <span className="uppercase tracking-[0.22em] text-[var(--color-fg-faint)]">
          theme
        </span>
        <button
          type="button"
          onClick={() => void window.api.theme.reload()}
          title="reload theme from omarchy"
          className="font-display text-xs uppercase text-[var(--color-accent)] transition-colors hover:text-[var(--color-hot)] neon-text-soft"
          style={{ letterSpacing: '0.22em' }}
        >
          {themeName}
        </button>
      </div>
    </header>
  )
}

function BottomBar({
  running,
  scheduled,
  lastRun
}: {
  running: number
  scheduled: number
  lastRun: string | null
}): JSX.Element {
  const hot = running > 0
  return (
    <footer className="bg-frame relative z-10 flex items-center gap-4 border-t border-[var(--color-rule-bright)] px-5 py-2 text-[11px]">
      <span
        className={`flex items-center gap-2 font-display uppercase ${
          hot ? 'text-[var(--color-cool)] neon-text-soft' : 'text-[var(--color-fg-dim)]'
        }`}
        style={{ letterSpacing: '0.24em' }}
      >
        <span
          className={
            hot
              ? 'pulse-soft inline-block size-1.5 rounded-full bg-[var(--color-cool)] shadow-[0_0_10px_currentColor]'
              : 'inline-block size-1.5 rounded-full bg-[var(--color-fg-faint)]'
          }
        />
        {hot ? `running ${running}` : 'system idle'}
      </span>

      <span className="text-[var(--color-rule-bright)]">│</span>

      <span className="flex items-baseline gap-1.5 tabular">
        <span className="font-display text-[var(--color-hot)] neon-text-soft">
          {scheduled.toString().padStart(2, '0')}
        </span>
        <span
          className="uppercase text-[var(--color-fg-dim)]"
          style={{ letterSpacing: '0.22em' }}
        >
          scheduled
        </span>
      </span>

      <span className="ml-auto flex items-baseline gap-2 text-[var(--color-fg-faint)] tabular">
        <span
          className="uppercase"
          style={{ letterSpacing: '0.22em' }}
        >
          last run
        </span>
        <span className="text-[var(--color-fg-dim)]">
          {lastRun ? formatLast(lastRun) : 'never'}
        </span>
      </span>
    </footer>
  )
}

function formatLast(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const m = Math.round(diff / 60_000)
  if (m < 1) return 'just now'
  if (m < 60) return `${m}m ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h ago`
  return `${Math.floor(h / 24)}d ago`
}

export default App
