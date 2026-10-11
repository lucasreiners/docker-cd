<template>
  <section class="surface logs-panel">
    <div class="panel-heading">
      <h2>Container logs</h2>
      <span class="tag">{{ streaming ? "Following" : status }}</span>
    </div>
    <div class="logs-toolbar">
      <label
        ><span class="sr-only">Log container</span
        ><select v-model="selected" aria-label="Log container">
          <option
            v-for="container in containers"
            :key="container.id"
            :value="container.id"
          >
            {{ container.service }} · {{ container.name }}
          </option>
        </select></label
      ><input
        v-model="search"
        type="search"
        aria-label="Filter log lines"
        placeholder="Filter log lines…"
      /><button
        class="button"
        :disabled="!selected"
        @click="streaming ? pause() : load()"
      >
        {{ streaming ? "Pause logs" : "Follow logs" }}</button
      ><button class="button" :disabled="!text" @click="download">
        Download
      </button>
    </div>
    <p v-if="error" class="notice notice-error" role="alert">{{ error }}</p>
    <pre ref="output" class="log-output" aria-label="Container log output">{{
      filtered ||
      (containers.length
        ? "Waiting for log output…"
        : "No containers available.")
    }}</pre>
    <p class="logs-caption">
      Last 200 lines on connect · Display limited to 200 KB · Streams reconnect
      on demand after 5 minutes or 1 MiB
    </p>
  </section>
</template>
<script setup lang="ts">
import { computed, nextTick, onUnmounted, ref, watch } from 'vue'
import { apiURL, type ContainerInfo } from '../services/api'

const props = defineProps<{ stackPath: string; containers: ContainerInfo[] }>()
const selected = ref(''),
  search = ref(''),
  text = ref(''),
  error = ref(''),
  streaming = ref(false),
  status = ref('Idle'),
  output = ref<HTMLElement | null>(null)
let controller: AbortController | null = null
const filtered = computed(() =>
  search.value
    ? text.value
        .split('\n')
        .filter((line) => line.toLowerCase().includes(search.value.toLowerCase()))
        .join('\n')
    : text.value,
)
function pause() {
  controller?.abort()
  controller = null
  streaming.value = false
  status.value = 'Paused'
}
async function load() {
  pause()
  if (!selected.value) return
  const current = new AbortController()
  controller = current
  text.value = ''
  error.value = ''
  streaming.value = true
  status.value = 'Connecting'
  try {
    const response = await fetch(
      apiURL(
        `/api/container-logs?stack=${encodeURIComponent(props.stackPath)}&container=${encodeURIComponent(selected.value)}&follow=true&tail=200`,
      ),
      { signal: current.signal },
    )
    if (!response.ok) {
      const body = await response.json().catch(() => ({}))
      throw new Error(body.error || `Logs unavailable (${response.status})`)
    }
    if (!response.body) throw new Error('Log streaming is unavailable')
    const reader = response.body.getReader(),
      decoder = new TextDecoder()
    try {
      while (true) {
        const { done, value } = await reader.read()
        if (current !== controller) break
        if (done) {
          text.value = (text.value + decoder.decode()).slice(-200000)
          status.value = 'Stream ended'
          break
        }
        text.value = (text.value + decoder.decode(value, { stream: true })).slice(-200000)
        await nextTick()
        if (output.value) output.value.scrollTop = output.value.scrollHeight
      }
    } finally {
      reader.releaseLock()
    }
  } catch (e) {
    if (!current.signal.aborted && controller === current) {
      error.value = e instanceof Error ? e.message : 'Log stream failed'
      status.value = 'Unavailable'
    }
  } finally {
    if (controller === current) streaming.value = false
  }
}
watch(
  () => props.containers.map((c) => c.id).join(','),
  () => {
    if (!props.containers.some((c) => c.id === selected.value))
      selected.value = props.containers[0]?.id || ''
  },
  { immediate: true },
)
watch(selected, () => load(), { immediate: true })
onUnmounted(pause)
function download() {
  const url = URL.createObjectURL(new Blob([text.value], { type: 'text/plain' }))
  const link = document.createElement('a')
  link.href = url
  link.download = `${props.stackPath.replace(/\//g, '-')}-logs.txt`
  link.click()
  URL.revokeObjectURL(url)
}
</script>
<style scoped>
.logs-toolbar {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  padding: 16px;
}
.logs-toolbar input {
  flex: 1;
  min-width: 140px;
  border: 1px solid var(--border-color);
  border-radius: 5px;
  padding: 7px 10px;
  background: var(--bg-primary);
  color: var(--text-primary);
}
.logs-toolbar select {
  max-width: 100%;
}
.logs-toolbar label {
  max-width: 100%;
}
.log-output {
  height: 460px;
  overflow: auto;
  margin: 0 16px 16px;
  padding: 18px;
  background: #101720;
  color: #c6d8e9;
  border-radius: 6px;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  font: 11px/1.8 var(--font-mono);
}
.logs-caption {
  font-size: 10px;
  color: var(--text-tertiary);
  padding: 0 16px 16px;
}
</style>
