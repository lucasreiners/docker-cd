import type { RuntimeState } from '../composables/useRuntimeOverview'
import type { StackRecord } from '../services/api'

export function needsAttention(stack: StackRecord, runtime?: RuntimeState): boolean {
  return (
    stack.status === 'failed' ||
    stack.status === 'missing' ||
    (stack.status === 'synced' &&
      stack.containersTotal != null &&
      stack.containersRunning != null &&
      stack.containersRunning < stack.containersTotal) ||
    !!runtime?.error ||
    !!runtime?.containers.some(
      (c) =>
        c.health === 'unhealthy' ||
        c.state === 'restarting' ||
        c.state === 'dead' ||
        (stack.status === 'synced' && c.state !== 'running'),
    )
  )
}

export function runtimeHealth(runtime: RuntimeState): { label: string; tone: string } {
  if (runtime.loading) return { label: 'Checking runtime…', tone: '' }
  if (runtime.error) return { label: 'Runtime unavailable', tone: 'text-warning' }
  if (!runtime.containers.length) return { label: 'No containers', tone: '' }
  if (runtime.containers.some((c) => c.state === 'restarting' || c.state === 'dead'))
    return { label: 'Runtime needs attention', tone: 'text-error' }
  const stopped = runtime.containers.filter((c) => c.state !== 'running')
  if (stopped.length) return { label: `${stopped.length} not running`, tone: 'text-warning' }
  const unhealthy = runtime.containers.filter((c) => c.health === 'unhealthy').length
  if (unhealthy) return { label: `${unhealthy} unhealthy`, tone: 'text-error' }
  const checked = runtime.containers.filter((c) => c.health && c.health !== 'none')
  if (checked.some((c) => c.health === 'starting'))
    return { label: 'Health checks starting', tone: 'text-warning' }
  if (!checked.length) return { label: 'No health checks', tone: '' }
  if (checked.every((c) => c.health === 'healthy')) {
    return {
      label:
        checked.length === runtime.containers.length
          ? 'Health checks passing'
          : `${checked.length}/${runtime.containers.length} health checks passing`,
      tone: 'text-success',
    }
  }
  return { label: 'Health unknown', tone: '' }
}
