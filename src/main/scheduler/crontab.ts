import { spawn } from 'child_process'
import { promises as fsp } from 'fs'
import { join } from 'path'
import type { ScheduleSpec } from './types'
import { compileToCron } from './spec'
import { TICK_CRON_SCHEDULE, TICK_MARKER_ID } from './tick-command'

export const BEGIN_MARKER = '# BEGIN agentic_os (managed - do not edit)'
export const END_MARKER = '# END agentic_os'

// Advisory lock around the crontab read-modify-write critical section.
// Both the Electron main process and the headless tick CLI call syncCrontab
// — without this lock, a near-simultaneous run can lose updates (the later
// writer's `crontab -` clobbers whatever the earlier writer just installed).
const LOCK_FILENAME = '.crontab.lock'
const LOCK_STALE_MS = 30_000
const LOCK_WAIT_TIMEOUT_MS = 10_000
const LOCK_POLL_MS = 100

export type ManagedExtract = {
  before: string
  managed: string[]
  after: string
  hasMarkers: boolean
  conflict: boolean
}

export type CrontabEntry = {
  agentId: string
  scriptPath: string
  spec: ScheduleSpec
}

export type SyncResult = {
  wrote: boolean
  conflict: boolean
  reason?: string
}

export async function readCrontab(): Promise<string> {
  return await new Promise<string>((resolve, reject) => {
    const cp = spawn('crontab', ['-l'])
    let stdout = ''
    let stderr = ''
    cp.stdout.on('data', (b) => {
      stdout += b.toString()
    })
    cp.stderr.on('data', (b) => {
      stderr += b.toString()
    })
    cp.on('error', reject)
    cp.on('close', (code) => {
      if (code === 0) return resolve(stdout)
      if (/no crontab/i.test(stderr)) return resolve('')
      if (code === 1 && stdout === '') return resolve('')
      reject(new Error(`crontab -l exited ${code}: ${stderr.trim() || '(no stderr)'}`))
    })
  })
}

export async function writeCrontab(text: string): Promise<void> {
  return await new Promise<void>((resolve, reject) => {
    const cp = spawn('crontab', ['-'])
    let stderr = ''
    cp.stderr.on('data', (b) => {
      stderr += b.toString()
    })
    cp.on('error', reject)
    cp.on('close', (code) => {
      if (code === 0) return resolve()
      reject(new Error(`crontab - exited ${code}: ${stderr.trim() || '(no stderr)'}`))
    })
    cp.stdin.write(text.endsWith('\n') ? text : text + '\n')
    cp.stdin.end()
  })
}

export function extractManaged(text: string): ManagedExtract {
  const lines = text.split(/\r?\n/)
  let inBlock = false
  let beginIdx = -1
  let endIdx = -1
  let beginCount = 0
  let endCount = 0
  let conflict = false
  const managed: string[] = []

  for (let i = 0; i < lines.length; i++) {
    const t = lines[i].trim()
    if (t === BEGIN_MARKER) {
      beginCount += 1
      if (inBlock) {
        conflict = true
        continue
      }
      inBlock = true
      if (beginIdx < 0) beginIdx = i
    } else if (t === END_MARKER) {
      endCount += 1
      if (!inBlock) {
        conflict = true
        continue
      }
      inBlock = false
      endIdx = i
    } else if (inBlock) {
      managed.push(lines[i])
    }
  }

  if (inBlock) conflict = true
  if (beginCount > 1 || endCount > 1) conflict = true

  const hasMarkers = beginIdx >= 0 && endIdx >= 0 && !conflict

  let before: string
  let after: string
  if (hasMarkers) {
    before = lines.slice(0, beginIdx).join('\n')
    after = lines.slice(endIdx + 1).join('\n')
  } else {
    before = text
    after = ''
  }

  return { before, managed, after, hasMarkers, conflict }
}

function shellQuote(s: string): string {
  return `'${s.replace(/'/g, "'\\''")}'`
}

export function buildManagedBlock(
  entries: CrontabEntry[],
  wrapperPath: string,
  dataDir: string,
  tickCommand?: string | null
): string {
  const out: string[] = [BEGIN_MARKER]
  if (tickCommand) {
    out.push(`${TICK_CRON_SCHEDULE} ${tickCommand} # agentic_os:${TICK_MARKER_ID}`)
  }
  for (const entry of entries) {
    const cron = compileToCron(entry.spec)
    const cmd = [
      shellQuote(wrapperPath),
      shellQuote(dataDir),
      shellQuote(entry.agentId),
      shellQuote(entry.agentId),
      shellQuote(entry.scriptPath)
    ].join(' ')
    out.push(`${cron} ${cmd} # agentic_os:${entry.agentId}`)
  }
  out.push(END_MARKER)
  return out.join('\n')
}

