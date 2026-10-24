import { computed, watch } from 'vue'
import type { StackRecord } from '../services/api'
import { useConsoleStore } from '../store/console'

// Detail, inventory and inspectors observe exactly the same cache. No targeted
// array endpoint can overwrite a newer global observation or lose provenance.
export function useStackContainers(getStack: () => StackRecord | undefined) {
  const store = useConsoleStore()
  watch(
    getStack,
    (stack, previous) => {
      if (!stack) return
      store.ensureRuntime(stack.path)
      if (previous && previous.path === stack.path && previous !== stack) store.refreshRuntime()
    },
    { immediate: true },
  )
  const observation = computed(() => {
    const stack = getStack()
    return stack ? store.runtimeFor(stack.path) : null
  })
  return {
    containers: computed(() => observation.value?.containers || []),
    containersLoading: computed(() => observation.value?.loading || false),
    containersError: computed(() => observation.value?.error || null),
    containersStale: computed(() => observation.value?.stale ?? true),
    containersObservedAt: computed(() => observation.value?.observedAt ?? null),
  }
}
