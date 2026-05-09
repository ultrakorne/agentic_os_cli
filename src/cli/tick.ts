import { homedir } from 'node:os'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { SchedulerEngine } from '../main/scheduler/engine'
import { AgentConfigStore } from '../main/scheduler/agent-config-store'
import { RunsStore } from '../main/scheduler/runs-store'
import { computeDevTickCommand } from '../main/scheduler/tick-command'
import { trimLog } from './log-trim'

const TICK_LOG_MAX_BYTES = 256 * 1024
const TICK_LOG_KEEP_BYTES = 128 * 1024

function defaultDataDir(): string {
  const home = homedir()
  if (process.platform === 'darwin') {
    return join(home, 'Library', 'Application Support', 'agentic-os', 'data')
  }
  const xdg = process.env.XDG_CONFIG_HOME ?? join(home, '.config')
  return join(xdg, 'agentic-os', 'data')
}

function appRoot(): string {
  // src/cli/tick.ts → repo root is two levels up
  const here = fileURLToPath(new URL('.', import.meta.url))
  return resolve(here, '../..')
}

async function main(): Promise<void> {
  const root = appRoot()
  const dataDir = process.env.AGENTIC_OS_DATA_DIR ?? defaultDataDir()
  const resourcesDir = process.env.AGENTIC_OS_RESOURCES_DIR ?? join(root, 'resources')
  const agentsDir = join(dataDir, 'agents')

  const configs = new AgentConfigStore(join(dataDir, 'agents.json'))
  const runs = new RunsStore(join(dataDir, 'runs'))
  // tick.ts only ever runs from a dev checkout (it's a tsx-driven script
  // under src/). A future packaged build invokes a different entry point.
  const tickCommand = computeDevTickCommand({
    appPath: root,
    tickLogPath: join(dataDir, 'tick.log')
  })
  const engine = new SchedulerEngine({
    configs,
    runs,
    dataDir,
    agentsDir,
    resourcesDir,
    tickCommand
  })

  await engine.start()
  engine.stop()

  const agents = engine.listAgents()
  const scheduled = agents.filter((a) => a.scheduled).length
  const missed = engine.listMissed().length
  const status = await engine.crontabStatus()
  const crontabState = status.error
    ? `error(${status.error})`
    : status.conflict
      ? 'conflict'
      : status.managed
        ? 'managed'
        : 'empty'

  // Bound tick.log growth before our own line lands at end-of-file.
  await trimLog(join(dataDir, 'tick.log'), TICK_LOG_MAX_BYTES, TICK_LOG_KEEP_BYTES)

  console.log(
    `[tick] scripts=${agents.length} scheduled=${scheduled} missed=${missed} crontab=${crontabState}`
  )
}

main().catch((err) => {
  console.error('[tick] failed:', err)
  process.exitCode = 1
})
