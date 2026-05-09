import { afterEach, beforeEach, describe, it, expect } from 'vitest'
import { promises as fs } from 'fs'
import { tmpdir } from 'os'
import { join } from 'path'
import {
  BEGIN_MARKER,
  END_MARKER,
  acquireCrontabLock,
  buildManagedBlock,
  computeNext,
  extractManaged,
  purgeAllManaged,
  type CrontabEntry
} from './crontab'

const entry = (agentId: string, scriptPath: string): CrontabEntry => ({
  agentId,
  scriptPath,
  spec: { kind: 'hourly', everyHours: 1, minute: 0 }
})

describe('extractManaged', () => {
  it('returns empty when no markers present', () => {
    const text = '0 9 * * 1 echo hi\n'
    const ex = extractManaged(text)
    expect(ex.hasMarkers).toBe(false)
    expect(ex.conflict).toBe(false)
    expect(ex.before).toBe(text)
    expect(ex.managed).toEqual([])
  })

  it('extracts the managed block between BEGIN/END', () => {
    const text = `# user line\n${BEGIN_MARKER}\n0 * * * * /wrapper a b c d\n${END_MARKER}\n# tail`
    const ex = extractManaged(text)
    expect(ex.hasMarkers).toBe(true)
    expect(ex.conflict).toBe(false)
    expect(ex.managed).toEqual(['0 * * * * /wrapper a b c d'])
    expect(ex.before).toBe('# user line')
    expect(ex.after).toBe('# tail')
  })

  it('flags conflict on duplicate BEGIN markers', () => {
    const text = `${BEGIN_MARKER}\nfoo\n${BEGIN_MARKER}\nbar\n${END_MARKER}`
    const ex = extractManaged(text)
    expect(ex.conflict).toBe(true)
  })

  it('flags conflict on dangling BEGIN with no END', () => {
    const text = `${BEGIN_MARKER}\nfoo\n`
    const ex = extractManaged(text)
    expect(ex.conflict).toBe(true)
  })

  it('flags conflict on END without BEGIN', () => {
    const text = `foo\n${END_MARKER}\n`
    const ex = extractManaged(text)
    expect(ex.conflict).toBe(true)
  })

  it('handles empty input', () => {
    const ex = extractManaged('')
    expect(ex.hasMarkers).toBe(false)
    expect(ex.conflict).toBe(false)
    expect(ex.managed).toEqual([])
  })
})

describe('buildManagedBlock', () => {
  it('renders one cron line per entry with shell-quoted args', () => {
    const block = buildManagedBlock(
      [
        entry('ping', '/tmp/agents/ping.sh'),
        entry('foo', "/tmp/agents/o'foo.sh")
      ],
      '/tmp/wrap.sh',
      '/tmp/data'
    )
    const lines = block.split('\n')
    expect(lines[0]).toBe(BEGIN_MARKER)
    expect(lines[lines.length - 1]).toBe(END_MARKER)
    expect(lines).toHaveLength(4)
    expect(lines[1]).toContain("'/tmp/wrap.sh'")
    expect(lines[1]).toContain("'/tmp/data'")
    expect(lines[1]).toContain("'ping'")
    expect(lines[1]).toContain("'/tmp/agents/ping.sh'")
    expect(lines[1]).toContain('# agentic_os:ping')
    expect(lines[2]).toContain("'/tmp/agents/o'\\''foo.sh'")
  })

  it('renders only the entries it is given (caller filters orphans)', () => {
    const block = buildManagedBlock(
      [entry('ping', '/tmp/ping.sh')],
      '/wrap.sh',
      '/data'
    )
    const lines = block.split('\n')
    expect(lines).toHaveLength(3)
    expect(lines[1]).toContain("'ping'")
  })

  it('emits an empty managed block (BEGIN/END only) when no entries and no tick', () => {
    const block = buildManagedBlock([], '/wrap.sh', '/data')
    expect(block).toBe(`${BEGIN_MARKER}\n${END_MARKER}`)
  })

  it('prepends a tick line when a tick command is supplied', () => {
    const block = buildManagedBlock(
      [entry('ping', '/tmp/ping.sh')],
      '/wrap.sh',
      '/data',
      "PATH=/usr/bin:/bin '/repo/node_modules/.bin/tsx' '/repo/src/cli/tick.ts'"
    )
    const lines = block.split('\n')
    expect(lines[0]).toBe(BEGIN_MARKER)
    expect(lines[1]).toMatch(/^\*\/10 \* \* \* \* /)
    expect(lines[1]).toContain('# agentic_os:__tick__')
    expect(lines[2]).toContain("'ping'")
    expect(lines[3]).toBe(END_MARKER)
  })

  it('emits a tick-only block when there are no agent entries', () => {
    const block = buildManagedBlock([], '/wrap.sh', '/data', '/bin/true')
    const lines = block.split('\n')
    expect(lines).toHaveLength(3)
    expect(lines[1]).toContain('/bin/true')
    expect(lines[1]).toContain('# agentic_os:__tick__')
  })
})

