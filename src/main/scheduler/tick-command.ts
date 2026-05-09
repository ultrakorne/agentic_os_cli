import { join } from 'node:path'

export const TICK_CRON_SCHEDULE = '*/10 * * * *'
export const TICK_MARKER_ID = '__tick__'

function shellQuote(s: string): string {
  return `'${s.replace(/'/g, "'\\''")}'`
}

export type DevTickCommandOpts = {
  appPath: string
  tickLogPath: string
}

// Returns the cron command line (everything after the schedule) that re-runs
// the engine tick from a development checkout. Bypasses pnpm/fnm entirely:
// invokes the project's own tsx binary with absolute paths and a hardcoded
// PATH so cron's minimal env can still find /usr/bin/node (tsx's shebang
// requires it).
//
// Packaged builds need a separate function — the binary will eventually take
// a --tick flag, and that path will be exposed via its own helper. There is
// deliberately no isDev flag here; the function name is the assumption.
export function computeDevTickCommand(opts: DevTickCommandOpts): string {
  const tsx = join(opts.appPath, 'node_modules', '.bin', 'tsx')
  const script = join(opts.appPath, 'src', 'cli', 'tick.ts')
  return `PATH=/usr/bin:/bin ${shellQuote(tsx)} ${shellQuote(script)} >> ${shellQuote(opts.tickLogPath)} 2>&1`
}
