import { onScopeDispose, ref, watch } from 'vue'
import { type ContainerInfo, fetchContainers, type StackRecord } from '../services/api'

export interface RuntimeState {
  containers: ContainerInfo[]
  loading: boolean
  error: string | null
}

// Fetch all stacks, including those outside the active filter. This lets the
// attention filter find an in-sync stack whose container health check fails.
// Each record replacement invalidates only that stack's runtime snapshot.
export function useRuntimeOverview(getStacks: () => StackRecord[]) {
  const runtime = ref(new Map<string, RuntimeState>())
  const records = new Map<string, StackRecord>()
  let disposed = false
  onScopeDispose(() => {
    disposed = true
  })
  watch(
    getStacks,
    (stacks) => {
      const paths = new Set(stacks.map((stack) => stack.path))
      for (const path of records.keys()) {
        if (!paths.has(path)) {
          records.delete(path)
          runtime.value.delete(path)
        }
      }
      for (const stack of stacks) {
        if (records.get(stack.path) === stack) continue
        records.set(stack.path, stack)
        runtime.value.set(stack.path, {
          containers: runtime.value.get(stack.path)?.containers ?? [],
          loading: true,
          error: null,
        })
        const current = () => !disposed && records.get(stack.path) === stack
        fetchContainers(stack.path)
          .then((containers) => {
            if (current())
              runtime.value.set(stack.path, { containers, loading: false, error: null })
          })
          .catch((error: unknown) => {
            if (current())
              runtime.value.set(stack.path, {
                containers: [],
                loading: false,
                error: error instanceof Error ? error.message : 'Runtime unavailable',
              })
          })
      }
    },
    { immediate: true },
  )
  return runtime
}
