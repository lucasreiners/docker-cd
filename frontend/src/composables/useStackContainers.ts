import { ref, watch } from 'vue'
import type { ContainerInfo, StackRecord } from '../services/api'
import { fetchContainers } from '../services/api'

// A full stack record is replaced by each SSE upsert/snapshot, even when a
// redeploy preserves the container count. Refresh runtime details accordingly.
export function useStackContainers(getStack: () => StackRecord | undefined) {
  const containers = ref<ContainerInfo[]>([])
  const containersLoading = ref(false)
  const containersError = ref<string | null>(null)

  watch(
    getStack,
    async (stack, previous, onCleanup) => {
      let stale = false
      onCleanup(() => {
        stale = true
      })
      if (stack?.path !== previous?.path) containers.value = []
      containersError.value = null
      if (!stack) {
        containers.value = []
        containersLoading.value = false
        return
      }
      containersLoading.value = true
      try {
        const result = await fetchContainers(stack.path)
        if (!stale) containers.value = result
      } catch (error) {
        if (!stale)
          containersError.value =
            error instanceof Error ? error.message : 'Failed to load containers'
      } finally {
        if (!stale) containersLoading.value = false
      }
    },
    { immediate: true },
  )

  return { containers, containersLoading, containersError }
}
