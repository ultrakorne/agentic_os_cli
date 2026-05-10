import { useEffect, useState, type JSX } from 'react'
import { useApp } from '../store'

type Action = { label: string; run: () => void; confirm?: boolean }

export function SystemBanner(): JSX.Element | null {
  const status = useApp((s) => s.crontabStatus)
  const scanIssues = useApp((s) => s.scanIssues)
  const reconcile = useApp((s) => s.reconcileCrontab)
  const rescan = useApp((s) => s.rescanAgents)
  const [pendingConfirm, setPendingConfirm] = useState<string | null>(null)

  useEffect(() => {
    if (!pendingConfirm) return
    const t = setTimeout(() => setPendingConfirm(null), 4000)
    return () => clearTimeout(t)
  }, [pendingConfirm])

  const issues: { tone: 'danger' | 'warn'; text: string; action?: Action }[] = []

  if (status) {
    if (!status.crontabOk) {
      issues.push({
        tone: 'danger',
        text: 'crontab not found — install cron (cronie on Arch, or vixie-cron / system cron) to enable scheduling'
      })
    }
    if (status.crontabOk && status.daemonOk === false) {
      issues.push({
        tone: 'danger',
        text: 'cron daemon not running — schedules will not fire (try: systemctl enable --now cronie)'
      })
    }
    if (!status.wrapperOk) {
      issues.push({ tone: 'danger', text: 'wrapper.sh missing — scheduled runs will not record' })
    }
    if (!status.pythonOk) {
      issues.push({
        tone: 'danger',
        text: 'python3 not found on PATH — install python3 to enable scheduled run recording'
      })
    }
    if (status.conflict) {
      issues.push({
        tone: 'warn',
        text: 'crontab managed section was edited externally — reconcile will overwrite the managed block',
        action: { label: 'reconcile', run: () => void reconcile(), confirm: true }
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
        const issueKey = `${i}:${issue.text}`
        const awaitingConfirm = issue.action?.confirm && pendingConfirm === issueKey
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
            {issue.action && (
              <button
                type="button"
                onClick={() => {
                  if (issue.action!.confirm && !awaitingConfirm) {
                    setPendingConfirm(issueKey)
                    return
                  }
                  setPendingConfirm(null)
                  issue.action!.run()
                }}
                className="border border-current px-2 py-0.5 font-display text-[10px] font-bold uppercase transition-colors hover:bg-current hover:text-[var(--color-bg)]"
                style={{ letterSpacing: '0.22em' }}
              >
                {awaitingConfirm ? `confirm ${issue.action.label}?` : issue.action.label}
              </button>
            )}
            {i === issues.length - 1 && (
              <button
                type="button"
                onClick={() => void rescan()}
                title="rescan agents"
                className="border border-[var(--color-rule-bright)] px-2 py-0.5 font-display text-[10px] font-bold uppercase text-[var(--color-fg-dim)] transition-colors hover:border-current hover:text-current"
                style={{ letterSpacing: '0.22em' }}
              >
                rescan
              </button>
            )}
          </div>
        )
      })}
    </div>
  )
}

function shortPath(full: string): string {
  const idx = full.lastIndexOf('/agents/')
  return idx >= 0 ? full.slice(idx + '/agents/'.length) : full
}
