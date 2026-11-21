<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Github, Loader2 } from 'lucide-vue-next'
import SettingsPage from '@/components/system/SettingsPage.vue'

const version = ref('')
const error = ref('')

async function loadVersion() {
  try {
    const response = await fetch('/api/version/health')
    if (!response.ok) throw new Error(`HTTP ${response.status}`)
    const data = await response.json() as { version?: string }
    version.value = data.version?.trim() || '未知'
  }
  catch (cause) {
    console.error('[关于] 读取版本失败', cause)
    error.value = '版本信息读取失败'
  }
}

onMounted(loadVersion)
</script>

<template>
  <SettingsPage title="关于" description="当前应用版本与项目地址。">
    <section class="divide-y rounded-xl border bg-card">
      <div class="flex items-center justify-between gap-4 px-4 py-3">
        <span class="text-sm">当前版本</span>
        <span v-if="version" class="font-mono text-sm font-semibold">{{ version }}</span>
        <span v-else-if="error" class="text-xs text-destructive">{{ error }}</span>
        <Loader2 v-else class="size-4 animate-spin text-muted-foreground" />
      </div>
      <div class="flex items-center justify-between gap-4 px-4 py-3">
        <span class="text-sm">GitHub</span>
        <a
          href="https://github.com/liuzhuogood/Farm"
          target="_blank"
          rel="noopener noreferrer"
          class="flex min-w-0 items-center gap-1.5 text-sm text-blue-600 hover:underline dark:text-blue-400"
        >
          <Github class="size-4 shrink-0" />
          <span class="truncate">github.com/liuzhuogood/Farm</span>
        </a>
      </div>
    </section>
  </SettingsPage>
</template>
