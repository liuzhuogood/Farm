<script setup lang="ts">
defineProps<{ title: string, description?: string }>()
</script>

<template>
  <div class="h-full overflow-y-auto">
    <main class="mx-auto w-full max-w-4xl px-4 py-6 sm:px-8">
      <header class="mb-6">
        <h1 class="text-2xl font-bold tracking-tight">{{ title }}</h1>
        <p v-if="description" class="mt-1 text-sm text-muted-foreground">{{ description }}</p>
      </header>
      <slot />
    </main>
  </div>
</template>