describe('purgeAllManaged', () => {
  it('removes matched BEGIN..END pairs', () => {
    const text = `# user keep\n${BEGIN_MARKER}\nmanaged line\n${END_MARKER}\n# trailer`
    const out = purgeAllManaged(text)
    expect(out).toContain('# user keep')
    expect(out).toContain('# trailer')
    expect(out).not.toContain('managed line')
    expect(out).not.toContain(BEGIN_MARKER)
    expect(out).not.toContain(END_MARKER)
  })

  it('preserves user lines after a stray BEGIN with no END (does NOT eat the rest of the file)', () => {
    const text = `# user before\n${BEGIN_MARKER}\nMY OWN CRON LINE\n0 9 * * * me`
    const out = purgeAllManaged(text)
    expect(out).toContain('# user before')
    expect(out).toContain('MY OWN CRON LINE')
    expect(out).toContain('0 9 * * * me')
    expect(out).not.toContain(BEGIN_MARKER)
  })

  it('removes a stray END marker without touching surrounding lines', () => {
    const text = `# a\n${END_MARKER}\n# b`
    const out = purgeAllManaged(text)
    expect(out).toContain('# a')
    expect(out).toContain('# b')
    expect(out).not.toContain(END_MARKER)
  })

  it('handles two valid pairs cleanly', () => {
    const text = `${BEGIN_MARKER}\nA\n${END_MARKER}\n# mid\n${BEGIN_MARKER}\nB\n${END_MARKER}`
    const out = purgeAllManaged(text)
    expect(out).toContain('# mid')
    expect(out).not.toContain('A\n')
    expect(out).not.toContain('B\n')
    expect(out).not.toContain(BEGIN_MARKER)
  })

  it('handles BEGIN..BEGIN..END (greedy first pair, second BEGIN orphaned)', () => {
    const text = `${BEGIN_MARKER}\nfoo\n${BEGIN_MARKER}\nbar\n${END_MARKER}\nkeep`
    const out = purgeAllManaged(text)
    // First BEGIN matched against END (consumes lines 0-4); 'keep' survives
    expect(out).toContain('keep')
    expect(out).not.toContain(BEGIN_MARKER)
    expect(out).not.toContain('foo')
    expect(out).not.toContain('bar')
  })

  // Regression test for the reviewer's exact reported input.
  it('reviewer regression: "# user line\\nBEGIN\\nfoo\\nbar\\n" preserves foo and bar', () => {
    const text = `# user line\n${BEGIN_MARKER}\nfoo\nbar\n`
    const out = purgeAllManaged(text)
    expect(out).toBe(`# user line\nfoo\nbar\n`)
  })
})

