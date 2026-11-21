<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { MessageSquare } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import SettingsPage from '@/components/system/SettingsPage.vue'

const type = ref('bug')
const title = ref('')
const detail = ref('')
const version = ref('')
const error = ref('')

const typeLabel = computed(() => ({
  bug: '问题反馈',
  feature: '功能建议',
  other: '其他反馈',
})[type.value] || '问题反馈')
const canSubmit = computed(() => title.value.trim().length > 0 && detail.value.trim().length > 0)

async function loadVersion() {
  try {
    const response = await fetch('/api/version/health')
    if (!response.ok) return
    const data = await response.json() as { version?: string }
    version.value = data.version?.trim() || ''
  }
  catch (cause) {
    console.error('[反馈] 读取版本失败', cause)
  }
}

function submit() {
  if (!canSubmit.value) {
    error.value = '请填写标题和详细描述'
    return
  }
  error.value = ''
  const body = [
    `### 反馈类型`,
    typeLabel.value,
    '',
    `### 详细描述`,
    detail.value.trim(),
    '',
    `### 环境`,
    `- Farm 版本：${version.value || '未知'}`,
    `- 平台：${navigator.userAgent}`,
  ].join('\n')
  const params = new URLSearchParams({
    title: `[${typeLabel.value}] ${title.value.trim()}`,
    body,
  })
  const opened = window.open(`https://github.com/liuzhuogood/Farm/issues/new?${params}`, '_blank')
  if (!opened) error.value = '浏览器阻止了 GitHub 页面，请允许弹出窗口后重试'
}

onMounted(loadVersion)
</script>

<template>
  <SettingsPage title="反馈" description="提交问题或建议到 Farm 的 GitHub Issues。">
    <form class="space-y-4 rounded-xl border bg-card p-4" @submit.prevent="submit">
      <div class="grid gap-4 sm:grid-cols-[180px_minmax(0,1fr)]">
        <label class="space-y-1.5 text-sm">
          <span class="text-xs text-muted-foreground">反馈类型</span>
          <Select v-model="type">
            <SelectTrigger class="w-full" aria-label="反馈类型"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="bug">问题反馈</SelectItem>
              <SelectItem value="feature">功能建议</SelectItem>
              <SelectItem value="other">其他反馈</SelectItem>
            </SelectContent>
          </Select>
        </label>
        <label class="space-y-1.5 text-sm">
          <span class="text-xs text-muted-foreground">标题</span>
          <Input v-model="title" maxlength="120" placeholder="简要描述反馈内容" autocomplete="off" />
        </label>
      </div>
      <label class="block space-y-1.5 text-sm">
        <span class="text-xs text-muted-foreground">详细描述</span>
        <textarea
          v-model="detail"
          rows="8"
          maxlength="8000"
          placeholder="请描述复现步骤、预期结果和实际结果"
          class="w-full resize-y rounded-lg border border-input bg-transparent px-3 py-2 text-sm shadow-xs outline-none transition-[color,box-shadow] placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50"
        />
      </label>
      <div class="flex flex-wrap items-center gap-3">
        <p v-if="error" role="alert" class="flex-1 text-xs text-destructive">{{ error }}</p>
        <p v-else class="flex-1 text-xs text-muted-foreground">提交后会打开 GitHub 新建 Issue 页面，确认内容后再发布。</p>
        <Button type="submit" class="bg-blue-600 text-white hover:bg-blue-700" :disabled="!canSubmit">
          <MessageSquare class="mr-1.5 size-4" />
          提交
        </Button>
      </div>
    </form>
  </SettingsPage>
</template>
