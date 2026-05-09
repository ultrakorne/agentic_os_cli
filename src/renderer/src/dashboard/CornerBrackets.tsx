import type { JSX } from 'react'

type Props = {
  subtle?: boolean
}

export function CornerBrackets({ subtle = false }: Props): JSX.Element {
  const base = subtle
    ? 'pointer-events-none absolute size-2.5 border-[var(--glow)] opacity-80 transition-opacity group-hover:opacity-100'
    : 'pointer-events-none absolute size-3 border-[var(--glow)]'
  return (
    <>
      <span className={`${base} left-0 top-0 border-l border-t`} />
      <span className={`${base} right-0 top-0 border-r border-t`} />
      <span className={`${base} bottom-0 left-0 border-b border-l`} />
      <span className={`${base} bottom-0 right-0 border-b border-r`} />
    </>
  )
}