describe('computeNext', () => {
  const TICK = "/bin/echo 'tick'"

  it('returns input unchanged when there are no entries, no tick, and no markers', () => {
    const text = '0 9 * * 1 echo hi\n'
    const ex = extractManaged(text)
    const next = computeNext(text, ex, [], '/wrap.sh', '/data')
    expect(next).toBe(text)
  })

  it('strips an existing managed block when there are no entries and no tick', () => {
    const text = `# user pre\n${BEGIN_MARKER}\nold managed line\n${END_MARKER}\n# user post`
    const ex = extractManaged(text)
    const next = computeNext(text, ex, [], '/wrap.sh', '/data')
    expect(next).not.toContain(BEGIN_MARKER)
    expect(next).not.toContain('old managed line')
    expect(next).toContain('# user pre')
    expect(next).toContain('# user post')
  })

  it('appends a tick-only block when there is no existing managed section', () => {
    const text = '0 9 * * 1 echo hi\n'
    const ex = extractManaged(text)
    const next = computeNext(text, ex, [], '/wrap.sh', '/data', TICK)
    expect(next).toContain(BEGIN_MARKER)
    expect(next).toContain(END_MARKER)
    expect(next).toContain(TICK)
    expect(next).toContain('0 9 * * 1 echo hi')
    expect(next).toContain('agentic_os:__tick__')
  })

  it('replaces the existing managed block in place when entries change', () => {
    const text =
      `# user pre\n${BEGIN_MARKER}\nstale\n${END_MARKER}\n# user post`
    const ex = extractManaged(text)
    const entries: CrontabEntry[] = [
      { agentId: 'foo', scriptPath: '/a/foo.sh', spec: { kind: 'hourly', everyHours: 1, minute: 0 } }
    ]
    const next = computeNext(text, ex, entries, '/wrap.sh', '/data', TICK)
    expect(next.indexOf('# user pre')).toBeLessThan(next.indexOf(BEGIN_MARKER))
    expect(next.indexOf(END_MARKER)).toBeLessThan(next.indexOf('# user post'))
    expect(next).not.toContain('stale')
    expect(next).toContain('agentic_os:foo')
    expect(next).toContain('agentic_os:__tick__')
  })

  it('is idempotent: feeding the output back in produces the same text', () => {
    const text = '0 9 * * 1 user-line\n'
    const ex = extractManaged(text)
    const entries: CrontabEntry[] = [
      { agentId: 'a', scriptPath: '/a.sh', spec: { kind: 'hourly', everyHours: 2, minute: 30 } }
    ]
    const once = computeNext(text, ex, entries, '/wrap.sh', '/data', TICK)
    const twice = computeNext(once, extractManaged(once), entries, '/wrap.sh', '/data', TICK)
    expect(twice).toBe(once)
  })
})

describe('acquireCrontabLock', () => {
  let dir: string

  beforeEach(async () => {
    dir = await fs.mkdtemp(join(tmpdir(), 'agentic-lock-'))
  })

  afterEach(async () => {
    await fs.rm(dir, { recursive: true, force: true })
  })

  it('serialises concurrent callers (second waits, then proceeds in order)', async () => {
    const order: string[] = []
    const a = acquireCrontabLock(dir).then(async (release) => {
      if (!release) throw new Error('A failed to acquire')
      order.push('A acquired')
      await new Promise((r) => setTimeout(r, 150))
      order.push('A releasing')
      await release()
    })
    // ensure A wins the race for the lock
    await new Promise((r) => setTimeout(r, 20))
    const b = acquireCrontabLock(dir).then(async (release) => {
      if (!release) throw new Error('B failed to acquire')
      order.push('B acquired')
      await release()
    })
    await Promise.all([a, b])
    expect(order).toEqual(['A acquired', 'A releasing', 'B acquired'])
  })

  it('reclaims a stale lockfile (mtime older than the staleness threshold)', async () => {
    const lockPath = join(dir, '.crontab.lock')
    await fs.writeFile(lockPath, '99999\n')
    // 60s in the past — well over LOCK_STALE_MS (30s)
    const past = Date.now() - 60_000
    await fs.utimes(lockPath, past / 1000, past / 1000)

    const release = await acquireCrontabLock(dir)
    expect(release).not.toBeNull()
    if (release) await release()
  })
})
