import type { JSX, ReactNode } from 'react'

type Props = {
  title: string
  agentCount: number
  scheduledCount: number
  children: ReactNode
}

export function Section({
  title,
  agentCount,
  scheduledCount,
  children
}: Props): JSX.Element {
  return (
    <section className="space-y-4">
      <header className="flex items-end gap-4 pb-2">
        <h2
          className="font-display text-[22px] font-extrabold uppercase text-[var(--color-accent)] neon-text"
          style={{ letterSpacing: '0.32em' }}
        >
          {title.toUpperCase()}
        </h2>

        <div className="mb-1 flex items-baseline gap-2 text-[10px] uppercase tabular">
          <span
            className="text-[var(--color-cool)] neon-text-soft"
            style={{ letterSpacing: '0.22em' }}
          >
            {agentCount.toString().padStart(2, '0')}
          </span>
          <span
            className="text-[var(--color-fg-faint)]"
            style={{ letterSpacing: '0.22em' }}
          >
            agent{agentCount === 1 ? '' : 's'}
          </span>
          {scheduledCount > 0 && (
            <>
              <span className="text-[var(--color-rule-bright)]">·</span>
              <span
                className="text-[var(--color-hot)] neon-text-soft"
                style={{ letterSpacing: '0.22em' }}
              >
                {scheduledCount.toString().padStart(2, '0')}
              </span>
              <span
                className="text-[var(--color-fg-faint)]"
                style={{ letterSpacing: '0.22em' }}
              >
                scheduled
              </span>
            </>
          )}
        </div>

        <span
          className="mb-2 h-px flex-1 self-end"
          style={{
            background:
              'linear-gradient(90deg, var(--color-accent) 0%, var(--color-hot) 30%, var(--color-cool) 70%, transparent 100%)',
            boxShadow: '0 0 8px -1px var(--color-hot)'
          }}
        />
      </header>
      <div>{children}</div>
    </section>
  )
}
