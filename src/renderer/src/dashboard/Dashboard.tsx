import { useEffect, useMemo, useState, type JSX, type ReactNode } from 'react'
import { SECTION_ORDER, useApp } from '../store'
import { Section } from './Section'
import { AgentCard } from './AgentCard'
import { AgentDetail } from './AgentDetail'
import { SystemBanner } from './SystemBanner'
import { MissedRunsBanner } from './MissedRunsBanner'

export function Dashboard(): JSX.Element {
  const { agents, runs, missed, loading, status } = useApp()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const cliMissing = status?.cliMissing === true

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

  const missedByAgent = useMemo(() => {
    const m = new Map<string, number>()
    for (const x of missed) m.set(x.agentId, (m.get(x.agentId) ?? 0) + 1)
    return m
  }, [missed])

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
        <SystemBanner />
        {cliMissing ? null : <MissedRunsBanner />}

        {cliMissing ? null : agents.length === 0 ? (
          <EmptyAgents />
        ) : (
          grouped.map((g) => {
            const scheduledCount = g.items.filter((a) => a.scheduled).length
            return (
              <Section
                key={g.name}
                title={g.name}
                agentCount={g.items.length}
                scheduledCount={scheduledCount}
              >
                <div className="grid gap-4 [grid-template-columns:repeat(auto-fill,minmax(320px,380px))]">
                  {g.items.map((agent) => (
                    <AgentCard
                      key={agent.id}
                      agent={agent}
                      recentRun={lastRunByJob.get(agent.id)}
                      missedCount={missedByAgent.get(agent.id) ?? 0}
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
          <AgentDetail agent={selectedAgent} onClose={() => setSelectedId(null)} />
        </EditorOverlay>
      )}
    </main>
  )
}

function EmptyAgents(): JSX.Element {
  const rescan = useApp((s) => s.rescanAgents)
  const reveal = (): void => {
    void window.api.agents.revealDir()
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
          onClick={() => void rescan()}
          className="border border-[var(--color-rule-bright)] px-3 py-1.5 font-display text-[10px] font-bold uppercase text-[var(--color-fg-dim)] transition-all hover:border-[var(--color-cool)] hover:text-[var(--color-cool)]"
          style={{ letterSpacing: '0.24em' }}
        >
          refresh
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
      if (e.key === 'Escape' && !e.defaultPrevented) onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [onClose])

  return (
    <div
      className="bg-overlay fixed inset-0 z-50 flex justify-center items-start overflow-y-auto px-4 pb-16 pt-20 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="w-[min(96vw,1300px)]"
      >
        {children}
      </div>
    </div>
  )
}
