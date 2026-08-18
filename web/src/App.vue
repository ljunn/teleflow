<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Activity,
  Bot,
  CheckCircle2,
  Download,
  LayoutDashboard,
  Megaphone,
  MessageSquareReply,
  RefreshCw,
  Search,
  Settings,
  ShieldCheck,
  Users,
} from '@lucide/vue'
import { api, type Overview, type ReleaseInfo, type SystemInfo } from './api'

const info = ref<SystemInfo | null>(null)
const overview = ref<Overview | null>(null)
const release = ref<ReleaseInfo | null>(null)
const loading = ref(true)
const checking = ref(false)
const error = ref('')

const versionText = computed(() => info.value?.version || 'dev')

async function load() {
  loading.value = true
  error.value = ''
  try {
    ;[info.value, overview.value] = await Promise.all([api.systemInfo(), api.overview()])
  } catch (err) {
    error.value = err instanceof Error ? err.message : '加载失败'
  } finally {
    loading.value = false
  }
}

async function checkUpdate() {
  checking.value = true
  error.value = ''
  try {
    release.value = await api.checkUpdate()
  } catch (err) {
    error.value = err instanceof Error ? err.message : '检查更新失败'
  } finally {
    checking.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand"><Bot :size="24" /><span>Teleflow</span></div>
      <nav aria-label="主菜单">
        <a class="active" href="#overview"><LayoutDashboard :size="18" />总览</a>
        <a href="#accounts"><Users :size="18" />账号矩阵</a>
        <a href="#discover"><Search :size="18" />数据采集</a>
        <a href="#campaigns"><Megaphone :size="18" />营销任务</a>
        <a href="#relay"><MessageSquareReply :size="18" />主号中转</a>
        <a href="#settings"><Settings :size="18" />系统设置</a>
      </nav>
      <div class="sidebar-foot">
        <span class="status-dot"></span>服务运行中
        <small>{{ versionText }}</small>
      </div>
    </aside>

    <main>
      <header>
        <div>
          <p>运行中心</p>
          <h1>系统总览</h1>
        </div>
        <button class="icon-button" title="刷新数据" aria-label="刷新数据" @click="load">
          <RefreshCw :size="18" :class="{ spinning: loading }" />
        </button>
      </header>

      <p v-if="error" class="error">{{ error }}</p>

      <section id="overview" class="metrics" aria-label="核心指标">
        <article><Users :size="20" /><div><span>已托管账号</span><strong>{{ overview?.accounts ?? '-' }}</strong></div></article>
        <article><Activity :size="20" /><div><span>在线账号</span><strong>{{ overview?.online ?? '-' }}</strong></div></article>
        <article><MessageSquareReply :size="20" /><div><span>今日中转</span><strong>{{ overview?.relayedToday ?? '-' }}</strong></div></article>
        <article><Megaphone :size="20" /><div><span>待执行任务</span><strong>{{ overview?.pendingJobs ?? '-' }}</strong></div></article>
      </section>

      <section class="content-grid">
        <div class="panel">
          <div class="panel-title"><div><h2>服务状态</h2><p>当前实例的基础运行状态</p></div><CheckCircle2 :size="20" /></div>
          <dl>
            <div><dt>应用版本</dt><dd>{{ versionText }}</dd></div>
            <div><dt>构建提交</dt><dd>{{ info?.commit || 'none' }}</dd></div>
            <div><dt>数据存储</dt><dd>SQLite WAL</dd></div>
            <div><dt>部署模式</dt><dd>单机单体</dd></div>
          </dl>
        </div>

        <div id="settings" class="panel update-panel">
          <div class="panel-title"><div><h2>版本升级</h2><p>从 GitHub Releases 获取稳定版本</p></div><ShieldCheck :size="20" /></div>
          <div v-if="!release" class="update-empty">
            <Download :size="28" />
            <p>检查是否有可用的新版本</p>
          </div>
          <div v-else class="release-result">
            <strong v-if="!release.configured">尚未配置 GitHub 仓库</strong>
            <strong v-else-if="release.available">发现新版本 {{ release.latestVersion }}</strong>
            <strong v-else>当前已是最新版本</strong>
            <a v-if="release.releaseUrl" :href="release.releaseUrl" target="_blank" rel="noreferrer">查看发布说明</a>
          </div>
          <button class="primary" :disabled="checking" @click="checkUpdate">
            <RefreshCw :size="17" :class="{ spinning: checking }" />
            {{ checking ? '正在检查' : '检查更新' }}
          </button>
        </div>
      </section>
    </main>
  </div>
</template>
