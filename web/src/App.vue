<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Activity,
  Bot,
  CheckCircle2,
  Download,
  Eye,
  EyeOff,
  LayoutDashboard,
  LockKeyhole,
  LogOut,
  Megaphone,
  MessageSquareReply,
  RefreshCw,
  Search,
  Settings,
  ShieldCheck,
  Users,
} from '@lucide/vue'
import { api, type AuthStatus, type Overview, type ReleaseInfo, type SystemInfo } from './api'

const auth = ref<AuthStatus | null>(null)
const authLoading = ref(true)
const authSubmitting = ref(false)
const password = ref('')
const confirmPassword = ref('')
const showPassword = ref(false)
const authError = ref('')
const info = ref<SystemInfo | null>(null)
const overview = ref<Overview | null>(null)
const release = ref<ReleaseInfo | null>(null)
const loading = ref(true)
const checking = ref(false)
const updating = ref(false)
const updateMessage = ref('')
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

async function bootstrap() {
  authLoading.value = true
  try {
    auth.value = await api.authStatus()
    if (auth.value.authenticated) await load()
  } catch (err) {
    authError.value = err instanceof Error ? err.message : '无法连接服务'
  } finally {
    authLoading.value = false
  }
}

async function submitAuth() {
  authError.value = ''
  if (!auth.value?.configured && password.value !== confirmPassword.value) {
    authError.value = '两次输入的密码不一致'
    return
  }
  authSubmitting.value = true
  try {
    if (auth.value?.configured) await api.login(password.value)
    else await api.setup(password.value)
    auth.value = { configured: true, authenticated: true }
    password.value = ''
    confirmPassword.value = ''
    await load()
  } catch (err) {
    authError.value = err instanceof Error ? err.message : '操作失败'
  } finally {
    authSubmitting.value = false
  }
}

async function logout() {
  await api.logout()
  auth.value = { configured: true, authenticated: false }
  info.value = null
  overview.value = null
  release.value = null
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

async function applyUpdate() {
  updating.value = true
  error.value = ''
  updateMessage.value = ''
  try {
    const result = await api.applyUpdate()
    release.value = result.release
    updateMessage.value = '新版本已安装，服务正在重启…'
    window.setTimeout(() => window.location.reload(), 4000)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '升级失败'
  } finally {
    updating.value = false
  }
}

onMounted(bootstrap)
</script>

<template>
  <div v-if="authLoading" class="auth-loading" aria-label="正在加载">
    <Bot :size="30" />
    <RefreshCw :size="20" class="spinning" />
  </div>

  <main v-else-if="!auth?.authenticated" class="auth-page">
    <section class="auth-copy">
      <div class="auth-brand"><Bot :size="28" /><span>Teleflow</span></div>
      <div>
        <p class="auth-kicker">私域运营控制台</p>
        <h1>{{ auth?.configured ? '欢迎回来' : '初始化管理员' }}</h1>
        <p>{{ auth?.configured ? '登录后管理账号矩阵、营销任务与消息中转。' : '设置管理员密码，保护此实例中的账号和运营数据。' }}</p>
      </div>
      <small>单一所有者模式 · 本地数据存储</small>
    </section>

    <section class="auth-form-wrap">
      <form class="auth-form" @submit.prevent="submitAuth">
        <div class="form-heading">
          <LockKeyhole :size="21" />
          <div>
            <h2>{{ auth?.configured ? '管理员登录' : '创建管理员密码' }}</h2>
            <p>{{ auth?.configured ? '请输入当前实例的管理员密码' : '密码至少需要 8 个字符' }}</p>
          </div>
        </div>

        <label for="password">管理员密码</label>
        <div class="password-field">
          <input id="password" v-model="password" :type="showPassword ? 'text' : 'password'" :autocomplete="auth?.configured ? 'current-password' : 'new-password'" minlength="8" required autofocus />
          <button type="button" :title="showPassword ? '隐藏密码' : '显示密码'" :aria-label="showPassword ? '隐藏密码' : '显示密码'" @click="showPassword = !showPassword">
            <EyeOff v-if="showPassword" :size="18" />
            <Eye v-else :size="18" />
          </button>
        </div>

        <template v-if="!auth?.configured">
          <label for="confirm-password">确认密码</label>
          <input id="confirm-password" v-model="confirmPassword" :type="showPassword ? 'text' : 'password'" autocomplete="new-password" minlength="8" required />
        </template>

        <p v-if="authError" class="auth-error">{{ authError }}</p>
        <button class="primary auth-submit" :disabled="authSubmitting" type="submit">
          <RefreshCw v-if="authSubmitting" :size="17" class="spinning" />
          <LockKeyhole v-else :size="17" />
          {{ authSubmitting ? '请稍候' : auth?.configured ? '登录' : '完成初始化' }}
        </button>
      </form>
    </section>
  </main>

  <div v-else class="shell">
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
        <div><span class="status-dot"></span>服务运行中</div>
        <button title="退出登录" aria-label="退出登录" @click="logout"><LogOut :size="16" /></button>
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
            <span v-if="updateMessage" class="update-message">{{ updateMessage }}</span>
          </div>
          <button v-if="release?.available" class="primary" :disabled="updating || !!updateMessage" @click="applyUpdate">
            <RefreshCw :size="17" :class="{ spinning: updating }" />
            {{ updateMessage ? '正在重启' : updating ? '正在升级' : '立即升级' }}
          </button>
          <button v-else class="primary" :disabled="checking" @click="checkUpdate">
            <RefreshCw :size="17" :class="{ spinning: checking }" />
            {{ checking ? '正在检查' : '检查更新' }}
          </button>
        </div>
      </section>
    </main>
  </div>
</template>
