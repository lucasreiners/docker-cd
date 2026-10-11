<template>
  <div>
    <router-link class="back-link" to="/operations"
      >← All operations</router-link
    >
    <div class="page-heading">
      <div>
        <p class="eyebrow">Execution / Detail</p>
        <h1>Operation details</h1>
      </div>
    </div>
    <section class="surface padded"><OperationDetails :id="id" /></section>
  </div>
</template>
<script setup lang="ts">
import { computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import OperationDetails from '../components/OperationDetails.vue'
import { type Operation, requestJSON } from '../services/api'
import { useConsoleStore } from '../store/console'

const route = useRoute(),
  store = useConsoleStore(),
  id = computed(() => String(route.params.id))
watch(
  id,
  async (value) => {
    try {
      store.onOperation(
        await requestJSON<Operation>(`/api/operations/${encodeURIComponent(value)}`),
      )
    } catch (e) {
      store.operationsError = e instanceof Error ? e.message : 'Operation unavailable'
    }
  },
  { immediate: true },
)
</script>
