import type { CSSProperties } from 'react'

export function glowFrame(color: string): CSSProperties {
  return { ['--glow' as never]: color }
}
