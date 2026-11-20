<script setup lang="ts">
import { computed, ref } from 'vue'
import { ArrowUpRight, Braces, Check, Copy, ExternalLink, Github, TerminalSquare } from 'lucide-vue-next'
import { toast } from 'vue-sonner'
import { Button } from '@/components/ui/button'

const baseUrl = computed(() => window.location.origin)
const docs = [
  {
    title: '自动化接口文档',
    description: 'AI 技能执行前必须读取的核心接口契约：设备发现、观察、操作与任务生命周期。',
    href: '/fram.skill',
    tag: '/fram.skill',
  },
  {
    title: '全量接口文档',
    description: '完整 OpenAPI 3.1 规范页，覆盖设备、投屏、视觉、文件、依赖与系统接口。',
    href: '/api-docs.html',
    tag: '/api-docs.html',
  },
  {
    title: 'OpenAPI 源文件',
    description: '可供客户端生成器、调试工具或自动化系统直接读取的完整规范。',
    href: '/api/openapi.json',
    tag: '/api/openapi.json',
  },
]

const examples = [
  {
    title: '列出在线设备',
    code: `curl "${baseUrl.value}/api/automation/devices"`,
  },
  {
    title: '观察当前页面',
    code: `curl -X POST "${baseUrl.value}/api/automation/observe?serial=<序列号>"`,
  },
  {
    title: '执行一次操作',
    code: `curl -X POST "${baseUrl.value}/api/automation/act?serial=<序列号>&title=打开设置" \\\n  --data-binary $'tap @540,1200'`,
  },
]

const copied = ref('')
let copyTimer: ReturnType<typeof setTimeout> | undefined

async function copy(text: string, key: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = key
    clearTimeout(copyTimer)
    copyTimer = setTimeout(() => { copied.value = '' }, 1500)
  }
  catch {
    toast.error('复制失败，请手动选择文本')
  }
}
</script>

<template>
  <div class="h-full overflow-y-auto">
    <main class="mx-auto w-full max-w-5xl px-4 py-6 sm:px-8">
      <header>
        <h1 class="text-2xl font-bold tracking-tight">API</h1>
        <p class="mt-1 text-sm text-muted-foreground">Farm 对外提供的自动化接口与完整接口规范。</p>
      </header>

      <section class="mt-6 rounded-xl border bg-card p-4">
        <div class="flex items-center gap-2 text-sm font-semibold"><TerminalSquare class="size-4" /> 服务地址</div>
        <div class="mt-3 flex items-center gap-2 rounded-lg border bg-muted/40 px-3 py-2">
          <code class="min-w-0 flex-1 truncate font-mono text-sm">{{ baseUrl }}</code>
          <Button variant="ghost" size="icon-sm" :aria-label="copied === 'base' ? '已复制' : '复制服务地址'" @click="copy(baseUrl, 'base')">
            <Check v-if="copied === 'base'" class="size-4 text-emerald-600" />
            <Copy v-else class="size-4" />
          </Button>
        </div>
        <p class="mt-2 text-xs text-muted-foreground">桌面端默认使用本地回环地址；局域网调用时请使用运行 Farm 的电脑地址。</p>
      </section>

      <section class="mt-6 grid gap-3 sm:grid-cols-3">
        <a
          v-for="doc in docs"
          :key="doc.href"
          :href="doc.href"
          target="_blank"
          rel="noopener noreferrer"
          class="group rounded-xl border bg-card p-4 transition-colors hover:border-blue-500/40 hover:bg-blue-50/40 dark:hover:bg-blue-500/5"
        >
          <div class="flex items-start justify-between gap-2">
            <Braces class="size-5 text-blue-600" />
            <ArrowUpRight class="size-4 text-muted-foreground transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
          </div>
          <h2 class="mt-3 text-sm font-semibold">{{ doc.title }}</h2>
          <p class="mt-1 text-xs leading-5 text-muted-foreground">{{ doc.description }}</p>
          <code class="mt-3 block truncate font-mono text-[11px] text-blue-600 dark:text-blue-300">{{ doc.tag }}</code>
        </a>
      </section>

      <section class="mt-6">
        <h2 class="text-sm font-semibold">常用调用</h2>
        <div class="mt-3 space-y-3">
          <div v-for="(example, index) in examples" :key="example.title" class="overflow-hidden rounded-xl border bg-card">
            <div class="flex items-center justify-between border-b px-3 py-2">
              <span class="text-xs font-medium">{{ example.title }}</span>
              <Button variant="ghost" size="icon-sm" :aria-label="`复制${example.title}`" @click="copy(example.code, String(index))">
                <Check v-if="copied === String(index)" class="size-3.5 text-emerald-600" />
                <Copy v-else class="size-3.5" />
              </Button>
            </div>
            <pre class="overflow-x-auto px-3 py-2.5 font-mono text-[11px] leading-5 text-slate-700 dark:text-slate-300">{{ example.code }}</pre>
          </div>
        </div>
      </section>

      <section class="mt-6 flex flex-wrap items-center gap-3 rounded-xl border bg-card p-4">
        <div class="min-w-0 flex-1">
          <h2 class="text-sm font-semibold">客户端与示例</h2>
          <p class="mt-1 text-xs text-muted-foreground">技能包、Python SDK 与源码都在 GitHub 维护。</p>
        </div>
        <Button variant="outline" size="sm" as-child>
          <a href="https://github.com/liuzhuogood/Farm" target="_blank" rel="noopener noreferrer">
            <Github class="mr-1.5 size-3.5" />GitHub
            <ExternalLink class="ml-1 size-3" />
          </a>
        </Button>
      </section>
    </main>
  </div>
</template>
