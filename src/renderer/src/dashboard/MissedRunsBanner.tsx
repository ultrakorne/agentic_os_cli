import { useEffect, useState, type JSX } from 'react'
import { useApp } from '../store'
import { relativeFromNow } from '../lib/format'

const SHOW = 3

export function MissedRunsBanner(): JSX.Element | null {
  const missed = useApp((s) => s.missed)
  const [launchError, setLaunchError] = useState<string | null>(null)

  useEffect(() => {
    if (!launchError) return
    const t = setTimeout(() => setLaunchError(null), 5000)
    return () => clearTimeout(t)
  }, [launchError])

  if (missed.length === 0) return null

  const top = missed.slice(0, SHOW)
  const remaining = missed.length - top.length

  const runNow = async (jobId: string): Promise<void> => {
    setLaunchError(null)
    try {
      const res = await window.api.scheduler.runNow(jobId)
      if (res.status === 'error') {
        setLaunchError(`${jobId}: ${res.error ?? 'run failed to launch'}`)
      }
    } catch (err) {
      setLaunchError(
        `${jobId}: ${err instanceof Error ? err.message : 'run failed to launch'}`
      )
    }
  }

  return (
    <div className="bg-card neon-edge border-[var(--color-hot)] p-3">
      <div className="flex items-baseline gap-2">
        <span
          className="font-display text-[11px] font-bold uppercase text-[var(--color-hot)] neon-text-soft"
          style={{ letterSpacing: '0.28em' }}
        >
          missed runs
        </span>
        <span className="text-[var(--color-rule-bright)]">·</span>
        <span
          className="font-display text-[11px] uppercase text-[var(--color-fg-dim)] tabular"
          style={{ letterSpacing: '0.22em' }}
        >
          {missed.length.toString().padStart(2, '0')}
        </span>
        <span
          className="ml-auto font-display text-[10px] uppercase text-[var(--color-fg-faint)]"
          style={{ letterSpacing: '0.22em' }}
        >
          last 24h
        </span>
      </div>
      <ul className="mt-2 flex flex-col divide-y divide-[var(--color-rule)] text-[11px]">
        {top.map((m) => (
          <li key={`${m.agentId}|${m.expectedAt}`} className="flex items-center gap-3 py-1.5">
            <span
              className="font-display text-[12px] font-bold uppercase text-[var(--color-fg)]"
              style={{ letterSpacing: '0.16em' }}
            >
              {m.agentId}
            </span>
            <span className="text-[var(--color-fg-dim)] tabular">
              expected {relativeFromNow(m.expectedAt)}
            </span>
            <button
              type="button"
              onClick={() => {
                void runNow(m.agentId)
              }}
              className="ml-auto border border-[var(--color-hot)] px-2 py-0.5 font-display text-[10px] font-bold uppercase text-[var(--color-hot)] transition-colors hover:bg-[var(--color-hot)] hover:text-[var(--color-bg)]"
              style={{ letterSpacing: '0.22em' }}
            >
              run now
            </button>
          </li>
        ))}
      </ul>
      {remaining > 0 && (
        <p
          className="mt-1 text-[10px] uppercase text-[var(--color-fg-faint)]"
          style={{ letterSpacing: '0.22em' }}
        >
          and {remaining} more
        </p>
      )}
      {launchError && (
        <p
          role="alert"
          className="mt-2 truncate text-[10px] uppercase text-[var(--color-danger)] neon-text-soft"
          style={{ letterSpacing: '0.16em' }}
          title={launchError}
        >
          ▲ {launchError}
        </p>
      )}
    </div>
  )
}
