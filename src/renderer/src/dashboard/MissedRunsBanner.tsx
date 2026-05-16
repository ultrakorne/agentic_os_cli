import { useEffect, useMemo, useState, type JSX } from 'react'
import type { JobRun } from '@shared/scheduler'
import { useApp } from '../store'
import { relativeFromNow } from '../lib/format'

const SHOW = 3

// An agent shows up here iff its most-recent run is status:"missed".
// That collapses an outage of many slots into one banner row per agent and
// auto-clears the row the moment a new run (manual or scheduled) lands.
// See agentic_os_cli/MISSES_AS_RUNS_PLAN.md.
function selectBehindAgents(runs: JobRun[]): JobRun[] {
  const latestPerAgent = new Map<string, JobRun>()
  for (const r of runs) {
    const cur = latestPerAgent.get(r.jobId)
    if (!cur || r.startedAt > cur.startedAt) latestPerAgent.set(r.jobId, r)
  }
  const out: JobRun[] = []
  for (const r of latestPerAgent.values()) {
    if (r.status === 'missed') out.push(r)
  }
  out.sort((a, b) => b.startedAt.localeCompare(a.startedAt))
  return out
}

export function MissedRunsBanner(): JSX.Element | null {
  const runs = useApp((s) => s.runs)
  const behind = useMemo(() => selectBehindAgents(runs), [runs])
  const [launchError, setLaunchError] = useState<string | null>(null)

  useEffect(() => {
    if (!launchError) return
    const t = setTimeout(() => setLaunchError(null), 5000)
    return () => clearTimeout(t)
  }, [launchError])

  if (behind.length === 0) return null

  const top = behind.slice(0, SHOW)
  const remaining = behind.length - top.length

  const runNow = async (jobId: string): Promise<void> => {
    setLaunchError(null)
    try {
      const res = await window.api.scheduler.runNow(jobId)
      if (res.status === 'error') {
        setLaunchError(`${jobId}: ${res.error ?? 'run failed to launch'}`)
      }
    } catch (err) {
      setLaunchError(`${jobId}: ${err instanceof Error ? err.message : 'run failed to launch'}`)
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
          {behind.length.toString().padStart(2, '0')}
        </span>
      </div>
      <ul className="mt-2 flex flex-col divide-y divide-[var(--color-rule)] text-[11px]">
        {top.map((r) => (
          <li key={r.id} className="flex items-center gap-3 py-1.5">
            <span
              className="font-display text-[12px] font-bold uppercase text-[var(--color-fg)]"
              style={{ letterSpacing: '0.16em' }}
            >
              {r.jobId}
            </span>
            <span className="text-[var(--color-fg-dim)] tabular">
              expected {relativeFromNow(r.startedAt)}
            </span>
            <button
              type="button"
              onClick={() => {
                void runNow(r.jobId)
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
