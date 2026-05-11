import type { JSX } from 'react'
import { formatClock, relativeFromNow } from '../lib/format'

type Props = {
  expectedAt: string
}

export function MissedRunRow({ expectedAt }: Props): JSX.Element {
  return (
    <div
      aria-label="missed scheduled run"
      className="border border-dashed border-[var(--color-rule-bright)] bg-[var(--color-surface)]"
    >
      <div className="grid w-full grid-cols-[auto_1fr_auto_auto_auto_auto] items-center gap-3 px-3 py-2">
        <span
          aria-hidden
          className="inline-flex size-4 items-center justify-center text-base text-[var(--color-hot)] neon-text-soft"
        >
          ▽
        </span>
        <span className="flex min-w-0 flex-col">
          <span
            className="font-display text-[11px] font-bold uppercase text-[var(--color-fg-dim)] tabular"
            style={{ letterSpacing: '0.14em' }}
          >
            {formatClock(expectedAt)}
          </span>
          <span className="text-[10px] text-[var(--color-fg-faint)] tabular">
            expected {relativeFromNow(expectedAt)}
          </span>
        </span>
        <span
          className="inline-block border px-1.5 py-0.5 font-display text-[9px] uppercase"
          style={{
            letterSpacing: '0.22em',
            color: 'var(--color-hot)',
            borderColor: 'var(--color-hot)'
          }}
        >
          missed
        </span>
        <span
          className="font-display text-[10px] uppercase text-[var(--color-fg-faint)] tabular min-w-[3.5em] text-right"
          style={{ letterSpacing: '0.18em' }}
        >
          —
        </span>
        <span
          className="font-display text-[10px] uppercase text-[var(--color-fg-faint)] tabular min-w-[2em] text-right"
          style={{ letterSpacing: '0.18em' }}
        >
          —
        </span>
        <span aria-hidden className="text-transparent select-none">
          ▸
        </span>
      </div>
    </div>
  )
}
