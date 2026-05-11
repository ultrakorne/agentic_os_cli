import { useCallback, useEffect, useMemo, useState, type JSX } from 'react'
import type { JobRun } from '@shared/scheduler'
import { useApp } from '../store'
import { RunRow } from './RunRow'
import { MissedRunRow } from './MissedRunRow'
import { FullOutputOverlay } from './FullOutputOverlay'

const PAGE_SIZE = 10

type Item =
  | { kind: 'run'; key: string; sortAt: string; run: JobRun }
  | { kind: 'missed'; key: string; sortAt: string; expectedAt: string }

type Props = {
  agentId: string
}

export function RunHistoryList({ agentId }: Props): JSX.Element {
  const [runs, setRuns] = useState<JobRun[]>([])
  const [page, setPage] = useState(0)
  const [expandedRunId, setExpandedRunId] = useState<string | null>(null)
  const [loadingRunId, setLoadingRunId] = useState<string | null>(null)
  const [outputs, setOutputs] = useState<Record<string, string>>({})
  const [fullViewRunId, setFullViewRunId] = useState<string | null>(null)

  const missed = useApp((s) => s.missed)
  const missedForAgent = useMemo(
    () => missed.filter((m) => m.agentId === agentId),
    [missed, agentId]
  )

  const refresh = useCallback(async (): Promise<void> => {
    const next = await window.api.scheduler.listRuns(agentId)
    setRuns(next)
  }, [agentId])

  useEffect(() => {
    void refresh()
    const off = window.api.scheduler.onChange(() => {
      void refresh()
    })
    return off
  }, [refresh])

  // When a previously-running run transitions out of running, drop its cached
  // output so the next expansion fetches the final log.
  useEffect(() => {
    setOutputs((cur) => {
      let next: Record<string, string> | null = null
      for (const r of runs) {
        if (r.status === 'running' && r.id in cur) {
          next ??= { ...cur }
          delete next[r.id]
        }
      }
      return next ?? cur
    })
  }, [runs])

  useEffect(() => {
    setPage(0)
    setExpandedRunId(null)
    setFullViewRunId(null)
  }, [agentId])

  const items = useMemo<Item[]>(() => {
    const runItems: Item[] = runs.map((r) => ({
      kind: 'run',
      key: `run:${r.id}`,
      sortAt: r.startedAt,
      run: r
    }))
    const missedItems: Item[] = missedForAgent.map((m) => ({
      kind: 'missed',
      key: `missed:${m.expectedAt}`,
      sortAt: m.expectedAt,
      expectedAt: m.expectedAt
    }))
    return [...runItems, ...missedItems].sort((a, b) =>
      b.sortAt.localeCompare(a.sortAt)
    )
  }, [runs, missedForAgent])

  const totalPages = Math.max(1, Math.ceil(items.length / PAGE_SIZE))
  const safePage = Math.min(page, totalPages - 1)
  const slice = useMemo(
    () => items.slice(safePage * PAGE_SIZE, safePage * PAGE_SIZE + PAGE_SIZE),
    [items, safePage]
  )

  const handleToggle = async (run: JobRun): Promise<void> => {
    if (expandedRunId === run.id) {
      setExpandedRunId(null)
      return
    }
    setExpandedRunId(run.id)
    if (run.status === 'running') return
    if (run.id in outputs) return
    setLoadingRunId(run.id)
    try {
      const txt = await window.api.scheduler.readOutput(run.id)
      setOutputs((cur) => ({ ...cur, [run.id]: txt ?? '' }))
    } finally {
      setLoadingRunId((cur) => (cur === run.id ? null : cur))
    }
  }

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-baseline gap-3">
        <span
          className="font-display text-[12px] font-bold uppercase text-[var(--color-cool)] neon-text-soft"
          style={{ letterSpacing: '0.28em' }}
        >
          run history
        </span>
        <span
          className="font-display text-[10px] uppercase text-[var(--color-fg-faint)] tabular"
          style={{ letterSpacing: '0.22em' }}
        >
          {runs.length} {runs.length === 1 ? 'run' : 'runs'}
          {missedForAgent.length > 0 && (
            <>
              <span className="px-1.5 text-[var(--color-rule-bright)]">·</span>
              <span className="text-[var(--color-hot)]">
                {missedForAgent.length} missed
              </span>
            </>
          )}
        </span>
      </div>

      {items.length === 0 ? (
        <div className="border border-dashed border-[var(--color-rule)] py-8 text-center">
          <p
            className="font-display text-[11px] uppercase text-[var(--color-fg-faint)]"
            style={{ letterSpacing: '0.28em' }}
          >
            {'// no runs yet'}
          </p>
        </div>
      ) : (
        <>
          <div className="flex flex-col gap-1.5">
            {slice.map((item) =>
              item.kind === 'missed' ? (
                <MissedRunRow key={item.key} expectedAt={item.expectedAt} />
              ) : (
                <RunRow
                  key={item.key}
                  run={item.run}
                  expanded={expandedRunId === item.run.id}
                  output={outputs[item.run.id] ?? null}
                  loading={loadingRunId === item.run.id}
                  onToggle={() => void handleToggle(item.run)}
                  onViewFull={() => setFullViewRunId(item.run.id)}
                />
              )
            )}
          </div>

          {totalPages > 1 && (
            <div
              className="flex items-center gap-3 border-t border-[var(--color-rule)] pt-3 font-display text-[10px] uppercase tabular"
              style={{ letterSpacing: '0.22em' }}
            >
              <PageButton
                label="◂ prev"
                disabled={safePage === 0}
                onClick={() => setPage((p) => Math.max(0, p - 1))}
              />
              <span className="text-[var(--color-fg-dim)]">
                page {safePage + 1} / {totalPages}
              </span>
              <PageButton
                label="next ▸"
                disabled={safePage >= totalPages - 1}
                onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
              />
            </div>
          )}
        </>
      )}

      {fullViewRunId && (
        <FullOutputOverlay
          run={runs.find((r) => r.id === fullViewRunId) ?? null}
          output={outputs[fullViewRunId] ?? null}
          onClose={() => setFullViewRunId(null)}
        />
      )}
    </section>
  )
}

function PageButton({
  label,
  disabled,
  onClick
}: {
  label: string
  disabled: boolean
  onClick: () => void
}): JSX.Element {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="border border-[var(--color-rule-bright)] px-2.5 py-1 font-display text-[10px] font-bold uppercase text-[var(--color-fg-dim)] transition-colors hover:border-[var(--color-cool)] hover:text-[var(--color-cool)] disabled:cursor-not-allowed disabled:border-[var(--color-rule)] disabled:text-[var(--color-fg-faint)] disabled:hover:border-[var(--color-rule)] disabled:hover:text-[var(--color-fg-faint)]"
      style={{ letterSpacing: '0.24em' }}
    >
      {label}
    </button>
  )
}
