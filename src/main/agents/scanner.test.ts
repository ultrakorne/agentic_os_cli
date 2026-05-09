import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { promises as fs } from 'fs'
import { tmpdir } from 'os'
import { join } from 'path'
import { scanScripts } from './scanner'

let dir: string

beforeEach(async () => {
  dir = await fs.mkdtemp(join(tmpdir(), 'agents-scanner-'))
})

afterEach(async () => {
  await fs.rm(dir, { recursive: true, force: true })
})

async function makeExec(path: string): Promise<void> {
  await fs.mkdir(join(path, '..'), { recursive: true })
  await fs.writeFile(path, '#!/usr/bin/env bash\necho hi\n')
  await fs.chmod(path, 0o755)
}

describe('scanScripts — extension policy', () => {
  it('lists shell extensions and shebanged files with no extension', async () => {
    await makeExec(join(dir, 'a.sh'))
    await makeExec(join(dir, 'b.bash'))
    await makeExec(join(dir, 'c.zsh'))
    await makeExec(join(dir, 'd')) // no extension
    const scripts = await scanScripts(dir)
    expect(scripts.map((s) => s.id).sort()).toEqual(['a', 'b', 'c', 'd'])
  })

  it('does not list non-shell extensions even when executable', async () => {
    await makeExec(join(dir, 'shell.sh'))
    await makeExec(join(dir, 'script.py'))
    await makeExec(join(dir, 'thing.rb'))
    await makeExec(join(dir, 'app.js'))
    await makeExec(join(dir, 'app.mjs'))
    await makeExec(join(dir, 'app.ts'))
    const scripts = await scanScripts(dir)
    expect(scripts.map((s) => s.id)).toEqual(['shell'])
  })

  it('skips files without the executable bit', async () => {
    const path = join(dir, 'no-exec.sh')
    await fs.writeFile(path, '#!/usr/bin/env bash\necho hi\n')
    const scripts = await scanScripts(dir)
    expect(scripts).toEqual([])
  })

  it('skips dotfiles and README siblings', async () => {
    await makeExec(join(dir, 'foo.sh'))
    await makeExec(join(dir, '.hidden.sh'))
    await fs.writeFile(join(dir, 'README.md'), '# notes')
    const scripts = await scanScripts(dir)
    expect(scripts.map((s) => s.id)).toEqual(['foo'])
  })
})

describe('scanScripts — section from folder structure', () => {
  it('top-level scripts default to section "Agents"', async () => {
    await makeExec(join(dir, 'top.sh'))
    const scripts = await scanScripts(dir)
    expect(scripts).toEqual([
      { id: 'top', scriptPath: join(dir, 'top.sh'), section: 'Agents' }
    ])
  })

  it('first-level subfolder name becomes the section', async () => {
    await makeExec(join(dir, 'Daily', 'morning.sh'))
    await makeExec(join(dir, 'Engineering', 'pr-watch.sh'))
    const scripts = await scanScripts(dir)
    expect(scripts.map((s) => `${s.id}:${s.section}`).sort()).toEqual([
      'morning:Daily',
      'pr-watch:Engineering'
    ])
  })

  it('mixes top-level and subfolder agents', async () => {
    await makeExec(join(dir, 'misc.sh'))
    await makeExec(join(dir, 'Daily', 'morning.sh'))
    const scripts = await scanScripts(dir)
    expect(scripts.map((s) => `${s.id}:${s.section}`).sort()).toEqual([
      'misc:Agents',
      'morning:Daily'
    ])
  })

  it('drops duplicates across folders (first wins, by directory walk order)', async () => {
    await makeExec(join(dir, 'Daily', 'ping.sh'))
    await makeExec(join(dir, 'Engineering', 'ping.sh'))
    const scripts = await scanScripts(dir)
    expect(scripts).toHaveLength(1)
    // top-level gets walked first, then subfolders alphabetically — Daily before Engineering
    expect(scripts[0].section).toBe('Daily')
  })

  it('top-level wins over a subfolder of the same id', async () => {
    await makeExec(join(dir, 'ping.sh'))
    await makeExec(join(dir, 'Daily', 'ping.sh'))
    const scripts = await scanScripts(dir)
    expect(scripts).toHaveLength(1)
    expect(scripts[0].section).toBe('Agents')
  })

  it('ignores nested subfolders deeper than one level', async () => {
    await makeExec(join(dir, 'Daily', 'sub', 'too-deep.sh'))
    const scripts = await scanScripts(dir)
    expect(scripts).toEqual([])
  })

  it('ignores hidden subfolders (dot-prefixed)', async () => {
    await makeExec(join(dir, '.hidden', 'sneaky.sh'))
    const scripts = await scanScripts(dir)
    expect(scripts).toEqual([])
  })
})

describe('scanScripts — reserved id namespace', () => {
  it('drops scripts whose id starts with __ (reserved for internal markers)', async () => {
    await makeExec(join(dir, '__tick__.sh'))
    await makeExec(join(dir, '__internal.sh'))
    await makeExec(join(dir, 'normal.sh'))
    const scripts = await scanScripts(dir)
    expect(scripts.map((s) => s.id)).toEqual(['normal'])
  })

  it('still allows ids that contain __ but do not start with it', async () => {
    await makeExec(join(dir, 'foo__bar.sh'))
    const scripts = await scanScripts(dir)
    expect(scripts.map((s) => s.id)).toEqual(['foo__bar'])
  })
})
