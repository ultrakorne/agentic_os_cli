import { spawn } from 'child_process'
import { promises as fs, constants as fsc } from 'fs'
import { homedir, platform } from 'os'
import { delimiter, join } from 'path'

// macOS GUI apps launched from Finder inherit a minimal PATH that excludes the
// common per-user bin dirs the install script writes into. Augment the search
// path so we still find aos when the user installed via `scripts/install.sh`.
function searchDirs(): string[] {
  const fromEnv = (process.env.PATH ?? '').split(delimiter).filter(Boolean)
  const extras = [
    join(homedir(), '.local', 'bin'),
    join(homedir(), 'bin'),
    '/usr/local/bin',
    '/opt/homebrew/bin'
  ]
  return [...new Set([...fromEnv, ...extras])]
}

async function isExecutable(path: string): Promise<boolean> {
  try {
    await fs.access(path, fsc.X_OK)
    const st = await fs.stat(path)
    return st.isFile()
  } catch {
    return false
  }
}

// Searches the user's PATH plus a few well-known install locations for the
// `aos` binary. Returns null if it's not installed.
export async function resolveAosBin(): Promise<string | null> {
  const name = platform() === 'win32' ? 'aos.exe' : 'aos'
  for (const dir of searchDirs()) {
    const candidate = join(dir, name)
    if (await isExecutable(candidate)) return candidate
  }
  return null
}

// Runs `aos home` and returns the resolved path on stdout, or null+reason on
// failure. The CLI exits 1 with a message on stderr when not initialized.
export async function readAosHome(aosBin: string): Promise<{ home: string } | { error: string }> {
  return new Promise((resolve) => {
    const cp = spawn(aosBin, ['home'], { stdio: ['ignore', 'pipe', 'pipe'] })
    let stdout = ''
    let stderr = ''
    cp.stdout.on('data', (c: Buffer) => {
      stdout += c.toString('utf8')
    })
    cp.stderr.on('data', (c: Buffer) => {
      stderr += c.toString('utf8')
    })
    cp.on('error', (err) => resolve({ error: err.message }))
    cp.on('close', (code) => {
      const path = stdout.trim()
      if (code === 0 && path) resolve({ home: path })
      else resolve({ error: stderr.trim() || `aos home exited ${code}` })
    })
  })
}
