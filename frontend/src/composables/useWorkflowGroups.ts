import { computed } from 'vue'
import type { Operation } from '../services/api'
import { useWorkStore, type WorkflowGroup } from '../store/work'

/** Live membership never comes from the retained-history page on v2 agents. */
export function useWorkflowGroups() {
  const work = useWorkStore()
  return computed<WorkflowGroup[]>(() => {
    if (work.supported !== false) return work.workflows.slice().sort(byStart)
    const groups = new Map<string, WorkflowGroup>()
    for (const operation of work.legacyActiveOperations) {
      let root: Operation = operation
      const seen = new Set<string>([root.id])
      let unavailable = false
      while (root.parentId && !seen.has(root.parentId)) {
        const parent = work.history[root.parentId] || work.outcomes[root.parentId]?.operation
        if (!parent) {
          unavailable = true
          break
        }
        seen.add(parent.id)
        root = parent
      }
      const key = unavailable ? root.parentId || root.id : root.id
      const group = groups.get(key) || {
        key,
        rootOperationId: key,
        root,
        groupingUnavailable: unavailable,
        operations: [],
        runs: [],
      }
      group.operations.push({ ...operation, version: operation.version || 1 })
      groups.set(key, group)
    }
    return [...groups.values()].sort(byStart)
  })
}
function byStart(a: WorkflowGroup, b: WorkflowGroup) {
  return (a.root?.startedAt || a.runs[0]?.startedAt || '').localeCompare(
    b.root?.startedAt || b.runs[0]?.startedAt || '',
  )
}
