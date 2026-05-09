import { useEffect, type JSX } from 'react'
import type { JobRun } from '@shared/scheduler'
import { formatClock } from '../lib/format'
import { CornerBrackets } from './CornerBrackets'
import { glowFrame } from './styles'

type Props = {
  run: JobRun | null
  output: string | null
  onClose: () => void
}

export function FullOutputOverlay({ run, output, onClose }: Props): JSX.Element {
  useEffect(() => {
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    }
    window.addEventListener('keydown', onKey, { capture: true })
    return () => window.removeEventListener('keydown', onKey, { capture: true })
  }, [onClose])

  return (
    <div
      className="bg-overlay fixed inset-0 z-[70] flex justify-center items-center p-6 backdrop-blur-md"
      onClick={onClose}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="bg-card-2 neon-edge-strong relative flex h-[90vh] w-[min(96vw,1400px)] flex-col p-5"
        style={glowFrame('var(--color-cool)')}
      >
        <CornerBrackets />

        <header className="flex items-baseline gap-3 border-b border-[var(--color-rule)] pb-3">
          <span
            className="font-display text-[13px] font-bold uppercase text-[var(--color-cool)] neon-text"
            style={{ letterSpacing: '0.28em' }}
          >
            full output
          </span>
          {run && (
            <>
              <span className="text-[var(--color-rule-bright)]">/</span>
              <span
                className="font-display text-[11px] uppercase tabular text-[var(--color-fg-dim)]"
                style={{ letterSpacing: '0.18em' }}
              >
                {formatClock(run.startedAt)}
              </span>
            </>
          )}
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

        <pre className="mt-3 flex-1 overflow-auto whitespace-pre-wrap break-words bg-[var(--color-surface)] p-4 font-mono text-[11px] leading-relaxed text-[var(--color-fg-dim)]">
          {output ?? ''}
        </pre>
      </div>
    </div>
  )
}
