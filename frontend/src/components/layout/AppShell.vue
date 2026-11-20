<script setup lang="ts">
import AppSidebar from './AppSidebar.vue'
</script>

<template>
  <div class="flex h-dvh w-full overflow-hidden bg-slate-50 text-slate-900 dark:bg-slate-950 dark:text-slate-100">
    <AppSidebar />
    <div class="min-w-0 flex-1 overflow-hidden">
      <slot />
    </div>
  </div>
</template>
