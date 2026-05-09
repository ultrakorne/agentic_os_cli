import { useCallback, useEffect, useRef, useState, type JSX } from 'react'
import type { Agent } from '@shared/scheduler'

type Props = {
  agent: Agent
}

const AUTOSAVE_DEBOUNCE_MS = 1000

export function DescriptionEditor({ agent }: Props): JSX.Element {
  const [value, setValue] = useState(agent.description ?? '')

  // Refs let the stable `save` callback see the latest value/agent id when
  // invoked async (debounce timer, blur, unmount). Updated via effects so we
  // never mutate refs during render.
  const valueRef = useRef(value)
  const agentIdRef = useRef(agent.id)
  const lastSavedRef = useRef(agent.description ?? '')

  useEffect(() => {
    valueRef.current = value
  }, [value])

  useEffect(() => {
    agentIdRef.current = agent.id
  }, [agent.id])

  // Reset local state when switching to a different agent.
  useEffect(() => {
    setValue(agent.description ?? '')
    lastSavedRef.current = agent.description ?? ''
  }, [agent.id])

  const save = useCallback((): void => {
    const cur = valueRef.current
    if (cur === lastSavedRef.current) return
    lastSavedRef.current = cur
    void window.api.agents.setDescription(agentIdRef.current, cur)
  }, [])

  // Debounced auto-save while typing.
  useEffect(() => {
    if (value === lastSavedRef.current) return
    const t = setTimeout(save, AUTOSAVE_DEBOUNCE_MS)
    return () => clearTimeout(t)
  }, [value, save])

  // Flush on unmount so closing the panel mid-edit always persists.
  useEffect(() => {
    return () => save()
  }, [save])

  return (
    <section className="flex flex-col gap-2">
      <span
        className="font-display text-[12px] font-bold uppercase text-[var(--color-cool)] neon-text-soft"
        style={{ letterSpacing: '0.28em' }}
      >
        description
      </span>
      <textarea
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={save}
        rows={2}
        spellCheck={false}
        placeholder="// short description"
        className="w-full resize-y border border-[var(--color-rule-bright)] bg-[var(--color-surface)] p-2.5 font-mono text-[12px] leading-relaxed text-[var(--color-fg)] outline-none transition-colors placeholder:text-[var(--color-fg-faint)] focus:border-[var(--color-cool)] focus:shadow-[0_0_18px_-6px_var(--color-cool)]"
      />
    </section>
  )
}
