import { spawn } from 'child_process'

// One-shot wait for a child process's stdout/stderr to drain and the exit code
// to be available. Returns ({code, stdout, stderr}); never throws.
export function execCapture(
  bin: string,
  args: string[]
): Promise<{ code: number; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    const cp = spawn(bin, args, { stdio: ['ignore', 'pipe', 'pipe'] })
    let stdout = ''
    let stderr = ''
    cp.stdout.on('data', (chunk: Buffer) => {
      stdout += chunk.toString('utf8')
    })
    cp.stderr.on('data', (chunk: Buffer) => {
      stderr += chunk.toString('utf8')
    })
    cp.on('error', (err) => {
      resolve({ code: -1, stdout, stderr: stderr || err.message })
    })
    cp.on('close', (code) => {
      resolve({ code: code ?? -1, stdout, stderr })
    })
  })
}