export async function syncCrontab(args: {
  entries: CrontabEntry[]
  wrapperPath: string
  dataDir: string
  tickCommand?: string | null
  force?: boolean
}): Promise<SyncResult> {
  const release = await acquireCrontabLock(args.dataDir)
  if (!release) {
    return {
      wrote: false,
      conflict: false,
      reason: 'crontab lock contended (another process is syncing)'
    }
  }
  try {
    const current = await readCrontab()
    const ex = extractManaged(current)
    if (ex.conflict && !args.force) {
      return { wrote: false, conflict: true, reason: 'managed section damaged or duplicated' }
    }

    const liveEntries = args.entries
    const baseText = ex.conflict ? purgeAllManaged(current) : current
    const baseEx = ex.conflict ? extractManaged(baseText) : ex

    const next = computeNext(
      baseText,
      baseEx,
      liveEntries,
      args.wrapperPath,
      args.dataDir,
      args.tickCommand
    )

    if (next === current) {
      return { wrote: false, conflict: false }
    }
    await writeCrontab(next)
    return { wrote: true, conflict: false }
  } finally {
    await release()
  }
}

// Returns a release callback once the lock is held, or null if the wait
// timeout elapsed (caller treats that as "skip this round, try again later").
// The lockfile holds the holder's pid for diagnostics. A lockfile older than
// LOCK_STALE_MS is assumed orphaned (process crashed mid-sync) and force-taken.
export async function acquireCrontabLock(
  dataDir: string
): Promise<(() => Promise<void>) | null> {
  await fsp.mkdir(dataDir, { recursive: true })
  const lockPath = join(dataDir, LOCK_FILENAME)
  const start = Date.now()

  while (true) {
    try {
      const handle = await fsp.open(lockPath, 'wx')
      await handle.writeFile(`${process.pid}\n`)
      return async () => {
        await handle.close().catch(() => {})
        await fsp.unlink(lockPath).catch(() => {})
      }
    } catch (err) {
      if ((err as NodeJS.ErrnoException).code !== 'EEXIST') throw err
      try {
        const st = await fsp.stat(lockPath)
        if (Date.now() - st.mtimeMs > LOCK_STALE_MS) {
          await fsp.unlink(lockPath).catch(() => {})
          continue
        }
      } catch {
        continue
      }
      if (Date.now() - start > LOCK_WAIT_TIMEOUT_MS) return null
      await new Promise((r) => setTimeout(r, LOCK_POLL_MS))
    }
  }
}

export function purgeAllManaged(text: string): string {
  const lines = text.split(/\r?\n/)
  const drop = new Set<number>()
  // Greedy match BEGIN→next END as a deletable pair. An unmatched BEGIN or END
  // is treated as a stray marker: only the marker line itself is removed, never
  // the trailing content (so a user-pasted BEGIN with no END can't eat the
  // lines they wrote afterwards).
  let i = 0
  while (i < lines.length) {
    const t = lines[i].trim()
    if (t === BEGIN_MARKER) {
      let j = i + 1
      while (j < lines.length && lines[j].trim() !== END_MARKER) j++
      if (j < lines.length) {
        for (let k = i; k <= j; k++) drop.add(k)
        i = j + 1
        continue
      }
      drop.add(i)
    } else if (t === END_MARKER) {
      drop.add(i)
    }
    i += 1
  }
  return lines.filter((_, idx) => !drop.has(idx)).join('\n')
}

export function computeNext(
  current: string,
  ex: ManagedExtract,
  entries: CrontabEntry[],
  wrapperPath: string,
  dataDir: string,
  tickCommand?: string | null
): string {
  // The block is "non-empty" if there's a tick command or any agent entries.
  // Without either, we strip the markers entirely (existing behavior).
  if (entries.length === 0 && !tickCommand) {
    if (!ex.hasMarkers) return current
    const before = ex.before.replace(/\n+$/, '')
    const after = ex.after.replace(/^\n+/, '')
    if (before.length === 0) return after
    if (after.length === 0) return before + '\n'
    return `${before}\n${after}`
  }

  const block = buildManagedBlock(entries, wrapperPath, dataDir, tickCommand)
  if (ex.hasMarkers) {
    const before = ex.before.replace(/\n+$/, '')
    const after = ex.after.replace(/^\n+/, '')
    const parts = [before, block, after].filter((s) => s.length > 0)
    return parts.join('\n') + '\n'
  }

  const trimmed = current.replace(/\n+$/, '')
  return trimmed.length > 0 ? `${trimmed}\n${block}\n` : `${block}\n`
}
