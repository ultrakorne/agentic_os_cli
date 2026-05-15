import { type JSX } from 'react'
import { useApp } from '../store'

type Issue = {
  tone: 'danger' | 'warn'
  text: string
}

export function SystemBanner(): JSX.Element | null {
  const status = useApp((s) => s.status)
  const scanIssues = useApp((s) => s.scanIssues)
  const rescan = useApp((s) => s.rescanAgents)

  if (!status) return null
  if (status.cliMissing) return <CliMissingBanner detail={status.lastRefreshError} />

  const issues: Issue[] = []
  const refresh = status.lastRefresh

  // Surface anything `aos refresh` flagged on its last run. The string forms
  // come straight from the CLI's summary line, so we branch on the exact
  // status rather than a derived boolean.
  if (status.lastRefreshError && !refresh) {
    issues.push({ tone: 'danger', text: `aos refresh failed: ${status.lastRefreshError}` })
  }
  if (refresh) {
    if (refresh.wrapper !== 'ok') {
      issues.push({
        tone: 'danger',
        text: 'wrapper.sh missing — run `aos init <path>` to restore it'
      })
    }
    if (refresh.python3 !== 'ok') {
      issues.push({
        tone: 'danger',
        text: 'python3 not on PATH — install python3 to enable scheduled run recording'
      })
    }
    if (refresh.daemon === 'down') {
      issues.push({
        tone: 'danger',
        text: 'cron daemon not running — schedules will not fire (try: systemctl enable --now cronie)'
      })
    }
    if (refresh.cron === 'skipped:no-crontab-bin') {
      issues.push({
        tone: 'danger',
        text: 'crontab not found — install cron (cronie on Arch, vixie-cron, system cron)'
      })
    } else if (refresh.cron === 'skipped:conflict') {
      issues.push({
        tone: 'warn',
        text: 'crontab managed section was edited externally — next refresh will overwrite the managed block'
      })
    } else if (refresh.cron.startsWith('skipped:')) {
      issues.push({
        tone: 'warn',
        text: `crontab not reconciled (${refresh.cron.slice('skipped:'.length)})`
      })
    }
  }
  for (const si of scanIssues) {
    if (si.kind === 'not-executable') {
      issues.push({
        tone: 'warn',
        text: `${shortPath(si.path)} not executable — chmod +x to enable`
      })
    }
  }

  if (issues.length === 0) return null

  return (
    <div className="flex flex-col gap-2">
      {issues.map((issue, i) => {
        const palette =
          issue.tone === 'danger'
            ? 'border-[var(--color-danger)] text-[var(--color-danger)]'
            : 'border-[var(--color-hot)] text-[var(--color-hot)]'
        return (
          <div
            key={i}
            className={`bg-card flex items-center gap-3 border ${palette} px-3 py-2 text-[11px]`}
          >
            <span aria-hidden>{issue.tone === 'danger' ? '▲' : '◇'}</span>
            <span
              className="flex-1 font-display uppercase"
              style={{ letterSpacing: '0.18em' }}
            >
              {issue.text}
            </span>
            {i === issues.length - 1 && (
              <button
                type="button"
                onClick={() => void rescan()}
                title="re-run aos refresh"
                className="border border-[var(--color-rule-bright)] px-2 py-0.5 font-display text-[10px] font-bold uppercase text-[var(--color-fg-dim)] transition-colors hover:border-current hover:text-current"
                style={{ letterSpacing: '0.22em' }}
              >
                refresh
              </button>
            )}
          </div>
        )
      })}
    </div>
  )
}

function CliMissingBanner({ detail }: { detail: string | null }): JSX.Element {
  return (
    <div className="bg-card neon-edge border-[var(--color-danger)] p-5 text-xs">
      <p
        className="font-display text-[14px] uppercase text-[var(--color-danger)] neon-text-soft"
        style={{ letterSpacing: '0.28em' }}
      >
        aos cli not found
      </p>
      <p className="mt-3 leading-relaxed text-[var(--color-fg-dim)]">
        agentic_os is a view over the <code>aos</code> runtime. install the CLI from{' '}
        <code>agentic_os_cli/</code>:
      </p>
      <pre className="mt-3 border border-[var(--color-rule)] bg-[var(--color-surface)] p-3 font-mono text-[11px] leading-relaxed text-[var(--color-fg)]">
        {`cd agentic_os_cli
scripts/install.sh
aos init ~/.aos`}
      </pre>
      {detail && (
        <p
          className="mt-3 font-mono text-[10px] uppercase text-[var(--color-fg-faint)]"
          style={{ letterSpacing: '0.18em' }}
        >
          {detail}
        </p>
      )}
    </div>
  )
}

function shortPath(full: string): string {
  const idx = full.lastIndexOf('/agents/')
  return idx >= 0 ? full.slice(idx + '/agents/'.length) : full
}
