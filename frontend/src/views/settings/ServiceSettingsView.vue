<script setup lang="ts">
import { onMounted } from 'vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import CopyValueButton from '@/components/common/CopyValueButton.vue'
import SettingsPage from '@/components/system/SettingsPage.vue'
import { useDesktopConfig } from '@/composables/useDesktopConfig'
import { useServiceConfig } from '@/composables/useServiceConfig'

const desktopConfig = useDesktopConfig()
const serviceConfig = useServiceConfig()

onMounted(() => {
  void desktopConfig.load()
  void serviceConfig.load()
})

async function save() {
  if (desktopConfig.isDesktop.value && !desktopConfig.config.value?.is_overridden) {
    await desktopConfig.save()
  }
  await serviceConfig.save()
}
</script>

<template>
  <SettingsPage title="服务" description="本地服务端口与 AI 视觉决策服务。">
    <div class="space-y-6">
      <section class="space-y-3 rounded-xl border bg-card p-4">
        <div class="flex items-center justify-between gap-3">
          <div>
            <label for="desktop-port" class="text-sm font-medium">服务端口</label>
            <p class="mt-1 text-xs text-muted-foreground">桌面内部服务与外部调用的本地回环端口（默认 1989）</p>
          </div>
          <Badge variant="outline">{{ desktopConfig.config.value?.effective ? `运行中 · ${desktopConfig.config.value.effective}` : '默认 1989' }}</Badge>
        </div>
        <Input
          id="desktop-port"
          v-model="desktopConfig.draftPort.value"
          type="number"
          min="1"
          max="65535"
          autocomplete="off"
          :disabled="desktopConfig.saving.value || desktopConfig.config.value?.is_overridden"
          class="w-40"
        />
        <p v-if="desktopConfig.config.value?.is_overridden" class="text-xs text-muted-foreground">
          受环境变量覆盖 ({{ desktopConfig.config.value?.env_port }})
        </p>
        <p class="text-xs text-muted-foreground">重启生效</p>
      </section>

      <section class="space-y-4 rounded-xl border bg-card p-4">
        <div>
          <h2 class="text-sm font-medium">AI 视觉决策服务</h2>
          <p class="mt-1 text-xs text-muted-foreground">配置兼容官方视觉协议的 Chat Completions 服务，用于页面状态决策分析</p>
        </div>
        <div class="space-y-3 text-xs">
          <div>
            <label for="ai-endpoint" class="mb-1 block text-muted-foreground">接口完整地址 (Chat Completions)</label>
            <div class="flex items-center gap-0.5">
              <Input id="ai-endpoint" v-model="serviceConfig.config.value.ai_chat_endpoint" placeholder="https://api.openai.com/v1/chat/completions" class="w-full font-mono text-xs" />
              <CopyValueButton :value="serviceConfig.config.value.ai_chat_endpoint" label="接口地址" />
            </div>
          </div>
          <div>
            <label for="ai-model" class="mb-1 block text-muted-foreground">模型名称</label>
            <Input id="ai-model" v-model="serviceConfig.config.value.ai_model" placeholder="gpt-4o" class="w-full font-mono text-xs" />
          </div>
          <div>
            <label for="ai-key" class="mb-1 block text-muted-foreground">API 密钥</label>
            <Input
              id="ai-key"
              v-model="serviceConfig.apiKeyDraft.value"
              type="password"
              :placeholder="serviceConfig.config.value.ai_has_key ? `${serviceConfig.config.value.ai_api_key_masked} (留空保持原密钥)` : '输入 API Key'"
              class="w-full font-mono text-xs"
            />
          </div>
          <div class="w-36">
            <label for="ai-timeout" class="mb-1 block text-muted-foreground">请求超时 (秒)</label>
            <Input id="ai-timeout" v-model="serviceConfig.config.value.ai_timeout_seconds" type="number" min="5" max="120" class="h-8" />
          </div>
        </div>
      </section>

      <div class="flex items-center gap-3">
        <p v-if="desktopConfig.error.value || serviceConfig.error.value" role="alert" class="flex-1 text-xs text-destructive">
          {{ desktopConfig.error.value || serviceConfig.error.value }}
        </p>
        <p v-else-if="desktopConfig.message.value || serviceConfig.message.value" role="status" class="flex-1 text-xs text-muted-foreground">
          {{ desktopConfig.message.value || serviceConfig.message.value }}
        </p>
        <Button
          size="sm"
          class="ml-auto bg-blue-600 text-white hover:bg-blue-700"
          :disabled="desktopConfig.saving.value || serviceConfig.saving.value"
          @click="save"
        >
          保存
        </Button>
      </div>
    </div>
  </SettingsPage>
</template>
