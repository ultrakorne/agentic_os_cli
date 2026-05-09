import { describe, expect, it } from 'vitest'
import { TICK_CRON_SCHEDULE, TICK_MARKER_ID, computeDevTickCommand } from './tick-command'

describe('computeDevTickCommand', () => {
  it('emits the expected dev cron command shape', () => {
    const cmd = computeDevTickCommand({
      appPath: '/home/me/agentic_os',
      tickLogPath: '/home/me/.config/agentic-os/data/tick.log'
    })
    expect(cmd).toBe(
      "PATH=/usr/bin:/bin '/home/me/agentic_os/node_modules/.bin/tsx' " +
        "'/home/me/agentic_os/src/cli/tick.ts' >> " +
        "'/home/me/.config/agentic-os/data/tick.log' 2>&1"
    )
  })

  it('escapes single quotes in paths so the cron line stays well-formed', () => {
    const cmd = computeDevTickCommand({
      appPath: "/path/with'apostrophe",
      tickLogPath: "/log/with'quote.log"
    })
    // POSIX single-quote escape: '...'\''...'  (close, escaped quote, reopen)
    expect(cmd).toContain("'/path/with'\\''apostrophe/node_modules/.bin/tsx'")
    expect(cmd).toContain("'/log/with'\\''quote.log'")
  })

  it('hardcodes a minimal PATH so cron-spawned shells can resolve node', () => {
    const cmd = computeDevTickCommand({ appPath: '/x', tickLogPath: '/y' })
    expect(cmd.startsWith('PATH=/usr/bin:/bin ')).toBe(true)
  })

  it('redirects stdout and stderr to the tick log', () => {
    const cmd = computeDevTickCommand({ appPath: '/x', tickLogPath: '/y' })
    expect(cmd.endsWith(">> '/y' 2>&1")).toBe(true)
  })
})

describe('tick constants', () => {
  it('schedule is a valid 5-field cron expression', () => {
    expect(TICK_CRON_SCHEDULE.split(/\s+/)).toHaveLength(5)
  })

  it("marker id uses the reserved __-prefix namespace", () => {
    expect(TICK_MARKER_ID.startsWith('__')).toBe(true)
  })
})
