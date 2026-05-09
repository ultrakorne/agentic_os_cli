import { useEffect, useMemo, useState, type JSX, type ReactNode } from 'react'
import { SECTION_ORDER, useApp } from '../store'
import { Section } from './Section'
import { AgentCard } from './AgentCard'
import { ScheduleEditor } from './ScheduleEditor'
import { SystemBanner } from './SystemBanner'
import { MissedRunsBanner } from './MissedRunsBanner'

export function Dashboard(): JSX.Element {
  const { agents, schedules, schedulesById, runs, missed, loading } = useApp()
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const grouped = useMemo(() => {
    const map = new Map<string, typeof agents>()
    for (const agent of agents) {
      const list = map.get(agent.section) ?? []
      list.push(agent)
      map.set(agent.section, list)
    }
    const known = SECTION_ORDER.filter((s) => map.has(s))
    const extras = [...map.keys()].filter((k) => !SECTION_ORDER.includes(k))
    return [...known, ...extras].map((name) => ({ name, items: map.get(name) ?? [] }))
  }, [agents])

  const lastRunByJob = useMemo(() => {
    const m = new Map<string, (typeof runs)[number]>()
    for (const r of runs) {
      const existing = m.get(r.jobId)
      if (!existing || r.startedAt > existing.startedAt) m.set(r.jobId, r)
    }
    return m
  }, [runs])

  const missedByJob = useMemo(() => {
    const m = new Map<string, number>()
    for (const x of missed) m.set(x.jobId, (m.get(x.jobId) ?? 0) + 1)
    return m
  }, [missed])

  const orphanSchedules = useMemo(
    () => [...schedulesById.values()].filter((s) => s.orphaned),
    [schedulesById]
  )

  const selectedAgent = useMemo(
    () => agents.find((a) => a.id === selectedId) ?? null,
    [agents, selectedId]
  )

  if (loading) {
    return (
      <div className="grid h-full place-items-center font-display text-xs uppercase tracking-[0.32em] text-[var(--color-fg-dim)]">
        booting...
      </div>
    )
  }

  return (
    <main className="relative z-[1] flex-1 overflow-y-auto">
      <div className="mx-auto max-w-6xl space-y-10 px-6 py-8">
        <SystemBanner orphanCount={orphanSchedules.length} />
        <MissedRunsBanner />

        {agents.length === 0 ? (
          <EmptyAgents />
        ) : (
          grouped.map((g) => {
            const scheduledCount = g.items.filter((a) => schedules.has(a.id)).length
            return (
              <Section
                key={g.name}
                title={g.name}
                agentCount={g.items.length}
                scheduledCount={scheduledCount}
              >
                <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
                  {g.items.map((agent) => (
                    <AgentCard
                      key={agent.id}
                      agent={agent}
                      schedule={schedules.get(agent.id)}
                      recentRun={lastRunByJob.get(agent.id)}
                      missedCount={missedByJob.get(agent.id) ?? 0}
                      selected={selectedId === agent.id}
                      onSelect={() =>
                        setSelectedId((cur) => (cur === agent.id ? null : agent.id))
                      }
                    />
                  ))}
                </div>
              </Section>
            )
          })
        )}
      </div>

      {selectedAgent && (
        <EditorOverlay onClose={() => setSelectedId(null)}>
          <ScheduleEditor
            agent={selectedAgent}
            current={schedules.get(selectedAgent.id)}
            onClose={() => setSelectedId(null)}
          />
        </EditorOverlay>
      )}
    </main>
  )
}

function EmptyAgents(): JSX.Element {
  const reveal = (): void => {
    void window.api.agents.revealDir()
  }
  const rescan = (): void => {
    void window.api.agents.rescan()
  }
  return (
    <div className="bg-card neon-edge p-8 text-center text-xs">
      <p
        className="font-display text-[14px] uppercase text-[var(--color-fg)] neon-text-soft"
        style={{ letterSpacing: '0.28em' }}
      >
        no agents found
      </p>
      <p className="mt-3 leading-relaxed text-[var(--color-fg-dim)]">
        Drop an executable script into your agents folder. The dashboard auto-discovers any
        executable file and treats its filename (without extension) as the agent id.
      </p>
      <div className="mt-5 flex justify-center gap-3">
        <button
          type="button"
          onClick={reveal}
          className="border border-[var(--color-cool)] px-3 py-1.5 font-display text-[10px] font-bold uppercase text-[var(--color-cool)] transition-all hover:bg-[var(--color-cool)] hover:text-[var(--color-bg)]"
          style={{ letterSpacing: '0.24em' }}
        >
          open agents folder
        </button>
        <button
          type="button"
          onClick={rescan}
          className="border border-[var(--color-rule-bright)] px-3 py-1.5 font-display text-[10px] font-bold uppercase text-[var(--color-fg-dim)] transition-all hover:border-[var(--color-cool)] hover:text-[var(--color-cool)]"
          style={{ letterSpacing: '0.24em' }}
        >
          rescan
        </button>
      </div>
    </div>
  )
}

function EditorOverlay({
  children,
  onClose
}: {
  children: ReactNode
  onClose: () => void
}): JSX.Element {
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div
      className="bg-overlay fixed inset-0 z-50 grid place-items-center px-4 py-10 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-2xl"
      >
        {children}
      </div>
    </div>
  )
}
