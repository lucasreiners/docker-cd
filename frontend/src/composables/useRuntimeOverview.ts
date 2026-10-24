import { computed, watch } from 'vue'
import type { ContainerInfo, StackRecord } from '../services/api'
import { useConsoleStore } from '../store/console'

export interface RuntimeState {
  containers: ContainerInfo[]
  loading: boolean
  error: string | null
}

export function useRuntimeOverview(getStacks: () => StackRecord[]) {
  const store = useConsoleStore()
  watch(
    getStacks,
    (stacks) => {
      const missing = stacks.find((stack) => !store.runtime[stack.path])
      if (missing) store.ensureRuntime(missing.path)
    },
    { immediate: true },
  )
  return computed(
    () => new Map(getStacks().map((stack) => [stack.path, store.runtimeFor(stack.path)])),
  )
}
