import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { promises as fs } from 'fs'
import { tmpdir } from 'os'
import { join } from 'path'
import { trimLog } from './log-trim'

describe('trimLog', () => {
  let dir: string
  let path: string

  beforeEach(async () => {
    dir = await fs.mkdtemp(join(tmpdir(), 'agentic-log-'))
    path = join(dir, 'tick.log')
  })

  afterEach(async () => {
    await fs.rm(dir, { recursive: true, force: true })
  })

  it('no-op when file is missing', async () => {
    await trimLog(path, 100, 50)
    await expect(fs.stat(path)).rejects.toThrow()
  })

  it('no-op when file is at or below maxBytes', async () => {
    const content = 'short content\n'
    await fs.writeFile(path, content)
    await trimLog(path, 1000, 500)
    expect((await fs.readFile(path, 'utf8'))).toBe(content)
  })

  it('keeps a tail aligned to the next newline when over maxBytes', async () => {
    const lines = Array.from({ length: 100 }, (_, i) => `line-${i}`).join('\n') + '\n'
    await fs.writeFile(path, lines)
    await trimLog(path, 200, 100)
    const out = await fs.readFile(path, 'utf8')
    expect(out.length).toBeLessThanOrEqual(100)
    expect(out.startsWith('line-')).toBe(true)
    expect(out.endsWith('line-99\n')).toBe(true)
  })

  it('drops a leading partial line if the cut lands mid-line', async () => {
    // 5-byte lines so the cut is guaranteed mid-line at offset 8 (size=15, keep=7)
    await fs.writeFile(path, 'AAAA\nBBBB\nCCCC\n')
    await trimLog(path, 10, 7)
    const out = await fs.readFile(path, 'utf8')
    // tail of 7 bytes from offset 8 is "BBB\nCCCC\n" minus the leading partial
    expect(out).toBe('CCCC\n')
  })
})
