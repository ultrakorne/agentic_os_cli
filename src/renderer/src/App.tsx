import { useEffect, useMemo, useRef, useState, type JSX } from 'react'
import { useApp } from './store'
import { Dashboard } from './dashboard/Dashboard'
import type { ThemeSummary } from '@shared/theme'

function App(): JSX.Element {
  const init = useApp((s) => s.init)
  const theme = useApp((s) => s.theme)
  const themes = useApp((s) => s.themes)
  const setTheme = useApp((s) => s.setTheme)
  const rescanAgents = useApp((s) => s.rescanAgents)
  const runs = useApp((s) => s.runs)
  const agents = useApp((s) => s.agents)

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
      scheduled: agents.filter((a) => a.scheduled).length,
      lastRun: lastSuccess?.endedAt ?? lastSuccess?.startedAt ?? null
    }
  }, [runs, agents])

  return (
    <div className="flex min-h-screen flex-col">
      <TopBar
        currentThemeId={theme?.id ?? null}
        currentThemeName={theme?.name ?? '...'}
        themes={themes}
        onPick={(id) => void setTheme(id)}
        onRescan={rescanAgents}
        agentCount={agents.length}
      />
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
  currentThemeId,
  currentThemeName,
  themes,
  onPick,
  onRescan,
  agentCount
}: {
  currentThemeId: string | null
  currentThemeName: string
  themes: ThemeSummary[]
  onPick: (id: string) => void
  onRescan: () => Promise<void>
  agentCount: number
}): JSX.Element {
  return (
    <header className="app-titlebar bg-frame relative z-[60] flex items-center gap-5 border-b border-[var(--color-rule-bright)] px-5 py-3">
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

      <div className="flex items-baseline gap-3 text-xs">
        <span className="font-display tabular text-base text-[var(--color-cool)] neon-text-soft">
          {agentCount.toString().padStart(2, '0')}
        </span>
        <span className="uppercase tracking-[0.22em] text-[var(--color-fg-dim)]">
          agents online
        </span>
        <RescanButton onRescan={onRescan} />
      </div>

      <div className="ml-auto">
        <ThemePicker
          currentThemeId={currentThemeId}
          currentThemeName={currentThemeName}
          themes={themes}
          onPick={onPick}
        />
      </div>
    </header>
  )
}

function RescanButton({ onRescan }: { onRescan: () => Promise<void> }): JSX.Element {
  const [pending, setPending] = useState(false)

  const click = async (): Promise<void> => {
    if (pending) return
    setPending(true)
    try {
      await onRescan()
    } finally {
      setPending(false)
    }
  }

  return (
    <button
      type="button"
      onClick={() => void click()}
      title="run aos refresh: rescan agents and reconcile cron"
      aria-busy={pending}
      className="border border-[var(--color-rule-bright)] px-2 py-0.5 font-display text-[10px] font-bold uppercase text-[var(--color-fg-dim)] transition-colors hover:border-[var(--color-cool)] hover:text-[var(--color-cool)] disabled:opacity-60"
      style={{ letterSpacing: '0.22em' }}
      disabled={pending}
    >
      {pending ? '…' : 'refresh'}
    </button>
  )
}

function ThemePicker({
  currentThemeId,
  currentThemeName,
  themes,
  onPick
}: {
  currentThemeId: string | null
  currentThemeName: string
  themes: ThemeSummary[]
  onPick: (id: string) => void
}): JSX.Element {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return
    const onDoc = (e: MouseEvent): void => {
      if (!containerRef.current?.contains(e.target as Node)) setOpen(false)
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setOpen(false)
    }
    document.addEventListener('mousedown', onDoc)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDoc)
      document.removeEventListener('keydown', onKey)
    }
  }, [open])

  return (
    <div ref={containerRef} className="relative flex items-baseline gap-3 text-xs">
      <span className="uppercase tracking-[0.22em] text-[var(--color-fg-faint)]">theme</span>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        title="change theme"
        aria-haspopup="listbox"
        aria-expanded={open}
        className="font-display text-xs uppercase text-[var(--color-accent)] transition-colors hover:text-[var(--color-hot)] neon-text-soft"
        style={{ letterSpacing: '0.22em' }}
      >
        {currentThemeName}
      </button>
      {open ? (
        <ul
          role="listbox"
          className="bg-frame absolute right-0 top-full z-20 mt-2 min-w-[180px] border border-[var(--color-rule-bright)] py-1 shadow-[0_8px_30px_-8px_rgba(0,0,0,0.6)]"
        >
          {themes.map((t) => {
            const active = t.id === currentThemeId
            return (
              <li key={t.id} role="option" aria-selected={active}>
                <button
                  type="button"
                  onClick={() => {
                    onPick(t.id)
                    setOpen(false)
                  }}
                  className={`flex w-full items-baseline gap-3 px-3 py-1.5 text-left text-xs transition-colors ${
                    active
                      ? 'text-[var(--color-hot)] neon-text-soft'
                      : 'text-[var(--color-fg-dim)] hover:text-[var(--color-accent)]'
                  }`}
                >
                  <span
                    className="font-display uppercase"
                    style={{ letterSpacing: '0.22em' }}
                  >
                    {t.name}
                  </span>
                  {active ? (
                    <span className="ml-auto text-[10px] tracking-[0.22em] text-[var(--color-fg-faint)]">
                      active
                    </span>
                  ) : null}
                </button>
              </li>
            )
          })}
        </ul>
      ) : null}
    </div>
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
    <footer className="bg-frame relative z-[60] flex items-center gap-4 border-t border-[var(--color-rule-bright)] px-5 py-2 text-[11px]">
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
