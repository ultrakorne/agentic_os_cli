import { describe, expect, it } from 'vitest'
import { parseAgentList, scheduleToArgs } from './agent-list'

describe('parseAgentList', () => {
  it('parses the canonical happy-path payload from `aos list --json`', () => {
    const agents = parseAgentList(
      JSON.stringify({
        agents: [
          {
            id: 'ping',
            section: 'Agents',
            scriptPath: '/data/agents/ping.sh',
            schedule: {
              kind: 'daily',
              days: ['mon', 'tue', 'wed', 'thu', 'fri'],
              hour: 9,
              minute: 0
            },
            cron: '0 9 * * 1,2,3,4,5',
            scheduledAt: '2026-05-15T20:50:04.341Z',
            description: 'Healthcheck'
          }
        ],
        issues: []
      })
    )
    expect(agents).toEqual([
      {
        id: 'ping',
        title: 'Ping',
        description: 'Healthcheck',
        section: 'Agents',
        scriptPath: '/data/agents/ping.sh',
        schedule: {
          kind: 'daily',
          days: ['mon', 'tue', 'wed', 'thu', 'fri'],
          hour: 9,
          minute: 0
        },
        scheduledAt: '2026-05-15T20:50:04.341Z',
        scheduled: true,
        warnings: []
      }
    ])
  })

  it('falls back to humanized id when title is absent', () => {
    const [agent] = parseAgentList(
      JSON.stringify({
        agents: [{ id: 'daily_planner', section: 'assistant', scriptPath: '/data/x.sh' }]
      })
    )
    expect(agent.title).toBe('Daily Planner')
    expect(agent.scheduled).toBe(false)
  })

  it('passes warnings through verbatim', () => {
    const [agent] = parseAgentList(
      JSON.stringify({
        agents: [
          {
            id: 'broken',
            section: 'Agents',
            scriptPath: '/data/agents/broken.sh',
            warnings: ['not-executable']
          }
        ]
      })
    )
    expect(agent.warnings).toEqual(['not-executable'])
  })

  it('defaults warnings to [] when omitted', () => {
    const [agent] = parseAgentList(
      JSON.stringify({
        agents: [{ id: 'a', section: 'Agents', scriptPath: '/x' }]
      })
    )
    expect(agent.warnings).toEqual([])
  })

  it('throws on missing agents array — surfacing a CLI/protocol mismatch instead of silently rendering empty', () => {
    expect(() => parseAgentList('{}')).toThrow(/agents/)
    expect(() => parseAgentList('null')).toThrow(/agents/)
  })

  it('handles an empty agents list', () => {
    expect(parseAgentList(JSON.stringify({ agents: [] }))).toEqual([])
  })
})

describe('scheduleToArgs', () => {
  it('null spec → --off', () => {
    expect(scheduleToArgs(null)).toEqual(['--off'])
  })

  it('hourly → --every-hours / --minute', () => {
    expect(scheduleToArgs({ kind: 'hourly', everyHours: 3, minute: 15 })).toEqual([
      '--every-hours',
      '3',
      '--minute',
      '15'
    ])
  })

  it('daily → --hour / --minute / --days (comma-joined)', () => {
    expect(
      scheduleToArgs({
        kind: 'daily',
        days: ['mon', 'tue', 'wed', 'thu', 'fri'],
        hour: 9,
        minute: 0
      })
    ).toEqual(['--hour', '9', '--minute', '0', '--days', 'mon,tue,wed,thu,fri'])
  })
})
