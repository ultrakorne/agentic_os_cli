import { type JSX } from 'react'
import type { Agent } from '@shared/scheduler'
import { describeSchedule } from '../lib/format'
import { ScheduleEditor } from './ScheduleEditor'
import { DescriptionEditor } from './DescriptionEditor'
import { RunHistoryList } from './RunHistoryList'
import { CornerBrackets } from './CornerBrackets'
import { glowFrame } from './styles'

type Props = {
  agent: Agent
  onClose: () => void
}

export function AgentDetail({ agent, onClose }: Props): JSX.Element {
  return (
    <div
      className="bg-card-2 neon-edge-strong relative flex flex-col gap-5 p-6 text-xs"
      style={glowFrame('var(--color-hot)')}
    >
      <CornerBrackets />

      <header className="flex items-baseline gap-3 border-b border-[var(--color-rule)] pb-3">
        <span
          className="font-display text-[15px] font-bold uppercase text-[var(--color-hot)] neon-text"
          style={{ letterSpacing: '0.28em' }}
        >
          agent
        </span>
        <span className="text-[var(--color-rule-bright)]">/</span>
        <span
          className="font-display text-[14px] font-bold uppercase text-[var(--color-fg)]"
          style={{ letterSpacing: '0.18em' }}
        >
          {agent.id}
        </span>
        <span
          className="font-display text-[10px] uppercase text-[var(--color-fg-faint)]"
          style={{ letterSpacing: '0.24em' }}
        >
          [{agent.section}]
        </span>
        <span className="ml-auto flex items-center gap-3">
          <span
            className="font-display text-[10px] uppercase text-[var(--color-fg-faint)]"
            style={{ letterSpacing: '0.24em' }}
          >
            esc to close
          </span>
          <button
            type="button"
            onClick={onClose}
            aria-label="close"
            className="inline-flex items-center gap-1.5 border border-[var(--color-rule-bright)] px-3 py-1.5 font-display text-[11px] font-bold uppercase text-[var(--color-fg-dim)] transition-all hover:border-[var(--color-hot)] hover:text-[var(--color-hot)] hover:shadow-[0_0_18px_-4px_var(--color-hot)] active:translate-y-px"
            style={{ letterSpacing: '0.24em' }}
          >
            <span aria-hidden>✕</span>
            <span>close</span>
          </button>
        </span>
      </header>

      <section className="flex flex-col gap-3 border border-[var(--color-rule)] bg-[var(--color-surface)]">
        <div className="flex items-center gap-3 border-b border-[var(--color-rule)] px-4 py-2">
          <span
            className="font-display text-[12px] font-bold uppercase text-[var(--color-cool)] neon-text-soft"
            style={{ letterSpacing: '0.28em' }}
          >
            schedule
          </span>
          <span className="text-[var(--color-rule-bright)]">/</span>
          <span
            className={`font-display text-[11px] uppercase tabular ${
              agent.schedule
                ? 'text-[var(--color-cool)] neon-text-soft'
                : 'text-[var(--color-fg-faint)]'
            }`}
            style={{ letterSpacing: '0.18em' }}
          >
            {describeSchedule(agent.schedule)}
          </span>
        </div>
        <div className="px-4 pb-4">
          <ScheduleEditor embedded agent={agent} />
        </div>
      </section>

      <DescriptionEditor agent={agent} />

      <RunHistoryList agentId={agent.id} />
    </div>
  )
}
