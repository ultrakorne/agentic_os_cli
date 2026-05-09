import { describe, it, expect } from 'vitest'
import {
  BEGIN_MARKER,
  END_MARKER,
  buildManagedBlock,
  extractManaged,
  purgeAllManaged
} from './crontab'
import type { Schedule } from './types'

const sched = (id: string, jobId: string): Schedule => ({
  id,
  jobId,
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
  it('renders one cron line per non-orphan schedule with shell-quoted args', () => {
    const block = buildManagedBlock(
      [
        { schedule: sched('s1', 'ping'), scriptPath: '/tmp/agents/ping.sh' },
        { schedule: sched('s2', 'foo'), scriptPath: "/tmp/agents/o'foo.sh" }
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
    expect(lines[1]).toContain("'s1'")
    expect(lines[1]).toContain("'ping'")
    expect(lines[1]).toContain("'/tmp/agents/ping.sh'")
    expect(lines[1]).toContain('# agentic_os:s1')
    expect(lines[2]).toContain("'/tmp/agents/o'\\''foo.sh'")
  })

  it('skips orphan schedules', () => {
    const block = buildManagedBlock(
      [
        { schedule: { ...sched('s1', 'gone'), orphaned: true }, scriptPath: '' },
        { schedule: sched('s2', 'ping'), scriptPath: '/tmp/ping.sh' }
      ],
      '/wrap.sh',
      '/data'
    )
    const lines = block.split('\n')
    expect(lines).toHaveLength(3)
    expect(lines[1]).toContain("'s2'")
  })

  it('emits an empty managed block (BEGIN/END only) when no entries', () => {
    const block = buildManagedBlock([], '/wrap.sh', '/data')
    expect(block).toBe(`${BEGIN_MARKER}\n${END_MARKER}`)
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
