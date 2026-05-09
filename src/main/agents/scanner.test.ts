import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { promises as fs } from 'fs'
import { tmpdir } from 'os'
import { join } from 'path'
import { scanAgents } from './scanner'

let dir: string

beforeEach(async () => {
  dir = await fs.mkdtemp(join(tmpdir(), 'agents-scanner-'))
})

afterEach(async () => {
  await fs.rm(dir, { recursive: true, force: true })
})

async function makeExec(name: string): Promise<void> {
  const path = join(dir, name)
  await fs.writeFile(path, '#!/usr/bin/env bash\necho hi\n')
  await fs.chmod(path, 0o755)
}

describe('scanAgents — extension policy', () => {
  it('lists shell extensions and shebanged files with no extension', async () => {
    await makeExec('a.sh')
    await makeExec('b.bash')
    await makeExec('c.zsh')
    await makeExec('d') // no extension
    const agents = await scanAgents(dir)
    expect(agents.map((a) => a.id).sort()).toEqual(['a', 'b', 'c', 'd'])
  })

  it('does not list non-shell extensions even when executable', async () => {
    await makeExec('shell.sh')
    await makeExec('script.py')
    await makeExec('thing.rb')
    await makeExec('app.js')
    await makeExec('app.mjs')
    await makeExec('app.ts')
    const agents = await scanAgents(dir)
    expect(agents.map((a) => a.id)).toEqual(['shell'])
  })

  it('skips files without the executable bit', async () => {
    const path = join(dir, 'no-exec.sh')
    await fs.writeFile(path, '#!/usr/bin/env bash\necho hi\n')
    // explicitly NOT chmod-ing +x
    const agents = await scanAgents(dir)
    expect(agents).toEqual([])
  })

  it('skips dotfiles, README, and *.meta.json siblings', async () => {
    await makeExec('foo.sh')
    await fs.writeFile(join(dir, '.hidden.sh'), '#!/usr/bin/env bash\n')
    await fs.chmod(join(dir, '.hidden.sh'), 0o755)
    await fs.writeFile(join(dir, 'README.md'), '# notes')
    await fs.writeFile(join(dir, 'foo.meta.json'), '{"title":"Foo"}')
    const agents = await scanAgents(dir)
    expect(agents.map((a) => a.id)).toEqual(['foo'])
    expect(agents[0].title).toBe('Foo')
  })
})
