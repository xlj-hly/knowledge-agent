import { spawn } from 'node:child_process'
import type { ChildProcess } from 'node:child_process'
import { randomUUID } from 'node:crypto'
import { mkdtemp, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { stopBackend, waitForBackendExit, waitForBackendIdentity } from './backend'

function observe(child: ChildProcess) {
  let output = ''
  child.stdout?.on('data', chunk => { output += String(chunk) })
  child.stderr?.on('data', chunk => { output += String(chunk) })
  const exited = new Promise<number | null>((resolve, reject) => { child.on('error', reject); child.on('exit', resolve) })
  return { exited, output: () => output }
}

export default async function setup() {
  if (!process.env.SAAS_TEST_DATABASE_URL) throw new Error('Set SAAS_TEST_DATABASE_URL to a disposable PostgreSQL database; browser tests never use DATABASE_URL.')
  const runId = randomUUID()
  const baseUrl = 'http://127.0.0.1:8081'
  const workdir = fileURLToPath(new URL('../../server', import.meta.url))
  const directory = await mkdtemp(join(tmpdir(), 'agent-saas-e2e-'))
  const binary = join(directory, process.platform === 'win32' ? 'saas.test.exe' : 'saas.test')
  let child: ChildProcess | undefined
  let running: ReturnType<typeof observe> | undefined
  try {
    // 直接运行编译产物，清理时只持有本次创建的进程 PID。
    const build = spawn('go', ['test', '-c', '-o', binary, './tests/integration'], { cwd: workdir, stdio: ['ignore', 'pipe', 'pipe'] })
    const buildResult = observe(build)
    try { await waitForBackendExit(buildResult.exited, 60_000) } catch (error) {
      build.kill()
      await waitForBackendExit(buildResult.exited, 5_000).catch(() => {})
      throw new Error(`E2E backend build failed: ${buildResult.output()}`, { cause: error })
    }
    child = spawn(binary, ['-test.run=^TestSaaSBrowserServer$', '-test.timeout=15m'], {
      cwd: workdir,
      env: { ...process.env, E2E_SERVER_ADDR: '127.0.0.1:8081', E2E_RUN_ID: runId },
      stdio: ['ignore', 'pipe', 'pipe'],
    })
    running = observe(child)
    const startup = new AbortController()
    try {
      await Promise.race([
        waitForBackendIdentity(baseUrl, runId, { signal: startup.signal }),
        running.exited.then(() => { throw new Error(`E2E backend exited before its handshake: ${running?.output()}`) }),
      ])
    } finally { startup.abort() }
  } catch (error) {
    if (child && running) {
      // 只终止本次创建的进程，不能通过端口查找并终止其他实例。
      if (child.exitCode === null) child.kill()
      await waitForBackendExit(running.exited, 5_000).catch(() => {})
    }
    await rm(directory, { recursive: true, force: true })
    throw error
  }
  const server = child
  const result = running
  if (!server || !result) throw new Error('E2E backend process was not created')
  return async () => {
    try { await stopBackend(baseUrl, runId, result.exited) } catch (error) {
      if (server.exitCode === null) server.kill()
      await waitForBackendExit(result.exited, 5_000).catch(() => {})
      throw new Error(`E2E backend cleanup failed for run ${runId}: ${result.output()}`, { cause: error })
    } finally { await rm(directory, { recursive: true, force: true }) }
  }
}
