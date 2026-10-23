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
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import OperationDetails from '../components/OperationDetails.vue'

const route = useRoute(),
  id = computed(() => String(route.params.id))
</script>
