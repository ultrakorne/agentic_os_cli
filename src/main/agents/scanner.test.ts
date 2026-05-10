import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { promises as fs } from 'fs'
import { tmpdir } from 'os'
import { join } from 'path'
import { scanScripts, findScanIssues } from './scanner'

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
      {
        id: 'top',
        scriptPath: join(dir, 'top.sh'),
        section: 'Agents',
        metaPath: join(dir, 'top.meta.json'),
        meta: {}
      }
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

describe('scanScripts — sidecar meta', () => {
  it('folds <id>.meta.json next to the script into meta', async () => {
    await makeExec(join(dir, 'ping.sh'))
    await fs.writeFile(
      join(dir, 'ping.meta.json'),
      JSON.stringify({
        title: 'Ping',
        description: 'liveness probe',
        schedule: { kind: 'hourly', everyHours: 1, minute: 0 }
      })
    )
    const [entry] = await scanScripts(dir)
    expect(entry.metaPath).toBe(join(dir, 'ping.meta.json'))
    expect(entry.meta.title).toBe('Ping')
    expect(entry.meta.description).toBe('liveness probe')
    expect(entry.meta.schedule).toEqual({ kind: 'hourly', everyHours: 1, minute: 0 })
  })

  it('folds sidecars from subfolders too', async () => {
    await makeExec(join(dir, 'Daily', 'morning.sh'))
    await fs.writeFile(
      join(dir, 'Daily', 'morning.meta.json'),
      JSON.stringify({ title: 'Morning' })
    )
    const [entry] = await scanScripts(dir)
    expect(entry.section).toBe('Daily')
    expect(entry.meta.title).toBe('Morning')
  })

  it('treats a missing sidecar as empty meta', async () => {
    await makeExec(join(dir, 'lonely.sh'))
    const [entry] = await scanScripts(dir)
    expect(entry.meta).toEqual({})
  })

  it('ignores malformed sidecars and warns', async () => {
    await makeExec(join(dir, 'broken.sh'))
    await fs.writeFile(join(dir, 'broken.meta.json'), '{ not json')
    const [entry] = await scanScripts(dir)
    expect(entry.meta).toEqual({})
  })

  it('does not treat a *.meta.json file as a script even if executable', async () => {
    const path = join(dir, 'pretender.meta.json')
    await fs.writeFile(path, '{}')
    await fs.chmod(path, 0o755)
    const scripts = await scanScripts(dir)
    expect(scripts).toEqual([])
  })

  it('skips lone sidecars (no matching script)', async () => {
    await fs.writeFile(
      join(dir, 'ghost.meta.json'),
      JSON.stringify({ title: 'Ghost' })
    )
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

describe('findScanIssues — non-executable detection', () => {
  it('flags a shell-extension file that lacks the executable bit', async () => {
    const path = join(dir, 'planner.sh')
    await fs.writeFile(path, '#!/usr/bin/env bash\necho hi\n')
    const issues = await findScanIssues(dir)
    expect(issues).toEqual([{ kind: 'not-executable', path }])
  })

  it('flags non-executable scripts inside first-level subfolders', async () => {
    const path = join(dir, 'Assistant', 'daily_planner.sh')
    await fs.mkdir(join(dir, 'Assistant'), { recursive: true })
    await fs.writeFile(path, '#!/usr/bin/env bash\necho hi\n')
    const issues = await findScanIssues(dir)
    expect(issues).toEqual([{ kind: 'not-executable', path }])
  })

  it('does not flag executable scripts', async () => {
    await makeExec(join(dir, 'ok.sh'))
    const issues = await findScanIssues(dir)
    expect(issues).toEqual([])
  })

  it('does not flag non-shell files (e.g. .py) — those are out of scope', async () => {
    await fs.writeFile(join(dir, 'thing.py'), 'print("hi")\n')
    const issues = await findScanIssues(dir)
    expect(issues).toEqual([])
  })

  it('flags only no-extension files that have a #! shebang', async () => {
    const withShebang = join(dir, 'launcher')
    const withoutShebang = join(dir, 'datafile')
    await fs.writeFile(withShebang, '#!/usr/bin/env bash\necho hi\n')
    await fs.writeFile(withoutShebang, 'just text, not a script')
    const issues = await findScanIssues(dir)
    expect(issues.map((i) => i.path)).toEqual([withShebang])
  })

  it('ignores sidecars, READMEs, and dotfiles', async () => {
    await fs.writeFile(join(dir, 'foo.meta.json'), '{}')
    await fs.writeFile(join(dir, 'README.md'), '# notes')
    await fs.writeFile(join(dir, '.hidden.sh'), '#!/usr/bin/env bash\n')
    const issues = await findScanIssues(dir)
    expect(issues).toEqual([])
  })
})
