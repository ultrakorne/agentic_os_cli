import { promises as fs } from 'node:fs'

// Truncate `path` to roughly its last `keepBytes` when it grows past `maxBytes`.
// Aligns the cut to the next newline so the file never starts mid-line.
// Safe to interleave with cron's O_APPEND writes: append always seeks to the
// current end-of-file at write time, so a write that fires after this trim
// lands at the new (smaller) end.
export async function trimLog(
  path: string,
  maxBytes: number,
  keepBytes: number
): Promise<void> {
  let size: number
  try {
    const st = await fs.stat(path)
    size = st.size
  } catch {
    return
  }
  if (size <= maxBytes) return

  const offset = Math.max(0, size - keepBytes)
  const len = size - offset
  const fd = await fs.open(path, 'r')
  try {
    const buf = Buffer.alloc(len)
    await fd.read(buf, 0, len, offset)
    const nl = buf.indexOf(0x0a)
    const trimmed = nl >= 0 ? buf.subarray(nl + 1) : buf
    await fs.writeFile(path, trimmed)
  } finally {
    await fd.close()
  }
}
