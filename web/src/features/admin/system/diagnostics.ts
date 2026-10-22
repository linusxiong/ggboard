import type { AdminLogEntry, AdminSystemStatus } from '@/lib/api/types'
import { toSeconds } from './format'

/**
 * Plain-text diagnostics bundle for bug reports: system status, failing jobs and the most recent
 * warnings/errors. Contains no secrets (settings are not included; the backend masks secrets in logs).
 */
export function buildDiagnostics(
  status: AdminSystemStatus | undefined,
  problems: readonly AdminLogEntry[],
): string {
  const report = {
    generated_at: new Date().toISOString(),
    panel: status
      ? {
          version: status.version,
          go_version: status.go_version,
          db_driver: status.db_driver,
          started_at: status.started_at,
          uptime_seconds: toSeconds(status.uptime),
          goroutines: status.goroutines,
          memory: status.memory,
          online: status.online,
          mail: status.mail ?? (status.mail_queue !== undefined ? { pending: status.mail_queue } : undefined),
          cache: status.cache,
          scheduler: status.scheduler.map((j) => ({
            name: j.name,
            spec: j.spec,
            running: j.running,
            last_run: j.last_run,
            last_error: j.last_error || undefined,
            runs: j.runs,
            failures: j.failures,
          })),
        }
      : null,
    browser:
      typeof navigator !== 'undefined'
        ? { user_agent: navigator.userAgent, language: navigator.language }
        : null,
    recent_problems: problems
      .slice(0, 50)
      .map((e) => ({ time: e.time, level: e.level, msg: e.msg, attrs: e.attrs ?? undefined })),
  }
  return JSON.stringify(report, null, 2)
}
